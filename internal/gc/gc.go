package gc

import (
	"context"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/model"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
)

// GC runs mark-and-sweep garbage collection for a repository. Mark collects
// every manifest reachable from tags and every blob those manifests reference;
// Sweep deletes unreferenced manifests and blobs that are no longer held by
// any in-flight upload.
type GC struct {
	repos     repo.Store
	tags      tag.Store
	manifests manifest.Store
	blobs     blob.Store
}

// NewGC wires the GC components together.
func NewGC(repos repo.Store, tags tag.Store, manifests manifest.Store, blobs blob.Store) *GC {
	return &GC{
		repos:     repos,
		tags:      tags,
		manifests: manifests,
		blobs:     blobs,
	}
}

// Run executes mark and sweep for a repository, including repositories that
// were deleted while pushes were in flight (their orphan blobs must be
// collectable).
func (g *GC) Run(ctx context.Context, repoName string) error {
	reachable, manifests, err := g.Mark(ctx, repoName)
	if err != nil {
		return err
	}
	return g.Sweep(ctx, repoName, reachable, manifests)
}

// RunOrphan reclaims an orphan namespace: one that was deleted while a push was
// still in flight. Such a namespace has no live tags, so Mark finds nothing
// reachable, but the blobs those pushes committed carry an upload reference no
// manifest will ever release. SweepUnreferenced honors that reference count and
// would leave the blobs behind, so an orphan is swept wholesale: every blob in
// the namespace is reclaimed regardless of reference count.
func (g *GC) RunOrphan(ctx context.Context, repoName string) error {
	if _, _, err := g.Mark(ctx, repoName); err != nil {
		return err
	}
	for _, digest := range g.manifests.List(ctx, repoName) {
		if err := g.deleteManifest(ctx, repoName, digest); err != nil {
			return err
		}
	}
	if _, err := blob.SweepNamespaceBlobs(ctx, g.blobs, repoName); err != nil {
		return err
	}
	return nil
}

// reachableManifests returns the set of manifest digests referenced by tags.
func (g *GC) reachableManifests(ctx context.Context, repoName string) (map[string]int, map[string]int) {
	tags := g.tags.List(ctx, repoName)
	counts := make(map[string]int, len(tags))
	for _, digest := range tags {
		counts[digest]++
	}
	ledger := g.tags.Refs(ctx, repoName)
	return counts, ledger
}

// deleteManifest removes a manifest and releases its blob references.
func (g *GC) deleteManifest(ctx context.Context, repoName, digest string) error {
	m, err := g.manifests.Get(ctx, repoName, digest)
	if err != nil {
		if err == model.ErrNotFound {
			return nil
		}
		return err
	}
	for _, descriptor := range m.Descriptors() {
		_ = g.blobs.DecRef(ctx, repoName, descriptor.Digest)
	}
	return g.manifests.Delete(ctx, repoName, digest)
}
