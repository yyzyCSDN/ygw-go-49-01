package index

import (
	"context"
)

// UpdateIndex is the single entry point used by the push pipeline to publish
// an artifact. The ready flag must be computed from blob verification before
// this call so the index never lists an unverified artifact.
func UpdateIndex(ctx context.Context, store Store, repo, name, digest string, ready bool) error {
	return store.Add(ctx, repo, name, digest, ready)
}
