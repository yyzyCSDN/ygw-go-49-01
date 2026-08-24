package manifest

import (
	"context"
	"encoding/json"
	"testing"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/model"
)

type uploadStateStub struct {
	uploading map[string]bool
}

func (u uploadStateStub) IsDigestUploading(ctx context.Context, repo, digest string) bool {
	return u.uploading[digest]
}

// TestManifestRefersUploadedBlob stores a manifest whose config and layer are
// still staged in an upload session. Manifest validation must refuse to
// publish a manifest that references unfinished layers.
func TestManifestRefersUploadedBlob(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemoryStore(4)
	digest := "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if err := blobs.Stage(ctx, "repo-a", digest, 1024); err != nil {
		t.Fatal(err)
	}
	store := NewMemoryStore(blobs, uploadStateStub{uploading: map[string]bool{digest: true}})
	payload := model.Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config:        model.Descriptor{MediaType: "application/octet-stream", Digest: digest, Size: 1024},
		Layers:        []model.Descriptor{{MediaType: "application/octet-stream", Digest: digest, Size: 1024}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Store(ctx, "repo-a", m); err == nil {
		t.Fatal("manifest referencing an unfinished blob must be rejected")
	}
}
