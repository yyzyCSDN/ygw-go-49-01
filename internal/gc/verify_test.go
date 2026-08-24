package gc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/model"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
)

// TestGCMarkKeepsLiveRefs runs GC while new blobs are being committed. The
// sweep must not delete blobs that were written while the mark set was being
// computed: they are in-flight uploads, not orphans.
func TestGCMarkKeepsLiveRefs(t *testing.T) {
	ctx := context.Background()
	repos := repo.NewMemoryStore()
	if _, err := repos.Create(ctx, "repo-a"); err != nil {
		t.Fatal(err)
	}
	blobs := blob.NewMemoryStore(4)
	manifests := manifest.NewMemoryStore(blobs, nil)
	tags := tag.NewMemoryStore()
	g := NewGC(repos, tags, manifests, blobs)

	config, _ := blobs.Put(ctx, "repo-a", []byte("cfg"))
	layer, _ := blobs.Put(ctx, "repo-a", []byte("lay"))
	payload := model.Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config:        model.Descriptor{MediaType: "application/octet-stream", Digest: config.Digest, Size: 3},
		Layers:        []model.Descriptor{{MediaType: "application/octet-stream", Digest: layer.Digest, Size: 3}},
	}
	raw, _ := json.Marshal(payload)
	m, err := manifest.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifests.Store(ctx, "repo-a", m); err != nil {
		t.Fatal(err)
	}
	if err := tags.Set(ctx, "repo-a", "v1", m.Digest); err != nil {
		t.Fatal(err)
	}

	const workers = 8
	var wg sync.WaitGroup
	gcDone := make(chan error, 1)
	go func() {
		gcDone <- g.Run(ctx, "repo-a")
	}()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("new-blob-%d", i))
			if _, err := blobs.Put(ctx, "repo-a", data); err != nil {
				t.Errorf("put: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if err := <-gcDone; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < workers; i++ {
		digest := blob.DigestOf([]byte(fmt.Sprintf("new-blob-%d", i)))
		if !blobs.Exists(ctx, "repo-a", digest) {
			t.Fatalf("blob committed during GC mark must survive, missing %s", digest)
		}
	}
}
