package repo

import (
	"context"
	"time"

	"artifactregistry/internal/model"
)

// Repository is one namespace in the registry. Deleting a repository is a soft
// delete: the namespace is marked deleted and in-flight uploads that commit
// afterwards must be refused so no orphan blob is left behind.
type Repository struct {
	Name      string
	CreatedAt time.Time
	DeletedAt *time.Time
}

// Deleted reports whether the repository was deleted.
func (r *Repository) Deleted() bool {
	return r != nil && r.DeletedAt != nil
}

// Store manages repository namespaces.
type Store interface {
	Create(ctx context.Context, name string) (Repository, error)
	Get(ctx context.Context, name string) (Repository, error)
	Delete(ctx context.Context, name string) error
	Exists(ctx context.Context, name string) bool
	Alive(ctx context.Context, name string) bool
	List(ctx context.Context) []string
	Orphans(ctx context.Context) []string
	ForgetOrphan(ctx context.Context, name string)
	Enter(ctx context.Context, repo string) (release func(), err error)
	Count(ctx context.Context, repo string) int
}

// ErrRepositoryDeleted is returned when an operation targets a deleted repo.
var ErrRepositoryDeleted = model.ErrNotFound
