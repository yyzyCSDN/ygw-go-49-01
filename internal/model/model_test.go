package model

import "testing"

func TestArtifactLifecycle(t *testing.T) {
	a := &Artifact{Repo: "r", Name: "n", Digest: "d", State: ArtifactDraft}
	if err := a.Publish(); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if a.State != ArtifactPublished {
		t.Fatalf("expected published, got %v", a.State)
	}
	if err := a.MarkGC(); err != nil {
		t.Fatalf("mark gc: %v", err)
	}
	if a.State != ArtifactGC {
		t.Fatalf("expected gc-eligible, got %v", a.State)
	}
	if err := a.Publish(); err == nil {
		t.Fatal("publish after gc-eligible must fail")
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := &UploadSession{State: SessionUploading}
	if err := s.Verify(); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if s.State != SessionVerifying {
		t.Fatalf("expected verifying, got %v", s.State)
	}
	if err := s.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if s.State != SessionCommitted {
		t.Fatalf("expected committed, got %v", s.State)
	}
	if err := s.Verify(); err == nil {
		t.Fatal("verify after committed must fail")
	}
}

func TestManifestValidateRejectsBadShape(t *testing.T) {
	m := &Manifest{
		SchemaVersion: 1,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config:        Descriptor{MediaType: "t", Digest: "sha256:1234567890abcdef", Size: 1},
		Layers:        []Descriptor{{MediaType: "t", Digest: "sha256:1234567890abcdef", Size: 1}},
	}
	if err := m.Validate(); err == nil {
		t.Fatal("schema version 1 must be rejected")
	}
	m.SchemaVersion = 2
	m.Layers = nil
	if err := m.Validate(); err == nil {
		t.Fatal("manifest without layers must be rejected")
	}
}

func TestBlobCommittedState(t *testing.T) {
	b := &Blob{Digest: "d", State: BlobCommitted}
	if !b.Committed() {
		t.Fatal("committed blob must report committed")
	}
	b.State = BlobVerifying
	if b.Committed() {
		t.Fatal("verifying blob must not report committed")
	}
}
