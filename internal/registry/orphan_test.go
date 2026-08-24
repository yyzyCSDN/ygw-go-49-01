package registry

import (
	"context"
	"testing"

	"artifactregistry/internal/blob"
)

// TestRunGCReclaimsOrphanNamespace is the end-to-end reproduction of the
// reported incident: a repository is deleted while a push is in flight, the
// push leaves an orphan blob behind, and a subsequent GC run must reclaim it
// and forget the namespace so the orphan does not come back.
func TestRunGCReclaimsOrphanNamespace(t *testing.T) {
	ctx := context.Background()
	reg, _ := newRegistry(t)

	// Simulate the race window directly: a blob committed into repo-a after it
	// was deleted while a push was in flight. It carries a refcount no manifest
	// will release.
	repoName := "repo-a"
	_, _ = reg.repos.Enter(ctx, repoName)
	if err := reg.DeleteRepo(ctx, repoName); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if orphans := reg.OrphanNamespaces(ctx); len(orphans) != 1 || orphans[0] != repoName {
		t.Fatalf("expected orphan [repo-a], got %v", orphans)
	}
	data := []byte("orphan-from-concurrent-push")
	digest := blob.DigestOf(data)
	if _, err := reg.blobs.Put(ctx, repoName, data); err != nil {
		t.Fatalf("put orphan: %v", err)
	}

	// The blob is unreachable by normal GC and protected by its refcount.
	if _, err := reg.RunGC(ctx, repoName); err != nil {
		t.Fatalf("gc: %v", err)
	}
	// RunGC scans the orphan namespace (repoName is itself the orphan) and
	// reclaims the blob wholesale.
	if reg.blobs.Exists(ctx, repoName, digest) {
		t.Fatal("orphan blob must be reclaimed by RunGC")
	}
	// The namespace must be forgotten once reclaimed so it does not linger and
	// the manual-clear-stays-cleared property holds.
	if orphans := reg.OrphanNamespaces(ctx); len(orphans) != 0 {
		t.Fatalf("orphan must be forgotten after reclaim, got %v", orphans)
	}
}

// TestRunGCKeepsRequestedRepoSeparateFromOrphans ensures that reclaiming an
// orphan namespace does not touch the live repository the GC run targeted.
func TestRunGCKeepsRequestedRepoSeparateFromOrphans(t *testing.T) {
	ctx := context.Background()
	reg, _ := newRegistry(t)
	// repo-a is created by newRegistry. Make a second namespace the orphan.
	_, _ = reg.CreateRepo(ctx, "repo-b")
	_, _ = reg.repos.Enter(ctx, "repo-b")
	_ = reg.DeleteRepo(ctx, "repo-b")
	orphanDigest := blob.DigestOf([]byte("orphan-in-repo-b"))
	if _, err := reg.blobs.Put(ctx, "repo-b", []byte("orphan-in-repo-b")); err != nil {
		t.Fatalf("put orphan: %v", err)
	}
	// Plant a live, unreferenced blob in repo-a (refcount 0) that normal GC
	// should reclaim.
	liveOrphan := []byte("unreferenced-in-repo-a")
	b, err := reg.blobs.Put(ctx, "repo-a", liveOrphan)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := reg.blobs.DecRef(ctx, "repo-a", b.Digest); err != nil {
		t.Fatalf("decref: %v", err)
	}

	if _, err := reg.RunGC(ctx, "repo-a"); err != nil {
		t.Fatalf("gc: %v", err)
	}
	// repo-a's unreferenced blob reclaimed by the normal sweep path.
	if reg.blobs.Exists(ctx, "repo-a", b.Digest) {
		t.Fatal("unreferenced blob in live repo must be reclaimed")
	}
	// repo-b's refcounted orphan reclaimed by the orphan sweep path.
	if reg.blobs.Exists(ctx, "repo-b", orphanDigest) {
		t.Fatal("orphan blob in deleted repo must be reclaimed")
	}
	if orphans := reg.OrphanNamespaces(ctx); len(orphans) != 0 {
		t.Fatalf("orphan must be forgotten, got %v", orphans)
	}
	// repo-a must still be alive and untouched.
	if !reg.repos.Alive(ctx, "repo-a") {
		t.Fatal("live repo must not be deleted by GC")
	}
}
