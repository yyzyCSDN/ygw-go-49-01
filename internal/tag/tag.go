package tag

import (
	"context"

	"artifactregistry/internal/model"
)

// Store maps tag names to manifest digests and keeps a per-manifest reference
// ledger. GC consumes both the tag table and the ledger; whenever the two
// disagree the registry must treat the ledger as suspect and recount from the
// tag table instead of collecting referenced manifests.
type Store interface {
	Set(ctx context.Context, repo, name, digest string) error
	Get(ctx context.Context, repo, name string) (string, error)
	Delete(ctx context.Context, repo, name string) error
	List(ctx context.Context, repo string) map[string]string
	Refs(ctx context.Context, repo string) map[string]int
	Revision(ctx context.Context, repo string) int64
}

// Tag is a lightweight value returned by store operations.
type Tag = model.Tag
