package transfer

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"artifactregistry/internal/auth"
	"artifactregistry/internal/blob"
	"artifactregistry/internal/index"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/model"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
)

func newHarness(t *testing.T) (*Pusher, *SessionManager, string) {
	t.Helper()
	ctx := context.Background()
	repos := repo.NewMemoryStore()
	_, _ = repos.Create(ctx, "repo-a")
	blobs := blob.NewMemoryStore(4)
	authStore := auth.NewMemoryStore()
	sessions := NewSessionManager(blobs, authStore, repos)
	manifests := manifest.NewMemoryStore(blobs, sessions)
	tags := tag.NewMemoryStore()
	indexStore := index.NewMemoryStore()
	pusher := NewPusher(authStore, blobs, manifests, tags, indexStore, sessions, repos)
	token, _ := authStore.Issue(ctx, "", time.Hour)
	return pusher, sessions, token.ID
}

func TestPushBlobSequentialDedup(t *testing.T) {
	ctx := context.Background()
	pusher, _, tokenID := newHarness(t)
	data := []byte("same-layer")
	first, err := pusher.PushBlob(ctx, tokenID, "repo-a", data)
	if err != nil {
		t.Fatalf("push blob: %v", err)
	}
	second, err := pusher.PushBlob(ctx, tokenID, "repo-a", data)
	if err != nil {
		t.Fatalf("push blob again: %v", err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("dedup mismatch: %s != %s", first.Digest, second.Digest)
	}
}

func TestSessionHappyPath(t *testing.T) {
	ctx := context.Background()
	pusher, _, tokenID := newHarness(t)
	chunks := [][]byte{[]byte("aaa"), []byte("bbb"), []byte("ccc")}
	assembled := bytes.Join(chunks, nil)
	expectedDigest := blob.DigestOf(assembled)
	session, err := pusher.CreateSession(ctx, tokenID, "repo-a", "big.bin", int64(len(assembled)), 3, expectedDigest)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	for index, chunk := range chunks {
		if err := pusher.UploadChunk(ctx, tokenID, session.ID, index, chunk); err != nil {
			t.Fatalf("upload chunk %d: %v", index, err)
		}
	}
	offset, err := pusher.ResumeOffset(ctx, tokenID, session.ID)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if offset != int64(len(assembled)) {
		t.Fatalf("expected resume offset %d, got %d", len(assembled), offset)
	}
	stored, err := pusher.CompleteSession(ctx, tokenID, session.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if stored.Digest != expectedDigest {
		t.Fatalf("digest mismatch: %s != %s", stored.Digest, expectedDigest)
	}
	data, err := pusher.PullBlob(ctx, tokenID, "repo-a", stored.Digest)
	if err != nil {
		t.Fatalf("pull blob: %v", err)
	}
	if !bytes.Equal(data, assembled) {
		t.Fatal("assembled content mismatch")
	}
}

func TestSessionRejectsDuplicateChunk(t *testing.T) {
	ctx := context.Background()
	pusher, _, tokenID := newHarness(t)
	session, err := pusher.CreateSession(ctx, tokenID, "repo-a", "dup.bin", 6, 3, "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := pusher.UploadChunk(ctx, tokenID, session.ID, 0, []byte("abc")); err != nil {
		t.Fatalf("chunk 0: %v", err)
	}
	if err := pusher.UploadChunk(ctx, tokenID, session.ID, 0, []byte("xyz")); err == nil {
		t.Fatal("duplicate chunk must be rejected")
	}
}

func TestPushManifestReleasesUploadRef(t *testing.T) {
	ctx := context.Background()
	pusher, _, tokenID := newHarness(t)
	config := []byte("cfg")
	layer := []byte("lay")
	configBlob, _ := pusher.PushBlob(ctx, tokenID, "repo-a", config)
	layerBlob, _ := pusher.PushBlob(ctx, tokenID, "repo-a", layer)
	payload := model.Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config:        model.Descriptor{MediaType: "application/octet-stream", Digest: configBlob.Digest, Size: int64(len(config))},
		Layers:        []model.Descriptor{{MediaType: "application/octet-stream", Digest: layerBlob.Digest, Size: int64(len(layer))}},
	}
	raw := marshalManifest(t, payload)
	if err := pusher.PushManifest(ctx, tokenID, "repo-a", raw, "v1"); err != nil {
		t.Fatalf("push manifest: %v", err)
	}
	m, err := pusher.PullManifest(ctx, tokenID, "repo-a", "v1")
	if err != nil {
		t.Fatalf("pull manifest: %v", err)
	}
	if m.Digest == "" {
		t.Fatal("manifest digest must be set")
	}
	fetched, err := pusher.FetchArtifact(ctx, tokenID, "repo-a", "v1")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !bytes.Equal(fetched[layerBlob.Digest], layer) {
		t.Fatal("fetched layer mismatch")
	}
}

func marshalManifest(t *testing.T, m model.Manifest) []byte {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
