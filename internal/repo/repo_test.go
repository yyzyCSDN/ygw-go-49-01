package repo

import (
	"context"
	"testing"
)

func TestCreateGetDeleteLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	if _, err := store.Create(ctx, "team-a"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.Create(ctx, "team-a"); err == nil {
		t.Fatal("duplicate create must fail")
	}
	r, err := store.Get(ctx, "team-a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if r.Name != "team-a" || r.Deleted() {
		t.Fatalf("unexpected repo: %+v", r)
	}
	if !store.Alive(ctx, "team-a") {
		t.Fatal("repo must be alive")
	}
	if err := store.Delete(ctx, "team-a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if store.Alive(ctx, "team-a") {
		t.Fatal("deleted repo must not be alive")
	}
	if _, err := store.Get(ctx, "team-a"); err == nil {
		t.Fatal("get on deleted repo must fail")
	}
	if names := store.List(ctx); len(names) != 0 {
		t.Fatalf("expected no live repos, got %v", names)
	}
}

func TestDeleteWithoutInflightProducesNoOrphans(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_, _ = store.Create(ctx, "team-a")
	if err := store.Delete(ctx, "team-a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if orphans := store.Orphans(ctx); len(orphans) != 0 {
		t.Fatalf("expected no orphans, got %v", orphans)
	}
}

func TestEnterGuardCountsAndReleases(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_, _ = store.Create(ctx, "team-a")
	release, err := store.Enter(ctx, "team-a")
	if err != nil {
		t.Fatalf("enter: %v", err)
	}
	if store.Count(ctx, "team-a") != 1 {
		t.Fatalf("expected inflight 1, got %d", store.Count(ctx, "team-a"))
	}
	release()
	if store.Count(ctx, "team-a") != 0 {
		t.Fatalf("expected inflight 0 after release, got %d", store.Count(ctx, "team-a"))
	}
}

func TestEnterRefusedAfterDelete(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_, _ = store.Create(ctx, "team-a")
	_ = store.Delete(ctx, "team-a")
	if _, err := store.Enter(ctx, "team-a"); err == nil {
		t.Fatal("enter into deleted repo must be refused")
	}
}
