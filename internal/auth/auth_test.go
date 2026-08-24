package auth

import (
	"context"
	"testing"
	"time"
)

func TestIssueValidate(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	token, err := store.Issue(ctx, "", time.Minute)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := store.Validate(ctx, token.ID)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got.ID != token.ID {
		t.Fatalf("unexpected token: %+v", got)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{tokens: map[string]Token{}, sessions: map[string]string{}, now: func() time.Time { return time.Unix(0, 0) }}
	token := Token{ID: "t", Family: "f", ExpiresAt: time.Unix(-1, 0)}
	store.tokens["t"] = token
	if _, err := store.Validate(ctx, "t"); err == nil {
		t.Fatal("expired token must be rejected")
	}
}

func TestIssueBindAndValidateForSession(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	token, err := store.Issue(ctx, "sess-1", time.Minute)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if err := store.ValidateForSession(ctx, token.ID, "sess-1"); err != nil {
		t.Fatalf("validate for session: %v", err)
	}
	if err := store.ValidateForSession(ctx, token.ID, "sess-2"); err == nil {
		t.Fatal("token must not validate against another session")
	}
}

func TestRenewKeepsTokenUsable(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	token, _ := store.Issue(ctx, "sess-1", time.Minute)
	renewed, err := store.Renew(ctx, token.ID, time.Minute)
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if renewed.ID == token.ID {
		t.Fatal("renewed token must have a new id")
	}
	if _, err := store.Validate(ctx, renewed.ID); err != nil {
		t.Fatalf("renewed token must validate: %v", err)
	}
}

func TestBindAttachesSession(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	token, _ := store.Issue(ctx, "", time.Minute)
	if err := store.Bind(ctx, token.ID, "sess-9"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := store.ValidateForSession(ctx, token.ID, "sess-9"); err != nil {
		t.Fatalf("validate after bind: %v", err)
	}
}
