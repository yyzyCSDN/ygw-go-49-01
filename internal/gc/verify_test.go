package gc

import (
	"context"
	"encoding/json"
	"testing"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/model"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
)

// TestOldManifestKeptWhileTagPoints points two tags at the same manifest. The
// reference ledger must agree with the tag table so GC never collects a
// manifest that is still referenced.
func TestOldManifestKeptWhileTagPoints(t *testing.T) {
	ctx := context.Background()
	repos := repo.NewMemoryStore()
	if _, err := repos.Create(ctx, "repo-a"); err != nil {
		t.Fatal(err)
	}
	blobs := blob.NewMemoryStore(4)
	manifests := manifest.NewMemoryStore(blobs, nil)
	tags := tag.NewMemoryStore()
	g := NewGC(repos, tags, manifests, blobs)

	config, _ := blobs.Put(ctx, "repo-a", []byte("cfg"))
	layer, _ := blobs.Put(ctx, "repo-a", []byte("lay"))
	payload := model.Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config:        model.Descriptor{MediaType: "application/octet-stream", Digest: config.Digest, Size: 3},
		Layers:        []model.Descriptor{{MediaType: "application/octet-stream", Digest: layer.Digest, Size: 3}},
	}
	raw, _ := json.Marshal(payload)
	m, err := manifest.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifests.Store(ctx, "repo-a", m); err != nil {
		t.Fatal(err)
	}
	if err := tags.Set(ctx, "repo-a", "v1", m.Digest); err != nil {
		t.Fatal(err)
	}
	if err := tags.Set(ctx, "repo-a", "v2", m.Digest); err != nil {
		t.Fatal(err)
	}
	if err := g.Run(ctx, "repo-a"); err != nil {
		t.Fatal(err)
	}
	if !manifests.Exists(ctx, "repo-a", m.Digest) {
		t.Fatal("manifest referenced by two tags must survive GC")
	}
	for _, descriptor := range m.Descriptors() {
		if !blobs.Exists(ctx, "repo-a", descriptor.Digest) {
			t.Fatalf("blob %s of a referenced manifest must survive GC", descriptor.Digest)
		}
	}
}
