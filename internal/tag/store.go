package tag

import (
	"context"
	"sort"
	"sync"

	"artifactregistry/internal/model"
)

type memoryStore struct {
	mu       sync.RWMutex
	tags     map[string]map[string]string
	refs     map[string]map[string]int
	revision map[string]int64
}

// NewMemoryStore creates an in-memory tag store.
func NewMemoryStore() Store {
	return &memoryStore{
		tags:     make(map[string]map[string]string),
		refs:     make(map[string]map[string]int),
		revision: make(map[string]int64),
	}
}

// Set points a tag at a manifest digest and updates the reference ledger.
// Every tag name referencing a digest contributes exactly one reference; two
// tags pointing at the same digest must produce a ledger count of two.
func (s *memoryStore) Set(ctx context.Context, repo, name, digest string) error {
	if name == "" || digest == "" {
		return model.ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	repoTags := s.tags[repo]
	if repoTags == nil {
		repoTags = make(map[string]string)
		s.tags[repo] = repoTags
	}
	repoRefs := s.refs[repo]
	if repoRefs == nil {
		repoRefs = make(map[string]int)
		s.refs[repo] = repoRefs
	}
	old := repoTags[name]
	if old != "" && old != digest {
		repoRefs[old]--
		if repoRefs[old] <= 0 {
			delete(repoRefs, old)
		}
	}
	repoTags[name] = digest
	if old == digest {
		// Re-pointing to the same digest keeps the reference.
	} else if repoRefs[digest] == 0 {
		repoRefs[digest] = 1
	} else {
		repoRefs[digest]++
	}
	s.revision[repo]++
	return nil
}

func (s *memoryStore) Get(ctx context.Context, repo, name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	digest, ok := s.tags[repo][name]
	if !ok {
		return "", model.ErrNotFound
	}
	return digest, nil
}

func (s *memoryStore) Delete(ctx context.Context, repo, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	repoTags := s.tags[repo]
	repoRefs := s.refs[repo]
	digest, ok := repoTags[name]
	if !ok {
		return model.ErrNotFound
	}
	delete(repoTags, name)
	if repoRefs != nil {
		repoRefs[digest]--
		if repoRefs[digest] <= 0 {
			delete(repoRefs, digest)
		}
	}
	s.revision[repo]++
	return nil
}

func (s *memoryStore) List(ctx context.Context, repo string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.tags[repo]))
	for name, digest := range s.tags[repo] {
		out[name] = digest
	}
	return out
}

func (s *memoryStore) Refs(ctx context.Context, repo string) map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]int, len(s.refs[repo]))
	for digest, count := range s.refs[repo] {
		out[digest] = count
	}
	return out
}

func (s *memoryStore) Revision(ctx context.Context, repo string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision[repo]
}

// SortedNames returns tag names in deterministic order for listings.
func SortedNames(list map[string]string) []string {
	out := make([]string, 0, len(list))
	for name := range list {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
