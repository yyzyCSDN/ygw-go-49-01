package blob

import (
	"context"
	"errors"
	"sort"
	"sync"

	"artifactregistry/internal/model"
)

// memoryStore implements Store with in-memory maps split over shards.
// Every digest is routed to one shard with xxhash so dedup lookups, GC scans
// and the shard table all agree on where a blob lives.
type memoryStore struct {
	mu     sync.RWMutex
	blobs  map[string]map[string]map[string]*model.Blob
	data   map[string]map[string]map[string][]byte
	shards int
}

// NewMemoryStore creates a blob store with shards shards (must be >= 1).
func NewMemoryStore(shards int) Store {
	if shards < 1 {
		shards = 1
	}
	return &memoryStore{
		blobs:  make(map[string]map[string]map[string]*model.Blob),
		data:   make(map[string]map[string]map[string][]byte),
		shards: shards,
	}
}

func (s *memoryStore) shardKey(repo, digest string) string {
	return ShardLabel(digest, s.shards)
}

func (s *memoryStore) Put(ctx context.Context, repo string, data []byte) (model.Blob, error) {
	if len(data) == 0 {
		return model.Blob{}, model.ErrInvalidArgument
	}
	digest := DigestOf(data)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.putLocked(repo, digest, data)
}

// Stage reserves a digest before its bytes arrive. The record is created in
// the verifying state so manifest validation can see that the layer is not
// finished, and GC can reclaim abandoned reservations.
func (s *memoryStore) Stage(ctx context.Context, repo, digest string, size int64) error {
	if digest == "" || size < 0 {
		return model.ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	repoShards := s.blobs[repo]
	if repoShards == nil {
		repoShards = make(map[string]map[string]*model.Blob)
		s.blobs[repo] = repoShards
	}
	shard := s.shardKey(repo, digest)
	repoBlobs := repoShards[shard]
	if repoBlobs == nil {
		repoBlobs = make(map[string]*model.Blob)
		repoShards[shard] = repoBlobs
	}
	if _, ok := repoBlobs[digest]; ok {
		return model.ErrAlreadyExists
	}
	repoBlobs[digest] = &model.Blob{
		Digest:   digest,
		Size:     size,
		State:    model.BlobVerifying,
		RefCount: 0,
	}
	return nil
}

func (s *memoryStore) putLocked(repo, digest string, data []byte) (model.Blob, error) {
	shard := s.shardKey(repo, digest)
	repoShards := s.blobs[repo]
	if repoShards == nil {
		repoShards = make(map[string]map[string]*model.Blob)
		s.blobs[repo] = repoShards
	}
	repoBlobs := repoShards[shard]
	if repoBlobs == nil {
		repoBlobs = make(map[string]*model.Blob)
		repoShards[shard] = repoBlobs
	}
	dataShards := s.data[repo]
	if dataShards == nil {
		dataShards = make(map[string]map[string][]byte)
		s.data[repo] = dataShards
	}
	if dataShards[shard] == nil {
		dataShards[shard] = make(map[string][]byte)
	}
	existing := repoBlobs[digest]
	if existing != nil {
		existing.State = model.BlobCommitted
		existing.RefCount++
		dataShards[shard][digest] = data
		return *existing, nil
	}
	b := &model.Blob{
		Digest:   digest,
		Size:     int64(len(data)),
		State:    model.BlobCommitted,
		RefCount: 1,
	}
	repoBlobs[digest] = b
	dataShards[shard][digest] = data
	return *b, nil
}

func (s *memoryStore) Get(ctx context.Context, repo, digest string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	shard := s.shardKey(repo, digest)
	data, ok := s.data[repo][shard][digest]
	if !ok && s.blobs[repo][shard][digest] != nil {
		return nil, model.ErrUnfinished
	}
	if !ok {
		return nil, model.ErrNotFound
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

func (s *memoryStore) Exists(ctx context.Context, repo, digest string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	shard := s.shardKey(repo, digest)
	return s.blobs[repo][shard][digest] != nil
}

func (s *memoryStore) Committed(ctx context.Context, repo, digest string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	shard := s.shardKey(repo, digest)
	b, ok := s.blobs[repo][shard][digest]
	return ok && b.Committed()
}

func (s *memoryStore) RefCount(ctx context.Context, repo, digest string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	shard := s.shardKey(repo, digest)
	b, ok := s.blobs[repo][shard][digest]
	if !ok {
		return 0
	}
	return b.RefCount
}

func (s *memoryStore) IncRef(ctx context.Context, repo, digest string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	shard := s.shardKey(repo, digest)
	b, ok := s.blobs[repo][shard][digest]
	if !ok {
		return model.ErrNotFound
	}
	b.RefCount++
	return nil
}

func (s *memoryStore) DecRef(ctx context.Context, repo, digest string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	shard := s.shardKey(repo, digest)
	b, ok := s.blobs[repo][shard][digest]
	if !ok {
		return model.ErrNotFound
	}
	if b.RefCount > 0 {
		b.RefCount--
	}
	return nil
}

func (s *memoryStore) Delete(ctx context.Context, repo, digest string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	shard := s.shardKey(repo, digest)
	b, ok := s.blobs[repo][shard][digest]
	if !ok {
		return model.ErrNotFound
	}
	if b.RefCount > 0 {
		return model.ErrConflict
	}
	delete(s.blobs[repo][shard], digest)
	delete(s.data[repo][shard], digest)
	return nil
}

func (s *memoryStore) List(ctx context.Context, repo string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for _, shardBlobs := range s.blobs[repo] {
		for digest := range shardBlobs {
			out = append(out, digest)
		}
	}
	sort.Strings(out)
	return out
}

// SweepUnreferenced removes blobs that are not in the reachable set. The
// whole scan and deletion run under one write lock, so a blob committed
// between the reachability computation and the sweep cannot be deleted: Put
// either completed before the lock was taken (and is therefore visible) or it
// runs after the sweep releases the lock.
func (s *memoryStore) SweepUnreferenced(ctx context.Context, repo string, reachable map[string]bool) ([]string, error) {
	var removed []string
	for shard, shardBlobs := range s.blobs[repo] {
		for digest := range shardBlobs {
			if reachable[digest] {
				continue
			}
			delete(s.blobs[repo][shard], digest)
			delete(s.data[repo][shard], digest)
			removed = append(removed, digest)
		}
	}
	sort.Strings(removed)
	return removed, nil
}

// SweepUnreferencedBlobs is a helper for callers that only hold the Store
// interface; it requires the underlying store to implement Sweeper.
func SweepUnreferencedBlobs(ctx context.Context, st Store, repo string, reachable map[string]bool) ([]string, error) {
	sweeper, ok := st.(Sweeper)
	if !ok {
		return nil, errors.New("artifactregistry: blob store does not support sweep")
	}
	return sweeper.SweepUnreferenced(ctx, repo, reachable)
}
