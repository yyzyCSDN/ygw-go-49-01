package index

import (
	"context"
	"testing"

	"artifactregistry/internal/model"
)

func TestAddRequiresNameAndDigest(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	if err := store.Add(ctx, "repo-a", "", "sha256:aaaa", true); err == nil {
		t.Fatal("add without name must fail")
	}
	if err := store.Add(ctx, "repo-a", "img", "", true); err == nil {
		t.Fatal("add without digest must fail")
	}
	if err := store.Add(ctx, "repo-a", "img", "sha256:aaaa", true); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !store.Contains(ctx, "repo-a", "img") {
		t.Fatal("added artifact must be listed")
	}
}

func TestAddListRemove(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	if err := store.Add(ctx, "repo-a", "img", "sha256:aaaa", true); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := store.Add(ctx, "repo-a", "sidecar", "sha256:bbbb", true); err != nil {
		t.Fatalf("add: %v", err)
	}
	list := store.List(ctx, "repo-a")
	if len(list) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(list))
	}
	for _, artifact := range list {
		if artifact.State != model.ArtifactPublished {
			t.Fatalf("artifact must be published, got %v", artifact.State)
		}
	}
	if err := store.Remove(ctx, "repo-a", "img"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if store.Contains(ctx, "repo-a", "img") {
		t.Fatal("removed artifact must not be listed")
	}
}

func TestAddUpdateDigest(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_ = store.Add(ctx, "repo-a", "img", "sha256:aaaa", true)
	if err := store.Add(ctx, "repo-a", "img", "sha256:cccc", true); err != nil {
		t.Fatalf("update: %v", err)
	}
	list := store.List(ctx, "repo-a")
	if len(list) != 1 || list[0].Digest != "sha256:cccc" {
		t.Fatalf("expected updated digest, got %+v", list)
	}
}

func TestUpdateIndexWrapper(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	if err := UpdateIndex(ctx, store, "repo-a", "img", "sha256:aaaa", true); err != nil {
		t.Fatalf("update index: %v", err)
	}
}
