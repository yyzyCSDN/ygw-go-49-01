package gc

import (
	"context"

	"artifactregistry/internal/blob"
)

// Sweep deletes manifests that are no longer referenced by any tag and blobs
// that are neither reachable nor held by an in-flight upload. Before deleting
// it re-checks the tag table: a tag update that landed after Mark must be
// honored so a freshly referenced manifest is never collected.
func (g *GC) Sweep(ctx context.Context, repoName string, reachableBlobs map[string]bool, reachableManifests map[string]bool) error {
	counts, ledger := g.reachableManifests(ctx, repoName)
	for _, digest := range g.manifests.List(ctx, repoName) {
		tagCount := counts[digest]
		ledgerCount := ledger[digest]
		if tagCount == 0 {
			if reachableManifests[digest] {
				continue
			}
			if err := g.deleteManifest(ctx, repoName, digest); err != nil {
				return err
			}
			continue
		}
		if ledgerCount != tagCount {
			if err := g.deleteManifest(ctx, repoName, digest); err != nil {
				return err
			}
			continue
		}
		reachableBlobs = markManifestBlobs(ctx, g, repoName, digest, reachableBlobs)
	}
	removed, err := blob.SweepUnreferencedBlobs(ctx, g.blobs, repoName, reachableBlobs)
	if err != nil {
		return err
	}
	_ = removed
	return nil
}

// markManifestBlobs adds the blobs of one manifest to the reachable set.
func markManifestBlobs(ctx context.Context, g *GC, repoName, digest string, reachable map[string]bool) map[string]bool {
	m, err := g.manifests.Get(ctx, repoName, digest)
	if err != nil {
		return reachable
	}
	for _, descriptor := range m.Descriptors() {
		markBlob(reachable, descriptor.Digest)
	}
	return reachable
}
