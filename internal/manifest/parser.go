package manifest

import (
	"encoding/json"

	"artifactregistry/internal/blob"
	"artifactregistry/internal/model"
)

// Parse decodes an OCI-style manifest payload and computes its content digest.
func Parse(raw []byte) (*model.Manifest, error) {
	var m model.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	m.Digest = blob.DigestOf(raw)
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}
