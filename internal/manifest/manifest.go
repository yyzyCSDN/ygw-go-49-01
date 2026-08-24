package manifest

import (
	"context"

	"artifactregistry/internal/model"
)

// BlobStatus is the subset of the blob store that manifest validation needs.
// Validation must only accept blobs that are committed, because a manifest
// that references an uploading or verifying layer publishes a broken image.
type BlobStatus interface {
	Exists(ctx context.Context, repo, digest string) bool
	Committed(ctx context.Context, repo, digest string) bool
	Get(ctx context.Context, repo, digest string) ([]byte, error)
}

// UploadState reports whether a digest is still being uploaded through an
// active chunked session. Manifest validation must treat such digests as
// unfinished even if a blob record exists for them.
type UploadState interface {
	IsDigestUploading(ctx context.Context, repo, digest string) bool
}

// Store keeps validated manifests per repository.
type Store interface {
	Store(ctx context.Context, repo string, m *model.Manifest) error
	Get(ctx context.Context, repo, digest string) (*model.Manifest, error)
	Exists(ctx context.Context, repo, digest string) bool
	Delete(ctx context.Context, repo, digest string) error
	List(ctx context.Context, repo string) []string
	Validate(ctx context.Context, repo string, m *model.Manifest) error
	Verify(ctx context.Context, repo string, m *model.Manifest) (bool, error)
}

type memoryStore struct {
	blobs     BlobStatus
	uploads   UploadState
	manifests map[string]map[string]*model.Manifest
}

// NewMemoryStore creates a manifest store backed by the given blob status
// provider and upload-state reporter.
func NewMemoryStore(blobs BlobStatus, uploads UploadState) Store {
	return &memoryStore{
		blobs:     blobs,
		uploads:   uploads,
		manifests: make(map[string]map[string]*model.Manifest),
	}
}

func (s *memoryStore) Store(ctx context.Context, repo string, m *model.Manifest) error {
	if m == nil || m.Digest == "" {
		return model.ErrInvalidArgument
	}
	if err := s.Validate(ctx, repo, m); err != nil {
		return err
	}
	repoManifests := s.manifests[repo]
	if repoManifests == nil {
		repoManifests = make(map[string]*model.Manifest)
		s.manifests[repo] = repoManifests
	}
	copied := *m
	repoManifests[m.Digest] = &copied
	return nil
}

func (s *memoryStore) Get(ctx context.Context, repo, digest string) (*model.Manifest, error) {
	m, ok := s.manifests[repo][digest]
	if !ok {
		return nil, model.ErrNotFound
	}
	copied := *m
	return &copied, nil
}

func (s *memoryStore) Exists(ctx context.Context, repo, digest string) bool {
	_, ok := s.manifests[repo][digest]
	return ok
}

func (s *memoryStore) Delete(ctx context.Context, repo, digest string) error {
	if _, ok := s.manifests[repo][digest]; !ok {
		return model.ErrNotFound
	}
	delete(s.manifests[repo], digest)
	return nil
}

func (s *memoryStore) List(ctx context.Context, repo string) []string {
	out := make([]string, 0, len(s.manifests[repo]))
	for digest := range s.manifests[repo] {
		out = append(out, digest)
	}
	return out
}
