package manifest

import (
	"context"
	"testing"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/model"
)

// fakeUploads is a minimal UploadState recording which digests are still in
// flight. It mirrors how SessionManager.IsDigestUploading should behave.
type fakeUploads struct {
	inFlight map[string]bool
}

func (f *fakeUploads) IsDigestUploading(_ context.Context, _ string, digest string) bool {
	return f.inFlight[digest]
}

// TestValidateRejectsStagedButUncommittedLayer reproduces the bug where a
// manifest referenced a layer that was staged (so a blob record existed and
// Exists returned true) but never completed (no bytes, not committed). Such a
// manifest must be rejected before it is stored, otherwise the push reports
// success while the published image is missing its layer bytes.
func TestValidateRejectsStagedButUncommittedLayer(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemoryStore(4)
	uploads := &fakeUploads{inFlight: map[string]bool{}}

	// A fully committed config layer that the manifest can reference safely.
	config := []byte("config")
	configBlob, _ := blobs.Put(ctx, "repo-a", config)

	// A staged, never-completed layer: Stage creates a verifying record with
	// no bytes, which is exactly the state of an upload that is still in
	// progress.
	layerDigest := "sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	uploads.inFlight[layerDigest] = true
	if err := blobs.Stage(ctx, "repo-a", layerDigest, 4); err != nil {
		t.Fatalf("stage: %v", err)
	}
	// Sanity: the staged layer exists as a record but is not committed, so it
	// has no readable bytes. This is the trap the old Exists-based check fell
	// into.
	if !blobs.Exists(ctx, "repo-a", layerDigest) {
		t.Fatal("staged layer must exist as a record")
	}
	if blobs.Committed(ctx, "repo-a", layerDigest) {
		t.Fatal("staged layer must not be committed")
	}

	store := NewMemoryStore(blobs, uploads)
	m, err := Parse(rawManifest(t, configBlob.Digest, layerDigest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Validate must reject the manifest because the layer is unfinished.
	if err := store.Validate(ctx, "repo-a", m); err != model.ErrUnfinished {
		t.Fatalf("validate: want ErrUnfinished, got %v", err)
	}
	// And Store must refuse to persist it for the same reason.
	if err := store.Store(ctx, "repo-a", m); err == nil {
		t.Fatal("store must reject manifest referencing an unfinished layer")
	}
}

// TestValidateRejectsMissingLayerWithoutUpload ensures that a layer that is
// neither committed nor in flight is classified as not found (client error),
// not silently accepted as finished.
func TestValidateRejectsMissingLayerWithoutUpload(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemoryStore(4)
	uploads := &fakeUploads{inFlight: map[string]bool{}}

	config := []byte("config")
	configBlob, _ := blobs.Put(ctx, "repo-a", config)

	missing := "sha256:c0ffeec0ffeec0ffeec0ffeec0ffeec0ffeec0ffeec0ffeec0ffeec0ffeeff"

	store := NewMemoryStore(blobs, uploads)
	m, err := Parse(rawManifest(t, configBlob.Digest, missing))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := store.Validate(ctx, "repo-a", m); err != model.ErrNotFound {
		t.Fatalf("validate: want ErrNotFound, got %v", err)
	}
}
