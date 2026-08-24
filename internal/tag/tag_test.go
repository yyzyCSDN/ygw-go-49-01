package tag

import (
	"context"
	"testing"
)

func TestSetGetDelete(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	if err := store.Set(ctx, "repo-a", "v1", "sha256:aaaa"); err != nil {
		t.Fatalf("set: %v", err)
	}
	digest, err := store.Get(ctx, "repo-a", "v1")
	if err != nil || digest != "sha256:aaaa" {
		t.Fatalf("get: %q err=%v", digest, err)
	}
	if err := store.Set(ctx, "repo-a", "v1", "sha256:bbbb"); err != nil {
		t.Fatalf("repoint: %v", err)
	}
	digest, _ = store.Get(ctx, "repo-a", "v1")
	if digest != "sha256:bbbb" {
		t.Fatalf("expected repointed digest, got %q", digest)
	}
	if err := store.Delete(ctx, "repo-a", "v1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(ctx, "repo-a", "v1"); err == nil {
		t.Fatal("deleted tag must not resolve")
	}
}

func TestLedgerMatchesSingleTag(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_ = store.Set(ctx, "repo-a", "v1", "sha256:cccc")
	refs := store.Refs(ctx, "repo-a")
	if refs["sha256:cccc"] != 1 {
		t.Fatalf("expected ledger 1, got %d", refs["sha256:cccc"])
	}
	list := store.List(ctx, "repo-a")
	if len(list) != 1 || list["v1"] != "sha256:cccc" {
		t.Fatalf("unexpected list: %v", list)
	}
}

func TestRepointUpdatesLedger(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	_ = store.Set(ctx, "repo-a", "v1", "sha256:dddd")
	_ = store.Set(ctx, "repo-a", "v1", "sha256:eeee")
	refs := store.Refs(ctx, "repo-a")
	if refs["sha256:dddd"] != 0 {
		t.Fatalf("old digest must be released, got %d", refs["sha256:dddd"])
	}
	if refs["sha256:eeee"] != 1 {
		t.Fatalf("new digest must have ledger 1, got %d", refs["sha256:eeee"])
	}
}

func TestRevisionAdvances(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	before := store.Revision(ctx, "repo-a")
	_ = store.Set(ctx, "repo-a", "v1", "sha256:ffff")
	after := store.Revision(ctx, "repo-a")
	if after <= before {
		t.Fatalf("revision must advance: %d -> %d", before, after)
	}
}

func TestSortedNames(t *testing.T) {
	got := SortedNames(map[string]string{"b": "x", "a": "y"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected sorted names: %v", got)
	}
}
