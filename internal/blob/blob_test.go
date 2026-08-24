package blob

import (
	"bytes"
	"context"
	"testing"
)

func TestPutGetAndDedup(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(4)
	data := []byte("layer-bytes-123")
	first, err := store.Put(ctx, "repo-a", data)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if first.RefCount != 1 {
		t.Fatalf("expected refcount 1, got %d", first.RefCount)
	}
	second, err := store.Put(ctx, "repo-a", data)
	if err != nil {
		t.Fatalf("second put: %v", err)
	}
	if second.Digest != first.Digest {
		t.Fatalf("dedup failed: %s != %s", second.Digest, first.Digest)
	}
	if got := store.RefCount(ctx, "repo-a", first.Digest); got != 2 {
		t.Fatalf("expected refcount 2 after dedup, got %d", got)
	}
	read, err := store.Get(ctx, "repo-a", first.Digest)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(read, data) {
		t.Fatal("read data mismatch")
	}
	if !store.Committed(ctx, "repo-a", first.Digest) {
		t.Fatal("stored blob must be committed")
	}
}

func TestStageBlocksCommittedAndUnfinishedGet(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(2)
	digest := "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if err := store.Stage(ctx, "repo-a", digest, 100); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if !store.Exists(ctx, "repo-a", digest) {
		t.Fatal("staged blob must exist")
	}
	if store.Committed(ctx, "repo-a", digest) {
		t.Fatal("staged blob must not be committed")
	}
	if _, err := store.Get(ctx, "repo-a", digest); err == nil {
		t.Fatal("get on staged blob must fail")
	}
	if err := store.DecRef(ctx, "repo-a", digest); err != nil {
		t.Fatalf("dec on staged blob must not error: %v", err)
	}
}

func TestDecRefThenSweepReclaims(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(3)
	data := []byte("orphan-bytes")
	b, err := store.Put(ctx, "repo-a", data)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := store.DecRef(ctx, "repo-a", b.Digest); err != nil {
		t.Fatalf("decref: %v", err)
	}
	removed, err := SweepUnreferencedBlobs(ctx, store, "repo-a", map[string]bool{})
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(removed) != 1 || removed[0] != b.Digest {
		t.Fatalf("expected one removal, got %v", removed)
	}
	if store.Exists(ctx, "repo-a", b.Digest) {
		t.Fatal("swept blob must be gone")
	}
}

func TestReachableBlobSurvivesSweep(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore(2)
	data := []byte("keep-me")
	b, err := store.Put(ctx, "repo-a", data)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	removed, err := SweepUnreferencedBlobs(ctx, store, "repo-a", map[string]bool{b.Digest: true})
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("reachable blob must survive, removed %v", removed)
	}
}

func TestShardStable(t *testing.T) {
	if Shard("sha256:aaa", 8) != Shard("sha256:aaa", 8) {
		t.Fatal("shard must be stable for a digest")
	}
	if ShardLabel("sha256:aaa", 8) == "" {
		t.Fatal("shard label must not be empty")
	}
}
