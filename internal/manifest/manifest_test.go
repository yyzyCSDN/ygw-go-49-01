package manifest

import (
	"context"
	"encoding/json"
	"testing"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/model"
)

func rawManifest(t *testing.T, configDigest string, layerDigests ...string) []byte {
	t.Helper()
	layers := make([]model.Descriptor, 0, len(layerDigests))
	for _, digest := range layerDigests {
		layers = append(layers, model.Descriptor{MediaType: "application/octet-stream", Digest: digest, Size: 1})
	}
	payload := model.Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config:        model.Descriptor{MediaType: "application/octet-stream", Digest: configDigest, Size: 1},
		Layers:        layers,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

func TestParseAndStoreCommittedManifest(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemoryStore(4)
	store := NewMemoryStore(blobs, nil)
	config := []byte("config")
	layer := []byte("layer")
	configBlob, _ := blobs.Put(ctx, "repo-a", config)
	layerBlob, _ := blobs.Put(ctx, "repo-a", layer)
	raw := rawManifest(t, configBlob.Digest, layerBlob.Digest)
	m, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := store.Store(ctx, "repo-a", m); err != nil {
		t.Fatalf("store: %v", err)
	}
	got, err := store.Get(ctx, "repo-a", m.Digest)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Digest != m.Digest || len(got.Layers) != 1 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	ready, err := store.Verify(ctx, "repo-a", m)
	if err != nil || !ready {
		t.Fatalf("verify: ready=%v err=%v", ready, err)
	}
}

func TestVerifyDetectsMissingBlob(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemoryStore(4)
	store := NewMemoryStore(blobs, nil)
	missing := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	raw := rawManifest(t, missing, missing)
	m, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ready, err := store.Verify(ctx, "repo-a", m)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ready {
		t.Fatal("verify must report not ready for missing blobs")
	}
	if err := store.Store(ctx, "repo-a", m); err == nil {
		t.Fatal("store must reject manifest with missing blobs")
	}
}

func TestParseRejectsMalformedPayload(t *testing.T) {
	if _, err := Parse([]byte("{not-json")); err == nil {
		t.Fatal("malformed payload must be rejected")
	}
	if _, err := Parse([]byte(`{"schemaVersion":2}`)); err == nil {
		t.Fatal("empty manifest must be rejected")
	}
}

func TestGetReturnsCopy(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemoryStore(4)
	store := NewMemoryStore(blobs, nil)
	c, _ := blobs.Put(ctx, "repo-a", []byte("c"))
	l, _ := blobs.Put(ctx, "repo-a", []byte("l"))
	m, _ := Parse(rawManifest(t, c.Digest, l.Digest))
	_ = store.Store(ctx, "repo-a", m)
	first, _ := store.Get(ctx, "repo-a", m.Digest)
	second, _ := store.Get(ctx, "repo-a", m.Digest)
	first.Annotations = map[string]string{"x": "y"}
	if len(second.Annotations) != 0 {
		t.Fatal("get must return independent copies")
	}
}

func TestDeleteRemovesManifest(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemoryStore(4)
	store := NewMemoryStore(blobs, nil)
	c, _ := blobs.Put(ctx, "repo-a", []byte("c"))
	l, _ := blobs.Put(ctx, "repo-a", []byte("l"))
	m, _ := Parse(rawManifest(t, c.Digest, l.Digest))
	_ = store.Store(ctx, "repo-a", m)
	if err := store.Delete(ctx, "repo-a", m.Digest); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if store.Exists(ctx, "repo-a", m.Digest) {
		t.Fatal("deleted manifest must not exist")
	}
}
