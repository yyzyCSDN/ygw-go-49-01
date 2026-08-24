package transfer

import (
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

// TestPushManifestRejectsUnfinishedLayer reproduces the bug where a manifest
// referencing a layer that is still uploading (staged, chunks incomplete) was
// accepted by the push, returning success, while the published layer had no
// bytes. The push must fail before the manifest is stored.
func TestPushManifestRejectsUnfinishedLayer(t *testing.T) {
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

	// Commit a real config blob so the only unfinished reference is the layer.
	config := []byte("cfg")
	configBlob, err := pusher.PushBlob(ctx, token.ID, "repo-a", config)
	if err != nil {
		t.Fatalf("push config: %v", err)
	}

	// Open a chunked upload for the layer but do NOT upload all chunks or
	// complete it. Because the digest is known, Create stages it: a verifying
	// record exists (Exists == true) with no committed bytes.
	layer := []byte("the-layer-bytes")
	layerDigest := blob.DigestOf(layer)
	if _, err := pusher.CreateSession(ctx, token.ID, "repo-a", "layer.bin",
		int64(len(layer)), 3, layerDigest); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Sanity: the staged layer is recorded but unfinished.
	if !blobs.Exists(ctx, "repo-a", layerDigest) {
		t.Fatal("staged layer record must exist")
	}
	if blobs.Committed(ctx, "repo-a", layerDigest) {
		t.Fatal("staged layer must not be committed")
	}
	if !sessions.IsDigestUploading(ctx, "repo-a", layerDigest) {
		t.Fatal("layer digest must report as uploading")
	}

	// Build a manifest that references the unfinished layer.
	payload := model.Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config:        model.Descriptor{MediaType: "application/octet-stream",
			Digest: configBlob.Digest, Size: int64(len(config))},
		Layers: []model.Descriptor{{MediaType: "application/octet-stream",
			Digest: layerDigest, Size: int64(len(layer))}},
	}
	raw, _ := json.Marshal(payload)

	// The push must be rejected, not silently accepted.
	if err := pusher.PushManifest(ctx, token.ID, "repo-a", raw, "v1"); err != model.ErrUnfinished {
		t.Fatalf("push manifest: want ErrUnfinished, got %v", err)
	}
	// And the tag must not have been pointed at a half-built image.
	if _, err := pusher.PullManifest(ctx, token.ID, "repo-a", "v1"); err == nil {
		t.Fatal("manifest must not be retrievable after rejected push")
	}
}
