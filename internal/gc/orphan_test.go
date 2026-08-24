package gc

import (
	"context"
	"testing"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
)

// TestRunOrphanReclaimsRefcountedBlob covers the sweep path that makes orphan
// blobs collectable. A blob committed into a deleted namespace carries an
// upload reference no manifest will ever release, so SweepUnreferenced (which
// honors the refcount) would leave it behind forever. RunOrphan sweeps the
// namespace wholesale and reclaims it.
func TestRunOrphanReclaimsRefcountedBlob(t *testing.T) {
	ctx := context.Background()
	repos := repo.NewMemoryStore()
	_, _ = repos.Create(ctx, "repo-a")
	blobs := blob.NewMemoryStore(4)
	manifests := manifest.NewMemoryStore(blobs, nil)
	tags := tag.NewMemoryStore()
	g := NewGC(repos, tags, manifests, blobs)

	// Plant the blob the race window produces: committed, refcount 1, no
	// manifest references it.
	data := []byte("orphan-committed-by-race")
	b, err := blobs.Put(ctx, "repo-a", data)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	// Mark the namespace as an orphan (the condition Delete records).
	_, _ = repos.Enter(ctx, "repo-a")
	_ = repos.Delete(ctx, "repo-a")

	if err := g.RunOrphan(ctx, "repo-a"); err != nil {
		t.Fatalf("run orphan: %v", err)
	}
	if blobs.Exists(ctx, "repo-a", b.Digest) {
		t.Fatal("orphan blob must be reclaimed by RunOrphan")
	}
	if got := blobs.List(ctx, "repo-a"); len(got) != 0 {
		t.Fatalf("orphan namespace must be empty after RunOrphan, got %v", got)
	}
}

// TestRunOrphanReclaimsOrphanedManifest confirms that manifests left in a
// deleted namespace are dropped together with their blobs.
func TestRunOrphanReclaimsOrphanedManifest(t *testing.T) {
	ctx := context.Background()
	repos := repo.NewMemoryStore()
	_, _ = repos.Create(ctx, "repo-a")
	blobs := blob.NewMemoryStore(4)
	manifests := manifest.NewMemoryStore(blobs, nil)
	tags := tag.NewMemoryStore()
	g := NewGC(repos, tags, manifests, blobs)

	m, _ := manifestFor(t, blobs, ctx, "repo-a")
	if err := manifests.Store(ctx, "repo-a", m); err != nil {
		t.Fatalf("store: %v", err)
	}
	_, _ = repos.Enter(ctx, "repo-a")
	_ = repos.Delete(ctx, "repo-a")

	if err := g.RunOrphan(ctx, "repo-a"); err != nil {
		t.Fatalf("run orphan: %v", err)
	}
	if manifests.Exists(ctx, "repo-a", m.Digest) {
		t.Fatal("orphan manifest must be reclaimed")
	}
	for _, d := range m.Descriptors() {
		if blobs.Exists(ctx, "repo-a", d.Digest) {
			t.Fatalf("orphan manifest blob %s must be reclaimed", d.Digest)
		}
	}
}
