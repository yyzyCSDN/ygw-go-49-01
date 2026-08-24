package manifest

import (
	"context"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/model"
)

// Validate verifies that the manifest is structurally sound and that every
// referenced object is present and committed in the blob store. A manifest
// whose config or layers are still uploading must not be accepted.
func (s *memoryStore) Validate(ctx context.Context, repo string, m *model.Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	for _, descriptor := range m.Descriptors() {
		// Only a committed blob has its bytes stored and its digest verified.
		// A staged or uploading blob has a metadata record (so Exists is true)
		// but no readable content; accepting it would publish a broken image.
		if s.blobs.Committed(ctx, repo, descriptor.Digest) {
			continue
		}
		if s.uploads != nil && s.uploads.IsDigestUploading(ctx, repo, descriptor.Digest) {
			return model.ErrUnfinished
		}
		return model.ErrNotFound
	}
	return nil
}

// Verify re-reads every referenced blob and recomputes its digest. It returns
// false when any object is missing, unfinished or fails the digest check.
func (s *memoryStore) Verify(ctx context.Context, repo string, m *model.Manifest) (bool, error) {
	if err := m.Validate(); err != nil {
		return false, err
	}
	for _, descriptor := range m.Descriptors() {
		data, err := s.blobs.Get(ctx, repo, descriptor.Digest)
		if err != nil {
			return false, nil
		}
		if blob.DigestOf(data) != descriptor.Digest {
			return false, nil
		}
	}
	return true, nil
}
