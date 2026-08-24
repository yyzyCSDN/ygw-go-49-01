package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"artifactregistry/internal/auth"
	"artifactregistry/internal/blob"
	"artifactregistry/internal/index"
	"artifactregistry/internal/manifest"
	"artifactregistry/internal/model"
	"artifactregistry/internal/registry"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/tag"
	"artifactregistry/internal/transfer"
)

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()
	repos := repo.NewMemoryStore()
	blobs := blob.NewMemoryStore(4)
	authStore := auth.NewMemoryStore()
	sessions := transfer.NewSessionManager(blobs, authStore, repos)
	manifests := manifest.NewMemoryStore(blobs, sessions)
	tags := tag.NewMemoryStore()
	indexStore := index.NewMemoryStore()
	reg := registry.New(repos, blobs, manifests, tags, indexStore, authStore, sessions)
	_, _ = reg.CreateRepo(ctx, "repo-a")
	token, _ := reg.IssueToken(ctx, 3600)
	browse := filepath.Join("..", "..", "web", "browse.html")
	server := httptest.NewServer(NewServer(reg, browse))
	t.Cleanup(server.Close)
	return server, token.ID
}

func doJSON(t *testing.T, method, url, token string, body any) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		if raw, ok := body.([]byte); ok {
			reader = bytes.NewReader(raw)
		} else {
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			reader = bytes.NewReader(raw)
		}
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if token != "" {
		request.Header.Set("X-Token", token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	payload, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	return response, payload
}

func TestHealthz(t *testing.T) {
	server, _ := newTestServer(t)
	response, payload := doJSON(t, http.MethodGet, server.URL+"/healthz", "", nil)
	if response.StatusCode != http.StatusOK || !strings.Contains(string(payload), "true") {
		t.Fatalf("healthz: %d %s", response.StatusCode, payload)
	}
}

func TestBrowsePage(t *testing.T) {
	server, _ := newTestServer(t)
	response, payload := doJSON(t, http.MethodGet, server.URL+"/", "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("browse status: %d", response.StatusCode)
	}
	if !strings.Contains(string(payload), "ArtifactRegistry") {
		t.Fatal("browse page must render the registry title")
	}
}

func TestRepoAPIAndBrowseData(t *testing.T) {
	server, tokenID := newTestServer(t)
	response, payload := doJSON(t, http.MethodPost, server.URL+"/v1/repos", "", map[string]string{"name": "repo-b"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create repo: %d %s", response.StatusCode, payload)
	}
	response, payload = doJSON(t, http.MethodPost, server.URL+"/v1/repos/repo-b/blobs", tokenID, []byte("layer-data"))
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("push blob: %d %s", response.StatusCode, payload)
	}
	var stored struct {
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	}
	_ = json.Unmarshal(payload, &stored)
	if stored.Digest == "" || stored.Size != 10 {
		t.Fatalf("unexpected blob record: %s", payload)
	}
	manifestPayload := model.Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config:        model.Descriptor{MediaType: "application/octet-stream", Digest: stored.Digest, Size: 10},
		Layers:        []model.Descriptor{{MediaType: "application/octet-stream", Digest: stored.Digest, Size: 10}},
	}
	response, payload = doJSON(t, http.MethodPost, server.URL+"/v1/repos/repo-b/manifests/v1", tokenID, manifestPayload)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("push manifest: %d %s", response.StatusCode, payload)
	}
	response, payload = doJSON(t, http.MethodGet, server.URL+"/api/repos", "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("repos json: %d %s", response.StatusCode, payload)
	}
	if !strings.Contains(string(payload), "repo-b") || !strings.Contains(string(payload), "v1") {
		t.Fatalf("browse data must include repo-b and v1: %s", payload)
	}
}

func TestGCAndSessionAPIs(t *testing.T) {
	server, tokenID := newTestServer(t)
	response, payload := doJSON(t, http.MethodPost, server.URL+"/v1/gc?repo=repo-a", "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("gc: %d %s", response.StatusCode, payload)
	}
	sessionBody := map[string]any{"name": "s.bin", "size": 6, "chunkSize": 3}
	response, payload = doJSON(t, http.MethodPost, server.URL+"/v1/repos/repo-a/uploads", tokenID, sessionBody)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create session: %d %s", response.StatusCode, payload)
	}
	var session struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(payload, &session)
	response, payload = doJSON(t, http.MethodPatch, server.URL+"/v1/repos/repo-a/uploads/"+session.ID+"?index=0", tokenID, []byte("abc"))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("chunk: %d %s", response.StatusCode, payload)
	}
	response, payload = doJSON(t, http.MethodGet, server.URL+"/v1/repos/repo-a/uploads/"+session.ID, tokenID, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status: %d %s", response.StatusCode, payload)
	}
	if !strings.Contains(string(payload), "resumeAt") {
		t.Fatalf("status payload must include resumeAt: %s", payload)
	}
}
