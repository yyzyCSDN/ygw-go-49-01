package transfer

import (
	"context"
	"testing"
	"time"

	"artifactregistry/internal/auth"
	"artifactregistry/internal/blob"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
)

// TestCompleteSessionRefusedAfterDelete reproduces the core race: a chunked
// upload that entered before a concurrent Delete must not be allowed to commit
// its assembled bytes into the deleted repository. Such a commit would leave an
// orphan blob carrying an upload reference that no manifest ever releases.
func TestCompleteSessionRefusedAfterDelete(t *testing.T) {
	ctx := context.Background()
	repos := repo.NewMemoryStore()
	_, _ = repos.Create(ctx, "repo-a")
	blobs := blob.NewMemoryStore(4)
	authStore := auth.NewMemoryStore()
	sessions := NewSessionManager(blobs, authStore, repos)
	manifests := manifest.NewMemoryStore(blobs, sessions)
	tags := tag.NewMemoryStore()
	pusher := NewPusher(authStore, blobs, manifests, tags, nil, sessions, repos)
	token, _ := authStore.Issue(ctx, "", time.Hour)

	content := []byte("chunked-orphan-content-1234567890")
	expectedDigest := blob.DigestOf(content)
	session, err := pusher.CreateSession(ctx, token.ID, "repo-a", "big.bin", int64(len(content)), 8, expectedDigest)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	for start := 0; start < len(content); start += 8 {
		end := start + 8
		if end > len(content) {
			end = len(content)
		}
		if err := pusher.UploadChunk(ctx, token.ID, session.ID, start/8, content[start:end]); err != nil {
			t.Fatalf("chunk %d: %v", start/8, err)
		}
	}

	// Delete lands while the push is in flight. The namespace must be marked as
	// an orphan so GC can still reach it.
	if err := repos.Delete(ctx, "repo-a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if orphans := repos.Orphans(ctx); len(orphans) != 1 || orphans[0] != "repo-a" {
		t.Fatalf("deleted repo with inflight push must be orphan, got %v", orphans)
	}

	// Completing into the deleted repo must be refused, and no committed blob
	// must be left behind in the blob store.
	if _, err := pusher.CompleteSession(ctx, token.ID, session.ID); err == nil {
		t.Fatal("complete into deleted repo must be rejected")
	}
	for _, digest := range blobs.List(ctx, "repo-a") {
		if blobs.Committed(ctx, "repo-a", digest) {
			t.Fatalf("no committed blob must remain after refused complete, found %s", digest)
		}
	}
}

// TestOrphanBlobReclaimedByNamespaceSweep covers the TOCTOU tail: a blob that
// committed into a deleted namespace (the window between the Alive check and
// Put) carries a refcount no manifest will release. The refcount guard in
// SweepUnreferenced would protect it forever; an orphan namespace sweep must
// reclaim it.
func TestOrphanBlobReclaimedByNamespaceSweep(t *testing.T) {
	ctx := context.Background()
	repos := repo.NewMemoryStore()
	_, _ = repos.Create(ctx, "repo-a")
	blobs := blob.NewMemoryStore(4)
	_ = repos.Delete(ctx, "repo-a")

	data := []byte("orphan-race-bytes")
	b, err := blobs.Put(ctx, "repo-a", data)
	if err != nil {
		t.Fatalf("put orphan: %v", err)
	}
	if got := blobs.RefCount(ctx, "repo-a", b.Digest); got != 1 {
		t.Fatalf("expected refcount 1, got %d", got)
	}

	// The normal sweep honors the refcount and must not reclaim the blob.
	if _, err := blob.SweepUnreferencedBlobs(ctx, blobs, "repo-a", map[string]bool{}); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if !blobs.Exists(ctx, "repo-a", b.Digest) {
		t.Fatal("refcount guard must protect blob from normal sweep")
	}

	// The orphan namespace sweep ignores the refcount and reclaims it.
	removed, err := blob.SweepNamespaceBlobs(ctx, blobs, "repo-a")
	if err != nil {
		t.Fatalf("sweep namespace: %v", err)
	}
	if len(removed) != 1 || removed[0] != b.Digest {
		t.Fatalf("expected orphan reclaimed, got %v", removed)
	}
	if blobs.Exists(ctx, "repo-a", b.Digest) {
		t.Fatal("orphan blob must be reclaimed by namespace sweep")
	}
}
