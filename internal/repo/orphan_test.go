package repo

import (
	"context"
	"testing"
)

// TestDeleteWithInflightMarksOrphan reproduces the root cause: a repository
// deleted while a push is still in flight must be recorded as an orphan so a
// later GC pass can return to it. Before the fix Delete never touched the
// orphans map, so Orphans() always returned empty and the blobs those pushes
// committed became permanent orphans.
func TestDeleteWithInflightMarksOrphan(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_, _ = store.Create(ctx, "team-a")

	release, err := store.Enter(ctx, "team-a")
	if err != nil {
		t.Fatalf("enter: %v", err)
	}
	if err := store.Delete(ctx, "team-a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	orphans := store.Orphans(ctx)
	if len(orphans) != 1 || orphans[0] != "team-a" {
		t.Fatalf("repo deleted with inflight push must be an orphan, got %v", orphans)
	}

	// The orphan marker stays until GC forgets it, even after the in-flight
	// push completes; a push that commits between GC passes keeps the namespace
	// listed so the next pass still reaches it.
	release()
	if orphans = store.Orphans(ctx); len(orphans) != 1 {
		t.Fatalf("orphan must persist past push completion until GC forgets it, got %v", orphans)
	}

	// Once GC reclaims the namespace the marker is dropped, so the namespace
	// does not stay on the orphan list forever and manual cleanup stays cleared.
	store.ForgetOrphan(ctx, "team-a")
	if orphans = store.Orphans(ctx); len(orphans) != 0 {
		t.Fatalf("orphan must be forgotten after GC reclaims it, got %v", orphans)
	}
}
