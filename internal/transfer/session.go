package transfer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"artifactregistry/internal/auth"
	"artifactregistry/internal/blob"
	"artifactregistry/internal/model"
	"artifactregistry/internal/repo"
)

// TokenValidator is the auth surface used by upload sessions.
type TokenValidator interface {
	ValidateForSession(ctx context.Context, tokenID, sessionID string) error
	Validate(ctx context.Context, tokenID string) (auth.Token, error)
	Bind(ctx context.Context, tokenID, sessionID string) error
}

// SessionManager owns chunked upload sessions and their chunk data.
type SessionManager struct {
	mu        sync.Mutex
	sessions  map[string]*model.UploadSession
	chunkData map[string]map[int][]byte
	releases  map[string]func()
	blobs     blob.Store
	auth      TokenValidator
	repos     repo.Store
	now       func() time.Time
}

// NewSessionManager creates a session manager.
func NewSessionManager(blobs blob.Store, auth TokenValidator, repos repo.Store) *SessionManager {
	return &SessionManager{
		sessions:  make(map[string]*model.UploadSession),
		chunkData: make(map[string]map[int][]byte),
		releases:  make(map[string]func()),
		blobs:     blobs,
		auth:      auth,
		repos:     repos,
		now:       time.Now,
	}
}

func sessionID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	return "sess-" + hex.EncodeToString(buf[:])
}

// Create opens a new chunked upload for the given artifact size. When
// expectedDigest is known (clients usually know the layer digest from the
// manifest), the digest is staged in the blob store so manifest validation
// can see that the layer is not finished yet.
func (m *SessionManager) Create(ctx context.Context, tokenID, repoName, name string, size, chunkSize int64, expectedDigest string) (model.UploadSession, error) {
	if _, err := m.auth.Validate(ctx, tokenID); err != nil {
		return model.UploadSession{}, err
	}
	if !m.repos.Alive(ctx, repoName) {
		return model.UploadSession{}, repo.ErrRepositoryDeleted
	}
	release, err := m.repos.Enter(ctx, repoName)
	if err != nil {
		return model.UploadSession{}, err
	}
	if name == "" || size <= 0 || chunkSize <= 0 {
		release()
		return model.UploadSession{}, model.ErrInvalidArgument
	}
	if expectedDigest != "" {
		if err := m.blobs.Stage(ctx, repoName, expectedDigest, size); err != nil && err != model.ErrAlreadyExists {
			release()
			return model.UploadSession{}, err
		}
	}
	id := sessionID()
	session := &model.UploadSession{
		ID:        id,
		Repo:      repoName,
		Name:      name,
		Digest:    expectedDigest,
		State:     model.SessionUploading,
		Size:      size,
		ChunkSize: chunkSize,
		TokenID:   tokenID,
		CreatedAt: m.now(),
		UpdatedAt: m.now(),
	}
	m.mu.Lock()
	m.sessions[id] = session
	m.chunkData[id] = make(map[int][]byte)
	m.releases[id] = release
	m.mu.Unlock()
	if err := m.auth.Bind(ctx, tokenID, id); err != nil {
		m.mu.Lock()
		delete(m.sessions, id)
		delete(m.chunkData, id)
		delete(m.releases, id)
		m.mu.Unlock()
		release()
		return model.UploadSession{}, err
	}
	return *session, nil
}

// Get returns a session snapshot after validating the caller's token.
func (m *SessionManager) Get(ctx context.Context, tokenID, sessionID string) (model.UploadSession, error) {
	if err := m.auth.ValidateForSession(ctx, tokenID, sessionID); err != nil {
		return model.UploadSession{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return model.UploadSession{}, model.ErrNotFound
	}
	return *s, nil
}

// Abort releases an in-flight session and its staged blob reservation.
func (m *SessionManager) Abort(ctx context.Context, tokenID, sessionID string) error {
	if err := m.auth.ValidateForSession(ctx, tokenID, sessionID); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return model.ErrNotFound
	}
	_ = s
	delete(m.sessions, sessionID)
	delete(m.chunkData, sessionID)
	if release := m.releases[sessionID]; release != nil {
		delete(m.releases, sessionID)
		release()
	}
	return nil
}

// IsDigestUploading reports whether any session in the repository is still
// uploading an object with the given expected digest.
func (m *SessionManager) IsDigestUploading(ctx context.Context, repoName, digest string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.Repo != repoName || s.State != model.SessionUploading {
			continue
		}
		if s.Digest == digest {
			return true
		}
	}
	return false
}
