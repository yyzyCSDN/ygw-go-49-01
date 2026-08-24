package transfer

import (
	"context"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/model"
)

// chunkSizeFor returns the byte size of chunk index for a session.
func chunkSizeFor(s *model.UploadSession, index int) int64 {
	if index < 0 {
		return 0
	}
	offset := int64(index) * s.ChunkSize
	if offset >= s.Size {
		return 0
	}
	remaining := s.Size - offset
	if remaining < s.ChunkSize {
		return remaining
	}
	return s.ChunkSize
}

// UploadChunk stores one chunk of an upload session. Chunks may arrive in any
// order; each chunk is validated against the size it should occupy so the
// assembled content never overlaps or leaves gaps.
func (m *SessionManager) UploadChunk(ctx context.Context, tokenID, sessionID string, index int, data []byte) error {
	if err := m.auth.ValidateForSession(ctx, tokenID, sessionID); err != nil {
		return err
	}
	if len(data) == 0 {
		return model.ErrInvalidArgument
	}
	m.mu.Lock()
	s, ok := m.sessions[sessionID]
	m.mu.Unlock()
	if !ok {
		return model.ErrNotFound
	}
	if s.State != model.SessionUploading {
		return model.ErrInvalidState
	}
	expected := chunkSizeFor(s, index)
	if expected == 0 {
		return model.ErrInvalidArgument
	}
	if int64(len(data)) != expected {
		return model.ErrConflict
	}
	if _, exists := m.chunkData[sessionID][index]; exists {
		return model.ErrConflict
	}
	dataCopy := append([]byte(nil), data...)
	m.chunkData[sessionID][index] = dataCopy
	s.Chunks = append(s.Chunks, model.Chunk{
		Index:  index,
		Offset: int64(index) * s.ChunkSize,
		Size:   int64(len(data)),
		Digest: blob.DigestOf(data),
	})
	s.Written += int64(len(data))
	s.UpdatedAt = m.now()
	return nil
}

// ResumeOffset returns the byte offset at which the next chunk should start.
// The offset is derived from the contiguous run of present chunks, so a
// partially uploaded last chunk is counted by its actual size, not by the
// fixed chunk size.
func (m *SessionManager) ResumeOffset(ctx context.Context, tokenID, sessionID string) (int64, error) {
	if err := m.auth.ValidateForSession(ctx, tokenID, sessionID); err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return 0, model.ErrNotFound
	}
	if s.State == model.SessionCommitted {
		return s.Size, nil
	}
	present := make(map[int]bool, len(s.Chunks))
	for _, c := range s.Chunks {
		present[c.Index] = true
	}
	var offset int64
	for index := 0; ; index++ {
		if !present[index] {
			break
		}
		size := chunkSizeFor(s, index)
		if size == 0 {
			break
		}
		offset += size
		if offset >= s.Size {
			break
		}
	}
	return offset, nil
}

// Complete assembles every chunk in index order, commits the assembled blob
// and moves the session from uploading to committed. Completion is
// synchronous: the session only reports committed after the blob is stored.
func (m *SessionManager) Complete(ctx context.Context, tokenID, sessionID string) (model.Blob, error) {
	if err := m.auth.ValidateForSession(ctx, tokenID, sessionID); err != nil {
		return model.Blob{}, err
	}
	m.mu.Lock()
	s, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return model.Blob{}, model.ErrNotFound
	}
	if s.State != model.SessionUploading {
		m.mu.Unlock()
		return model.Blob{}, model.ErrInvalidState
	}
	chunkCount := int((s.Size + s.ChunkSize - 1) / s.ChunkSize)
	data := m.chunkData[sessionID]
	if len(data) != chunkCount {
		m.mu.Unlock()
		return model.Blob{}, model.ErrUnfinished
	}
	indexes := make([]int, 0, len(data))
	for index := range data {
		indexes = append(indexes, index)
	}
	assembled := make([]byte, 0, s.Size)
	for _, chunk := range s.Chunks {
		assembled = append(assembled, data[chunk.Index]...)
	}
	session := *s
	m.mu.Unlock()

	if err := session.Verify(); err != nil {
		return model.Blob{}, err
	}
	stored, err := m.blobs.Put(ctx, session.Repo, assembled)
	if err != nil {
		return model.Blob{}, err
	}
	m.mu.Lock()
	current, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return model.Blob{}, model.ErrNotFound
	}
	current.State = model.SessionCommitted
	current.UpdatedAt = m.now()
	if release := m.releases[sessionID]; release != nil {
		delete(m.releases, sessionID)
		release()
	}
	m.mu.Unlock()
	return stored, nil
}
