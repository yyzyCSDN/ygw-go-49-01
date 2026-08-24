package model

// BlobState describes where a content-addressed blob is in its lifecycle.
// A blob starts as uploading while its bytes are arriving, moves to verifying
// while its digest is confirmed, and only becomes committed once the digest
// check passed. GC and manifest validation must never treat a blob that is
// still uploading or verifying as a complete layer.
type BlobState int

const (
	// BlobUploading means bytes are still arriving.
	BlobUploading BlobState = iota
	// BlobVerifying means all bytes arrived and the digest is being checked.
	BlobVerifying
	// BlobCommitted means the digest check passed and the blob is readable.
	BlobCommitted
)

// String returns a stable name for the state.
func (s BlobState) String() string {
	switch s {
	case BlobUploading:
		return "uploading"
	case BlobVerifying:
		return "verifying"
	case BlobCommitted:
		return "committed"
	default:
		return "unknown"
	}
}

// Blob is the metadata record for one content-addressed object.
type Blob struct {
	Digest   string
	Size     int64
	State    BlobState
	RefCount int
}

// Committed reports whether the blob passed verification and is readable.
func (b *Blob) Committed() bool {
	return b != nil && b.State == BlobCommitted
}

// Descriptor describes one content-addressed object referenced by a manifest.
type Descriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64   `json:"size"`
}

// Validate checks the basic shape of a descriptor.
func (d Descriptor) Validate() error {
	if d.MediaType == "" {
		return ErrInvalidArgument
	}
	if d.Digest == "" || len(d.Digest) < 16 {
		return ErrInvalidArgument
	}
	if d.Size < 0 {
		return ErrInvalidArgument
	}
	return nil
}
