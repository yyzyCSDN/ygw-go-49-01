package index

import (
	"context"
	"time"

	"artifactregistry/internal/model"
)

// Store keeps the repository index of published artifacts. An artifact may
// only enter the index after its blobs passed verification; publishing a
// manifest whose verification is pending or failed would hand clients a
// corrupt image.
type Store interface {
	Add(ctx context.Context, repo, name, digest string, ready bool) error
	Remove(ctx context.Context, repo, name string) error
	List(ctx context.Context, repo string) []model.Artifact
	Contains(ctx context.Context, repo, name string) bool
}

type memoryStore struct {
	artifacts map[string]map[string]*model.Artifact
}

// NewMemoryStore creates an in-memory index store.
func NewMemoryStore() Store {
	return &memoryStore{
		artifacts: make(map[string]map[string]*model.Artifact),
	}
}

// Add records a published artifact. The ready flag reports whether blob
// verification finished; an artifact that is not ready must not be listed.
func (s *memoryStore) Add(ctx context.Context, repo, name, digest string, ready bool) error {
	if name == "" || digest == "" {
		return model.ErrInvalidArgument
	}
	if !ready {
		return model.ErrVerificationPending
	}
	repoArtifacts := s.artifacts[repo]
	if repoArtifacts == nil {
		repoArtifacts = make(map[string]*model.Artifact)
		s.artifacts[repo] = repoArtifacts
	}
	existing := repoArtifacts[name]
	if existing == nil {
		existing = &model.Artifact{Repo: repo, Name: name, State: model.ArtifactDraft}
		repoArtifacts[name] = existing
	}
	existing.Digest = digest
	if existing.State != model.ArtifactPublished {
		return existing.Publish()
	}
	existing.UpdatedAt = time.Now()
	return nil
}

func (s *memoryStore) Remove(ctx context.Context, repo, name string) error {
	if _, ok := s.artifacts[repo][name]; !ok {
		return model.ErrNotFound
	}
	delete(s.artifacts[repo], name)
	return nil
}

func (s *memoryStore) List(ctx context.Context, repo string) []model.Artifact {
	out := make([]model.Artifact, 0, len(s.artifacts[repo]))
	for _, artifact := range s.artifacts[repo] {
		out = append(out, *artifact)
	}
	return out
}

func (s *memoryStore) Contains(ctx context.Context, repo, name string) bool {
	_, ok := s.artifacts[repo][name]
	return ok
}
