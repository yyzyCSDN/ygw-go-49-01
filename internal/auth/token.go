package auth

import (
	"context"
	"time"
)

// Token is a short-lived credential bound to one upload session family. A
// renewed token keeps the same family so an in-flight push can keep sending
// chunks after its original token expired.
type Token struct {
	ID        string
	SessionID string
	Family    string
	ExpiresAt time.Time
}

// Expired reports whether the token is past its expiry.
func (t Token) Expired(now time.Time) bool {
	return now.After(t.ExpiresAt)
}

// Store issues, renews and validates push tokens.
type Store interface {
	Issue(ctx context.Context, sessionID string, ttl time.Duration) (Token, error)
	Renew(ctx context.Context, tokenID string, ttl time.Duration) (Token, error)
	Validate(ctx context.Context, tokenID string) (Token, error)
	ValidateForSession(ctx context.Context, tokenID, sessionID string) error
	Bind(ctx context.Context, tokenID, sessionID string) error
}
