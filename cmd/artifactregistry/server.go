package main

import (
	"net/http"

	"artifactregistry/internal/registry"
)

// NewServer builds the HTTP handler for the registry API and the browse page.
func NewServer(reg *registry.Registry, browsePath string) http.Handler {
	mux := http.NewServeMux()
	api := &api{reg: reg, browsePath: browsePath}
	mux.HandleFunc("GET /healthz", api.healthz)
	mux.HandleFunc("GET /{$}", api.browse)
	mux.HandleFunc("POST /v1/token", api.issueToken)
	mux.HandleFunc("POST /v1/repos", api.createRepo)
	mux.HandleFunc("GET /v1/repos", api.listRepos)
	mux.HandleFunc("DELETE /v1/repos/{repo}", api.deleteRepo)
	mux.HandleFunc("POST /v1/repos/{repo}/blobs", api.pushBlob)
	mux.HandleFunc("POST /v1/repos/{repo}/manifests/{ref}", api.pushManifest)
	mux.HandleFunc("GET /v1/repos/{repo}/manifests/{ref}", api.pullManifest)
	mux.HandleFunc("GET /v1/repos/{repo}/blobs/{digest}", api.pullBlob)
	mux.HandleFunc("POST /v1/repos/{repo}/uploads", api.createSession)
	mux.HandleFunc("PATCH /v1/repos/{repo}/uploads/{id}", api.uploadChunk)
	mux.HandleFunc("GET /v1/repos/{repo}/uploads/{id}", api.sessionStatus)
	mux.HandleFunc("POST /v1/repos/{repo}/uploads/{id}/commit", api.commitSession)
	mux.HandleFunc("POST /v1/gc", api.runGC)
	mux.HandleFunc("GET /api/repos", api.reposJSON)
	return mux
}
