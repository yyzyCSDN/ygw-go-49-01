package repo

import (
	"context"
	"testing"
)

// TestRepoDeleteOrphansBlob deletes a repository while a push is still in
// flight. The deleted namespace must be recorded so GC can sweep blobs that
// land after the deletion; otherwise they become permanent orphans.
func TestRepoDeleteOrphansBlob(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	if _, err := store.Create(ctx, "team-a"); err != nil {
		t.Fatal(err)
	}
	release, err := store.Enter(ctx, "team-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "team-a"); err != nil {
		t.Fatal(err)
	}
	release()
	orphans := store.Orphans(ctx)
	if len(orphans) != 1 || orphans[0] != "team-a" {
		t.Fatalf("deleted repo with in-flight push must be recorded for GC, got %v", orphans)
	}
}
