package repo

import (
	"context"
	"sort"
	"sync"
	"time"

	"artifactregistry/internal/model"
)

type memoryStore struct {
	mu       sync.RWMutex
	repos    map[string]Repository
	inflight map[string]int
	orphans  map[string]bool
}

// NewMemoryStore creates a repository namespace store.
func NewMemoryStore() Store {
	return &memoryStore{
		repos:    make(map[string]Repository),
		inflight: make(map[string]int),
		orphans:  make(map[string]bool),
	}
}

func (s *memoryStore) Create(ctx context.Context, name string) (Repository, error) {
	if name == "" {
		return Repository{}, model.ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.repos[name]; exists {
		return Repository{}, model.ErrAlreadyExists
	}
	r := Repository{Name: name, CreatedAt: time.Now()}
	s.repos[name] = r
	return r, nil
}

func (s *memoryStore) Get(ctx context.Context, name string) (Repository, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.repos[name]
	if !ok || r.Deleted() {
		return Repository{}, model.ErrNotFound
	}
	return r, nil
}

func (s *memoryStore) Delete(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.repos[name]
	if !ok {
		return model.ErrNotFound
	}
	now := time.Now()
	r.DeletedAt = &now
	s.repos[name] = r
	return nil
}

func (s *memoryStore) Exists(ctx context.Context, name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.repos[name]
	return ok
}

func (s *memoryStore) Alive(ctx context.Context, name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.repos[name]
	return ok && !r.Deleted()
}

func (s *memoryStore) List(ctx context.Context) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for name, r := range s.repos {
		if !r.Deleted() {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Enter records one in-flight push for a repository. If the repository was
// already deleted the push is refused before it starts.
func (s *memoryStore) Enter(ctx context.Context, repo string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.repos[repo]
	if !ok || r.Deleted() {
		return nil, ErrRepositoryDeleted
	}
	s.inflight[repo]++
	var once sync.Once
	release := func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.inflight[repo] > 0 {
				s.inflight[repo]--
			}
		})
	}
	return release, nil
}

// Count returns the number of in-flight pushes for a repository.
func (s *memoryStore) Count(ctx context.Context, repo string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inflight[repo]
}

// Orphans returns the namespaces that were deleted while a push was still in
// flight. GC must scan these namespaces so blobs uploaded into them before the
// deletion landed do not become permanent orphans.
func (s *memoryStore) Orphans(ctx context.Context) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.orphans))
	for name := range s.orphans {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
