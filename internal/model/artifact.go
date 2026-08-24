package model

import "time"

// ArtifactState is the artifact lifecycle: a pushed artifact starts as a
// draft, becomes published when its blobs are verified and its tag/index
// entries are committed, and finally becomes gc-eligible after the registry
// decided it is no longer reachable.
type ArtifactState int

const (
	// ArtifactDraft is the initial state before verification finished.
	ArtifactDraft ArtifactState = iota
	// ArtifactPublished means the artifact is listed and pullable.
	ArtifactPublished
	// ArtifactGC means the artifact was unreferenced and can be collected.
	ArtifactGC
)

// String returns a stable name for the artifact state.
func (s ArtifactState) String() string {
	switch s {
	case ArtifactDraft:
		return "draft"
	case ArtifactPublished:
		return "published"
	case ArtifactGC:
		return "gc-eligible"
	default:
		return "unknown"
	}
}

// Artifact is the repository-level record for one published name.
type Artifact struct {
	Repo      string
	Name      string
	Digest    string
	State     ArtifactState
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Publish advances a draft artifact to published.
func (a *Artifact) Publish() error {
	if a == nil {
		return ErrInvalidArgument
	}
	if a.State != ArtifactDraft {
		return ErrInvalidState
	}
	a.State = ArtifactPublished
	a.UpdatedAt = time.Now()
	return nil
}

// MarkGC advances a published artifact to gc-eligible.
func (a *Artifact) MarkGC() error {
	if a == nil {
		return ErrInvalidArgument
	}
	if a.State != ArtifactPublished {
		return ErrInvalidState
	}
	a.State = ArtifactGC
	a.UpdatedAt = time.Now()
	return nil
}
