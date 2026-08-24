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

// Delete soft-deletes a repository namespace. When pushes are still in flight
// the namespace is recorded as an orphan so a later GC pass can return to it
// and reclaim blobs those pushes committed after the deletion landed. Without
// this marker the namespace drops out of every GC scan and the blobs become
// permanent orphans that manual cleanup has to chase down again and again.
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
	if s.inflight[name] > 0 {
		s.orphans[name] = true
	}
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

// ForgetOrphan clears the orphan marker for a namespace after GC has reclaimed
// everything in it. A namespace stays an orphan from the moment it is deleted
// with pushes in flight until GC confirms it is empty, so concurrent pushes
// that commit into the deleted namespace between GC passes keep it listed.
// Once GC has swept the namespace clean the marker is dropped so the namespace
// does not stay on the orphan list forever and the manual-clear-stays-cleared
// property holds.
func (s *memoryStore) ForgetOrphan(ctx context.Context, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.orphans, name)
}
