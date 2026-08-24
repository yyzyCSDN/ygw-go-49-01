package manifest

import (
	"context"
	"encoding/json"
	"testing"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/index"
	"artifactregistry/internal/model"
)

// TestIndexNoCorruptArtifact simulates the publish decision for a manifest
// whose blobs failed verification. The index must not list the artifact until
// every blob verified successfully.
func TestIndexNoCorruptArtifact(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemoryStore(4)
	store := NewMemoryStore(blobs, nil)
	indexStore := index.NewMemoryStore()
	missing := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	payload := model.Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config:        model.Descriptor{MediaType: "application/octet-stream", Digest: missing, Size: 1},
		Layers:        []model.Descriptor{{MediaType: "application/octet-stream", Digest: missing, Size: 1}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := store.Verify(ctx, "repo-a", m)
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("manifest with missing blobs must not verify")
	}
	if err := index.UpdateIndex(ctx, indexStore, "repo-a", "v1", m.Digest, ready); err == nil {
		t.Fatal("index must reject an artifact whose blobs failed verification")
	}
	if indexStore.Contains(ctx, "repo-a", "v1") {
		t.Fatal("unverified artifact must not be listed")
	}
}
