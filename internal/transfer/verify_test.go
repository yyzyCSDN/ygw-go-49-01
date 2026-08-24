package transfer

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"artifactregistry/internal/auth"
	"artifactregistry/internal/blob"
	"artifactregistry/internal/index"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
)

// TestChunkMergeOrderConsistent uploads chunks concurrently and verifies the
// assembled artifact matches the index order. Completion order must never
// decide the file layout.
func TestChunkMergeOrderConsistent(t *testing.T) {
	ctx := context.Background()
	repos := repo.NewMemoryStore()
	if _, err := repos.Create(ctx, "repo-a"); err != nil {
		t.Fatal(err)
	}
	blobs := blob.NewMemoryStore(4)
	authStore := auth.NewMemoryStore()
	sessions := NewSessionManager(blobs, authStore, repos)
	manifests := manifest.NewMemoryStore(blobs, sessions)
	tags := tag.NewMemoryStore()
	indexStore := index.NewMemoryStore()
	pusher := NewPusher(authStore, blobs, manifests, tags, indexStore, sessions, repos)
	token, err := authStore.Issue(ctx, "", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("0123456789abcdefghij")
	expectedDigest := blob.DigestOf(content)
	session, err := pusher.CreateSession(ctx, token.ID, "repo-a", "merge.bin", int64(len(content)), 5, expectedDigest)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := pusher.UploadChunk(ctx, token.ID, session.ID, i, content[i*5:(i+1)*5]); err != nil {
				t.Errorf("chunk %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	stored, err := pusher.CompleteSession(ctx, token.ID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := pusher.PullBlob(ctx, token.ID, "repo-a", stored.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("merged content mismatch: got %q want %q", data, content)
	}
}
