package blob

import (
	"context"

	"artifactregistry/internal/model"
)

// Store is the content-addressed blob layer. Blobs are stored once per digest
// and share one reference count across every manifest and tag that uses them.
// Put returns the stored blob after incrementing its reference count; callers
// that no longer need a blob must call DecRef so GC can reclaim it.
type Store interface {
	Put(ctx context.Context, repo string, data []byte) (model.Blob, error)
	Stage(ctx context.Context, repo, digest string, size int64) error
	Get(ctx context.Context, repo, digest string) ([]byte, error)
	Exists(ctx context.Context, repo, digest string) bool
	Committed(ctx context.Context, repo, digest string) bool
	RefCount(ctx context.Context, repo, digest string) int
	IncRef(ctx context.Context, repo, digest string) error
	DecRef(ctx context.Context, repo, digest string) error
	Delete(ctx context.Context, repo, digest string) error
	List(ctx context.Context, repo string) []string
}

// Sweeper removes blobs whose digest is absent from a reachable set. The
// implementation must synchronize the scan with concurrent Put calls so a blob
// committed while the reachable set was computed is never deleted.
type Sweeper interface {
	SweepUnreferenced(ctx context.Context, repo string, reachable map[string]bool) ([]string, error)
	// SweepNamespace reclaims every blob in a namespace regardless of its
	// reference count. It is reserved for namespaces that were deleted while
	// pushes were in flight: such a namespace has no live tags or manifests, so
	// the refcounts on its blobs are upload references that no manifest will
	// ever release. SweepUnreferenced would skip them forever.
	SweepNamespace(ctx context.Context, repo string) ([]string, error)
}
