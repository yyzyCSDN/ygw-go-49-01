package transfer

import (
	"context"
	"testing"
	"time"

	"artifactregistry/internal/auth"
	"artifactregistry/internal/blob"
	"artifactregistry/internal/index"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
)

// TestChunkResumeOffsetCorrect uploads a partially filled last chunk and
// checks that the resume offset is derived from the actual contiguous bytes,
// not from the fixed chunk size multiplied by the chunk count.
func TestChunkResumeOffsetCorrect(t *testing.T) {
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
	content := make([]byte, 250)
	for i := range content {
		content[i] = byte(i)
	}
	session, err := pusher.CreateSession(ctx, token.ID, "repo-a", "big.bin", 250, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := pusher.UploadChunk(ctx, token.ID, session.ID, 0, content[0:100]); err != nil {
		t.Fatal(err)
	}
	if err := pusher.UploadChunk(ctx, token.ID, session.ID, 1, content[100:200]); err != nil {
		t.Fatal(err)
	}
	if err := pusher.UploadChunk(ctx, token.ID, session.ID, 2, content[200:250]); err != nil {
		t.Fatal(err)
	}
	offset, err := pusher.ResumeOffset(ctx, token.ID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 250 {
		t.Fatalf("resume offset after partial last chunk = %d, want 250", offset)
	}
}
