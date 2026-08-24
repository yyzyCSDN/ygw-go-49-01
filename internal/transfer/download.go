package transfer

import (
	"context"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/model"
)

// PullManifest resolves a tag to a manifest.
func (p *Pusher) PullManifest(ctx context.Context, tokenID, repoName, ref string) (*model.Manifest, error) {
	if _, err := p.auth.Validate(ctx, tokenID); err != nil {
		return nil, err
	}
	digest, err := p.tags.Get(ctx, repoName, ref)
	if err != nil {
		return nil, err
	}
	return p.manifests.Get(ctx, repoName, digest)
}

// PullBlob reads one layer and verifies its digest before returning it.
func (p *Pusher) PullBlob(ctx context.Context, tokenID, repoName, digest string) ([]byte, error) {
	if _, err := p.auth.Validate(ctx, tokenID); err != nil {
		return nil, err
	}
	data, err := p.blobs.Get(ctx, repoName, digest)
	if err != nil {
		return nil, err
	}
	if blob.DigestOf(data) != digest {
		return nil, model.ErrCorrupt
	}
	return data, nil
}

// FetchArtifact downloads every layer of an artifact and verifies the whole
// payload. It is used by the pull API and by the browse page probe.
func (p *Pusher) FetchArtifact(ctx context.Context, tokenID, repoName, ref string) (map[string][]byte, error) {
	m, err := p.PullManifest(ctx, tokenID, repoName, ref)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, 1+len(m.Layers))
	for _, descriptor := range m.Descriptors() {
		data, err := p.PullBlob(ctx, tokenID, repoName, descriptor.Digest)
		if err != nil {
			return nil, err
		}
		out[descriptor.Digest] = data
	}
	return out, nil
}
