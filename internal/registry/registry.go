package registry

import (
	"context"
	"time"

	"artifactregistry/internal/auth"
	"artifactregistry/internal/blob"
	"artifactregistry/internal/gc"
	"artifactregistry/internal/index"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/model"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
	"artifactregistry/internal/transfer"
)

// Registry is the top-level facade used by the HTTP layer. It wires the blob,
// manifest, tag, index, transfer, auth and GC components into one push-pull
// pipeline.
type Registry struct {
	repos     repo.Store
	blobs     blob.Store
	manifests manifest.Store
	tags      tag.Store
	index     index.Store
	auth      auth.Store
	sessions  *transfer.SessionManager
	pusher    *transfer.Pusher
	gc        *gc.GC
}

// New builds a registry from its component stores.
func New(
	repos repo.Store,
	blobs blob.Store,
	manifests manifest.Store,
	tags tag.Store,
	indexStore index.Store,
	authStore auth.Store,
	sessions *transfer.SessionManager,
) *Registry {
	pusher := transfer.NewPusher(authStore, blobs, manifests, tags, indexStore, sessions, repos)
	return &Registry{
		repos:     repos,
		blobs:     blobs,
		manifests: manifests,
		tags:      tags,
		index:     indexStore,
		auth:      authStore,
		sessions:  sessions,
		pusher:    pusher,
		gc:        gc.NewGC(repos, tags, manifests, blobs),
	}
}

// CreateRepo creates a repository namespace.
func (r *Registry) CreateRepo(ctx context.Context, name string) (repo.Repository, error) {
	return r.repos.Create(ctx, name)
}

// DeleteRepo deletes a repository namespace.
func (r *Registry) DeleteRepo(ctx context.Context, name string) error {
	return r.repos.Delete(ctx, name)
}

// ListRepos returns live repository names.
func (r *Registry) ListRepos(ctx context.Context) []string {
	return r.repos.List(ctx)
}

// OrphanNamespaces returns deleted namespaces with in-flight pushes.
func (r *Registry) OrphanNamespaces(ctx context.Context) []string {
	return r.repos.Orphans(ctx)
}

// IssueToken issues a push token.
func (r *Registry) IssueToken(ctx context.Context, ttlSeconds int) (auth.Token, error) {
	ttl := secondsToDuration(ttlSeconds)
	return r.auth.Issue(ctx, "", ttl)
}

// PushBlob uploads one blob.
func (r *Registry) PushBlob(ctx context.Context, tokenID, repoName string, data []byte) (model.Blob, error) {
	return r.pusher.PushBlob(ctx, tokenID, repoName, data)
}

// PushManifest publishes a manifest under a tag.
func (r *Registry) PushManifest(ctx context.Context, tokenID, repoName string, raw []byte, ref string) error {
	return r.pusher.PushManifest(ctx, tokenID, repoName, raw, ref)
}

// CreateSession opens a chunked upload.
func (r *Registry) CreateSession(ctx context.Context, tokenID, repoName, name string, size, chunkSize int64, expectedDigest string) (model.UploadSession, error) {
	return r.pusher.CreateSession(ctx, tokenID, repoName, name, size, chunkSize, expectedDigest)
}

// UploadChunk stores one chunk.
func (r *Registry) UploadChunk(ctx context.Context, tokenID, sessionID string, index int, data []byte) error {
	return r.pusher.UploadChunk(ctx, tokenID, sessionID, index, data)
}

// ResumeOffset returns the resume offset of a session.
func (r *Registry) ResumeOffset(ctx context.Context, tokenID, sessionID string) (int64, error) {
	return r.pusher.ResumeOffset(ctx, tokenID, sessionID)
}

// SessionStatus returns a session snapshot for status queries.
func (r *Registry) SessionStatus(ctx context.Context, tokenID, sessionID string) (model.UploadSession, error) {
	return r.sessions.Get(ctx, tokenID, sessionID)
}

// CompleteSession commits a chunked upload.
func (r *Registry) CompleteSession(ctx context.Context, tokenID, sessionID string) (model.Blob, error) {
	return r.pusher.CompleteSession(ctx, tokenID, sessionID)
}

// PullManifest resolves a tag to its manifest.
func (r *Registry) PullManifest(ctx context.Context, tokenID, repoName, ref string) (*model.Manifest, error) {
	return r.pusher.PullManifest(ctx, tokenID, repoName, ref)
}

// PullBlob reads and verifies one blob.
func (r *Registry) PullBlob(ctx context.Context, tokenID, repoName, digest string) ([]byte, error) {
	return r.pusher.PullBlob(ctx, tokenID, repoName, digest)
}

// ListArtifacts returns the published artifacts of a repository.
func (r *Registry) ListArtifacts(ctx context.Context, repoName string) []model.Artifact {
	return r.index.List(ctx, repoName)
}

// ListTags returns the tag table of a repository.
func (r *Registry) ListTags(ctx context.Context, repoName string) map[string]string {
	return r.tags.List(ctx, repoName)
}

// GCReport summarizes one garbage collection run.
type GCReport struct {
	Namespaces []string
}

// RunGC runs garbage collection on the requested repository plus every
// orphaned namespace so blobs left behind by interrupted pushes are reclaimed.
func (r *Registry) RunGC(ctx context.Context, repoName string) (GCReport, error) {
	namespaces := []string{repoName}
	namespaces = append(namespaces, r.repos.Orphans(ctx)...)
	for _, ns := range namespaces {
		if err := r.gc.Run(ctx, ns); err != nil {
			return GCReport{}, err
		}
	}
	return GCReport{Namespaces: namespaces}, nil
}

// secondsToDuration converts seconds to a duration, defaulting to 3600.
func secondsToDuration(seconds int) (d time.Duration) {
	if seconds <= 0 {
		seconds = 3600
	}
	return time.Duration(seconds) * time.Second
}
