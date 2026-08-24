package model

// Manifest is the OCI-style image manifest kept by the registry. It lists the
// config object and the ordered layers that make up one artifact.
type Manifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType"`
	Config        Descriptor        `json:"config"`
	Layers        []Descriptor      `json:"layers"`
	Annotations   map[string]string `json:"annotations,omitempty"`
	Digest        string            `json:"-"`
}

// Descriptors returns the config descriptor followed by every layer.
func (m *Manifest) Descriptors() []Descriptor {
	out := make([]Descriptor, 0, 1+len(m.Layers))
	if m == nil {
		return out
	}
	out = append(out, m.Config)
	out = append(out, m.Layers...)
	return out
}

// Validate checks the structural requirements of a manifest.
func (m *Manifest) Validate() error {
	if m == nil {
		return ErrInvalidArgument
	}
	if m.SchemaVersion != 2 {
		return ErrInvalidArgument
	}
	if m.MediaType == "" {
		return ErrInvalidArgument
	}
	if err := m.Config.Validate(); err != nil {
		return err
	}
	if len(m.Layers) == 0 {
		return ErrInvalidArgument
	}
	for _, layer := range m.Layers {
		if err := layer.Validate(); err != nil {
			return err
		}
	}
	return nil
}
