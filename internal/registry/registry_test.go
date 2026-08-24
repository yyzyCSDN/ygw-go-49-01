package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"artifactregistry/internal/auth"
	"artifactregistry/internal/blob"
	"artifactregistry/internal/index"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/model"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
	"artifactregistry/internal/transfer"
)

func newRegistry(t *testing.T) (*Registry, string) {
	t.Helper()
	ctx := context.Background()
	repos := repo.NewMemoryStore()
	blobs := blob.NewMemoryStore(4)
	authStore := auth.NewMemoryStore()
	sessions := transfer.NewSessionManager(blobs, authStore, repos)
	manifests := manifest.NewMemoryStore(blobs, sessions)
	tags := tag.NewMemoryStore()
	indexStore := index.NewMemoryStore()
	reg := New(repos, blobs, manifests, tags, indexStore, authStore, sessions)
	if _, err := reg.CreateRepo(ctx, "repo-a"); err != nil {
		t.Fatalf("create repo: %v", err)
	}
	token, err := reg.IssueToken(ctx, 3600)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return reg, token.ID
}

func TestPushPullAndGC(t *testing.T) {
	ctx := context.Background()
	reg, tokenID := newRegistry(t)
	config := []byte("config-bytes")
	layer := []byte("layer-bytes")
	configBlob, err := reg.PushBlob(ctx, tokenID, "repo-a", config)
	if err != nil {
		t.Fatalf("push config: %v", err)
	}
	layerBlob, err := reg.PushBlob(ctx, tokenID, "repo-a", layer)
	if err != nil {
		t.Fatalf("push layer: %v", err)
	}
	payload := model.Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config:        model.Descriptor{MediaType: "application/octet-stream", Digest: configBlob.Digest, Size: int64(len(config))},
		Layers:        []model.Descriptor{{MediaType: "application/octet-stream", Digest: layerBlob.Digest, Size: int64(len(layer))}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := reg.PushManifest(ctx, tokenID, "repo-a", raw, "v1"); err != nil {
		t.Fatalf("push manifest: %v", err)
	}
	m, err := reg.PullManifest(ctx, tokenID, "repo-a", "v1")
	if err != nil {
		t.Fatalf("pull manifest: %v", err)
	}
	if m.Digest == "" {
		t.Fatal("manifest digest missing")
	}
	got, err := reg.PullBlob(ctx, tokenID, "repo-a", layerBlob.Digest)
	if err != nil {
		t.Fatalf("pull blob: %v", err)
	}
	if !bytes.Equal(got, layer) {
		t.Fatal("layer mismatch")
	}
	if _, err := reg.RunGC(ctx, "repo-a"); err != nil {
		t.Fatalf("gc: %v", err)
	}
	if _, err := reg.PullManifest(ctx, tokenID, "repo-a", "v1"); err != nil {
		t.Fatalf("published artifact must survive GC: %v", err)
	}
	artifacts := reg.ListArtifacts(ctx, "repo-a")
	if len(artifacts) != 1 || artifacts[0].Name != "v1" {
		t.Fatalf("unexpected artifacts: %+v", artifacts)
	}
}

func TestChunkedRoundTripThroughRegistry(t *testing.T) {
	ctx := context.Background()
	reg, tokenID := newRegistry(t)
	content := []byte("chunked-artifact-content-0123456789abcde")
	expectedDigest := blob.DigestOf(content)
	session, err := reg.CreateSession(ctx, tokenID, "repo-a", "chunk.bin", int64(len(content)), 8, expectedDigest)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	for start := 0; start < len(content); start += 8 {
		end := start + 8
		if end > len(content) {
			end = len(content)
		}
		if err := reg.UploadChunk(ctx, tokenID, session.ID, start/8, content[start:end]); err != nil {
			t.Fatalf("chunk %d: %v", start/8, err)
		}
	}
	offset, err := reg.ResumeOffset(ctx, tokenID, session.ID)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if offset != int64(len(content)) {
		t.Fatalf("resume offset %d != %d", offset, len(content))
	}
	stored, err := reg.CompleteSession(ctx, tokenID, session.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if stored.Digest != expectedDigest {
		t.Fatalf("stored digest mismatch")
	}
}

func TestDeleteRepoAndList(t *testing.T) {
	ctx := context.Background()
	reg, _ := newRegistry(t)
	if err := reg.DeleteRepo(ctx, "repo-a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if names := reg.ListRepos(ctx); len(names) != 0 {
		t.Fatalf("expected no repos, got %v", names)
	}
}
