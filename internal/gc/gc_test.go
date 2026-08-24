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

func newGC(t *testing.T) (*GC, blob.Store, manifest.Store, tag.Store) {
	t.Helper()
	ctx := context.Background()
	repos := repo.NewMemoryStore()
	_, _ = repos.Create(ctx, "repo-a")
	blobs := blob.NewMemoryStore(4)
	manifests := manifest.NewMemoryStore(blobs, nil)
	tags := tag.NewMemoryStore()
	return NewGC(repos, tags, manifests, blobs), blobs, manifests, tags
}

func manifestFor(t *testing.T, blobs blob.Store, ctx context.Context, repoName string) (*model.Manifest, []byte) {
	t.Helper()
	config := []byte("cfg")
	layer := []byte("lay")
	configBlob, _ := blobs.Put(ctx, repoName, config)
	layerBlob, _ := blobs.Put(ctx, repoName, layer)
	payload := model.Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config:        model.Descriptor{MediaType: "application/octet-stream", Digest: configBlob.Digest, Size: 3},
		Layers:        []model.Descriptor{{MediaType: "application/octet-stream", Digest: layerBlob.Digest, Size: 3}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	m, err := manifest.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return m, raw
}

func TestGCKeepsReferencedManifest(t *testing.T) {
	ctx := context.Background()
	g, blobs, manifests, tags := newGC(t)
	m, _ := manifestFor(t, blobs, ctx, "repo-a")
	if err := manifests.Store(ctx, "repo-a", m); err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := tags.Set(ctx, "repo-a", "v1", m.Digest); err != nil {
		t.Fatalf("tag: %v", err)
	}
	if err := g.Run(ctx, "repo-a"); err != nil {
		t.Fatalf("gc: %v", err)
	}
	if !manifests.Exists(ctx, "repo-a", m.Digest) {
		t.Fatal("referenced manifest must survive GC")
	}
	for _, descriptor := range m.Descriptors() {
		if !blobs.Exists(ctx, "repo-a", descriptor.Digest) {
			t.Fatalf("referenced blob %s must survive GC", descriptor.Digest)
		}
	}
}

func TestGCDeletesUnreferencedManifest(t *testing.T) {
	ctx := context.Background()
	g, blobs, manifests, _ := newGC(t)
	m, _ := manifestFor(t, blobs, ctx, "repo-a")
	if err := manifests.Store(ctx, "repo-a", m); err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := g.Run(ctx, "repo-a"); err != nil {
		t.Fatalf("gc: %v", err)
	}
	if manifests.Exists(ctx, "repo-a", m.Digest) {
		t.Fatal("unreferenced manifest must be collected")
	}
}

func TestGCReclaimsReleasedBlob(t *testing.T) {
	ctx := context.Background()
	g, blobs, _, _ := newGC(t)
	data := []byte("releasable")
	b, err := blobs.Put(ctx, "repo-a", data)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := blobs.DecRef(ctx, "repo-a", b.Digest); err != nil {
		t.Fatalf("dec: %v", err)
	}
	if err := g.Run(ctx, "repo-a"); err != nil {
		t.Fatalf("gc: %v", err)
	}
	if blobs.Exists(ctx, "repo-a", b.Digest) {
		t.Fatal("released blob must be collected")
	}
}

func TestMarkProducesReachableSet(t *testing.T) {
	ctx := context.Background()
	g, blobs, manifests, tags := newGC(t)
	m, _ := manifestFor(t, blobs, ctx, "repo-a")
	_ = manifests.Store(ctx, "repo-a", m)
	_ = tags.Set(ctx, "repo-a", "v1", m.Digest)
	blobsReachable, manifestsReachable, err := g.Mark(ctx, "repo-a")
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	if !manifestsReachable[m.Digest] {
		t.Fatal("tagged manifest must be reachable")
	}
	for _, descriptor := range m.Descriptors() {
		if !blobsReachable[descriptor.Digest] {
			t.Fatalf("blob %s must be reachable", descriptor.Digest)
		}
	}
}
