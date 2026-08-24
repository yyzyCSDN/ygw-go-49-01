package transfer

import (
	"context"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/index"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/model"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
)

// Pusher drives the push pipeline: authenticate, upload blobs with dedup,
// validate and store the manifest, point the tag and publish the index entry.
type Pusher struct {
	auth      TokenValidator
	blobs     blob.Store
	manifests manifest.Store
	tags      tag.Store
	index     index.Store
	sessions  *SessionManager
	repos     repo.Store
}

// NewPusher wires the push pipeline components together.
func NewPusher(
	auth TokenValidator,
	blobs blob.Store,
	manifests manifest.Store,
	tags tag.Store,
	indexStore index.Store,
	sessions *SessionManager,
	repos repo.Store,
) *Pusher {
	return &Pusher{
		auth:      auth,
		blobs:     blobs,
		manifests: manifests,
		tags:      tags,
		index:     indexStore,
		sessions:  sessions,
		repos:     repos,
	}
}

// PushBlob uploads one layer with dedup and returns its committed record.
func (p *Pusher) PushBlob(ctx context.Context, tokenID, repoName string, data []byte) (model.Blob, error) {
	if _, err := p.auth.Validate(ctx, tokenID); err != nil {
		return model.Blob{}, err
	}
	if !p.repos.Alive(ctx, repoName) {
		return model.Blob{}, repo.ErrRepositoryDeleted
	}
	return p.blobs.Put(ctx, repoName, data)
}

// PushManifest stores a manifest, points the tag at it and publishes the
// index entry. The upload reference added by PushBlob is released after the
// manifest is durably stored so the blob's remaining references are exact.
func (p *Pusher) PushManifest(ctx context.Context, tokenID, repoName string, raw []byte, ref string) error {
	if _, err := p.auth.Validate(ctx, tokenID); err != nil {
		return err
	}
	m, err := manifest.Parse(raw)
	if err != nil {
		return err
	}
	if err := p.manifests.Validate(ctx, repoName, m); err != nil {
		return err
	}
	if err := p.manifests.Store(ctx, repoName, m); err != nil {
		return err
	}
	if err := p.tags.Set(ctx, repoName, ref, m.Digest); err != nil {
		return err
	}
	ready, err := p.manifests.Verify(ctx, repoName, m)
	if err != nil {
		return err
	}
	if err := index.UpdateIndex(ctx, p.index, repoName, ref, m.Digest, ready); err != nil {
		return err
	}
	for _, descriptor := range m.Descriptors() {
		_ = p.blobs.DecRef(ctx, repoName, descriptor.Digest)
	}
	return nil
}

// CreateSession opens a chunked upload session for an artifact.
func (p *Pusher) CreateSession(ctx context.Context, tokenID, repoName, name string, size, chunkSize int64, expectedDigest string) (model.UploadSession, error) {
	return p.sessions.Create(ctx, tokenID, repoName, name, size, chunkSize, expectedDigest)
}

// UploadChunk forwards a chunk to the session manager.
func (p *Pusher) UploadChunk(ctx context.Context, tokenID, sessionID string, index int, data []byte) error {
	return p.sessions.UploadChunk(ctx, tokenID, sessionID, index, data)
}

// ResumeOffset forwards a resume request to the session manager.
func (p *Pusher) ResumeOffset(ctx context.Context, tokenID, sessionID string) (int64, error) {
	return p.sessions.ResumeOffset(ctx, tokenID, sessionID)
}

// CompleteSession finishes a chunked upload after confirming the repository
// is still alive. A push that commits into a deleted repository would leave an
// orphan blob that no GC pass can reach.
func (p *Pusher) CompleteSession(ctx context.Context, tokenID, sessionID string) (model.Blob, error) {
	session, err := p.sessions.Get(ctx, tokenID, sessionID)
	if err != nil {
		return model.Blob{}, err
	}
	if !p.repos.Alive(ctx, session.Repo) {
		_ = p.sessions.Abort(ctx, tokenID, sessionID)
		return model.Blob{}, repo.ErrRepositoryDeleted
	}
	return p.sessions.Complete(ctx, tokenID, sessionID)
}
