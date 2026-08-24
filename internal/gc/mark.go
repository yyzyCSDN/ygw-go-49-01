package gc

import (
	"context"

	"artifactregistry/internal/model"
)

// Mark walks tags to manifests to blobs and returns the set of reachable blob
// digests plus the set of reachable manifest digests.
func (g *GC) Mark(ctx context.Context, repoName string) (map[string]bool, map[string]bool, error) {
	reachableBlobs := make(map[string]bool)
	reachableManifests := make(map[string]bool)
	tags := g.tags.List(ctx, repoName)
	for _, digest := range tags {
		if digest == "" {
			continue
		}
		m, err := g.manifests.Get(ctx, repoName, digest)
		if err != nil {
			if err == model.ErrNotFound {
				continue
			}
			return nil, nil, err
		}
		reachableManifests[digest] = true
		for _, descriptor := range m.Descriptors() {
			reachableBlobs[descriptor.Digest] = true
		}
	}
	return reachableBlobs, reachableManifests, nil
}

// markBlob is a helper to record a reachable blob.
func markBlob(reachable map[string]bool, digest string) {
	if digest != "" {
		reachable[digest] = true
	}
}
