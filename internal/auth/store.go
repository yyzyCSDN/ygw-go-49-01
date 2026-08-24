package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"artifactregistry/internal/model"
)

type memoryStore struct {
	mu       sync.RWMutex
	tokens   map[string]Token
	sessions map[string]string
	now      func() time.Time
}

// NewMemoryStore creates an in-memory token store.
func NewMemoryStore() Store {
	return &memoryStore{
		tokens:   make(map[string]Token),
		sessions: make(map[string]string),
		now:      time.Now,
	}
}

func randomID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

func (s *memoryStore) Issue(ctx context.Context, sessionID string, ttl time.Duration) (Token, error) {
	if ttl <= 0 {
		return Token{}, model.ErrInvalidArgument
	}
	family := "fam-" + randomID()
	s.mu.Lock()
	defer s.mu.Unlock()
	if sessionID != "" {
		s.sessions[sessionID] = family
	}
	token := Token{
		ID:        "tok-" + randomID(),
		SessionID: sessionID,
		Family:    family,
		ExpiresAt: s.now().Add(ttl),
	}
	s.tokens[token.ID] = token
	return token, nil
}

// Renew creates a successor token that keeps the same family and session
// binding, so upload sessions authenticated with the old token continue to
// work with the renewed one.
func (s *memoryStore) Renew(ctx context.Context, tokenID string, ttl time.Duration) (Token, error) {
	if ttl <= 0 {
		return Token{}, model.ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.tokens[tokenID]
	if !ok {
		return Token{}, model.ErrNotFound
	}
	family := "fam-" + randomID()
	token := Token{
		ID:        "tok-" + randomID(),
		SessionID: old.SessionID,
		Family:    family,
		ExpiresAt: s.now().Add(ttl),
	}
	s.tokens[token.ID] = token
	return token, nil
}

func (s *memoryStore) Validate(ctx context.Context, tokenID string) (Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	token, ok := s.tokens[tokenID]
	if !ok {
		return Token{}, model.ErrUnauthorized
	}
	if token.Expired(s.now()) {
		return Token{}, model.ErrUnauthorized
	}
	return token, nil
}

// ValidateForSession checks that the token is valid, bound to the given
// session, and still in the session's current token family.
func (s *memoryStore) ValidateForSession(ctx context.Context, tokenID, sessionID string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	token, ok := s.tokens[tokenID]
	if !ok || token.Expired(s.now()) {
		return model.ErrUnauthorized
	}
	if token.SessionID != sessionID {
		return model.ErrUnauthorized
	}
	family, bound := s.sessions[sessionID]
	if !bound || family != token.Family {
		return model.ErrUnauthorized
	}
	return nil
}

// Bind attaches a token to a session family.
func (s *memoryStore) Bind(ctx context.Context, tokenID, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.tokens[tokenID]
	if !ok {
		return model.ErrNotFound
	}
	token.SessionID = sessionID
	s.sessions[sessionID] = token.Family
	s.tokens[tokenID] = token
	return nil
}
