package model

import "time"

// SessionState is the upload session lifecycle. A session accepts chunks while
// uploading, transitions to verifying once every chunk is present and the
// assembled content is being checked, and becomes committed when the digest
// check passed and the blob is stored.
type SessionState int

const (
	// SessionUploading accepts new chunks.
	SessionUploading SessionState = iota
	// SessionVerifying has all chunks and is checking the assembled digest.
	SessionVerifying
	// SessionCommitted means the assembled blob is stored and complete.
	SessionCommitted
)

// String returns a stable name for the session state.
func (s SessionState) String() string {
	switch s {
	case SessionUploading:
		return "uploading"
	case SessionVerifying:
		return "verifying"
	case SessionCommitted:
		return "committed"
	default:
		return "unknown"
	}
}

// Chunk records one accepted chunk of an upload session.
type Chunk struct {
	Index  int
	Offset int64
	Size   int64
	Digest string
}

// UploadSession tracks a chunked upload and its resume state.
type UploadSession struct {
	ID        string
	Repo      string
	Name      string
	Digest    string
	State     SessionState
	Size      int64
	ChunkSize int64
	Chunks    []Chunk
	Written   int64
	TokenID   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AdvanceTo moves the session through its allowed transitions.
func (s *UploadSession) AdvanceTo(state SessionState) error {
	if s == nil {
		return ErrInvalidArgument
	}
	switch s.State {
	case SessionUploading:
		if state != SessionVerifying {
			return ErrInvalidState
		}
	case SessionVerifying:
		if state != SessionCommitted {
			return ErrInvalidState
		}
	case SessionCommitted:
		return ErrInvalidState
	default:
		return ErrInvalidState
	}
	s.State = state
	s.UpdatedAt = time.Now()
	return nil
}

// Verify starts the verification phase of the session.
func (s *UploadSession) Verify() error {
	return s.AdvanceTo(SessionVerifying)
}

// Commit finishes the session after verification passed.
func (s *UploadSession) Commit() error {
	return s.AdvanceTo(SessionCommitted)
}
