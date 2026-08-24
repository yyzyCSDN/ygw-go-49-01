package auth

import (
	"context"
	"testing"
	"time"
)

// TestPushSurvivesTokenRenew renews a token while an upload session is open.
// The renewed token must keep the session's family binding so the in-flight
// push can keep sending chunks instead of being interrupted.
func TestPushSurvivesTokenRenew(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	token, err := store.Issue(ctx, "sess-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	renewed, err := store.Renew(ctx, token.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ValidateForSession(ctx, renewed.ID, "sess-1"); err != nil {
		t.Fatalf("renewed token must keep the upload session binding: %v", err)
	}
}
