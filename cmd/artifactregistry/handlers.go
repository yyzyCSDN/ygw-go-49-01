package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"artifactregistry/internal/model"
	"artifactregistry/internal/repo"
	"artifactregistry/internal/registry"
)

type api struct {
	reg        *registry.Registry
	browsePath string
}

func tokenOf(r *http.Request) string {
	return r.Header.Get("X-Token")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func statusFor(err error) int {
	switch err {
	case model.ErrNotFound, repo.ErrRepositoryDeleted:
		return http.StatusNotFound
	case model.ErrAlreadyExists, model.ErrConflict:
		return http.StatusConflict
	case model.ErrInvalidArgument, model.ErrInvalidState:
		return http.StatusBadRequest
	case model.ErrUnauthorized:
		return http.StatusUnauthorized
	case model.ErrUnfinished, model.ErrVerificationPending:
		return http.StatusPreconditionFailed
	case model.ErrCorrupt:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

func (a *api) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *api) browse(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, a.browsePath)
}

func (a *api) issueToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TTL int `json:"ttl"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	token, err := a.reg.IssueToken(r.Context(), req.TTL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, token)
}

func (a *api) createRepo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := a.reg.CreateRepo(r.Context(), req.Name)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *api) listRepos(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.reg.ListRepos(r.Context()))
}

func (a *api) deleteRepo(w http.ResponseWriter, r *http.Request) {
	repoName := r.PathValue("repo")
	if err := a.reg.DeleteRepo(r.Context(), repoName); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": repoName})
}

func (a *api) pushBlob(w http.ResponseWriter, r *http.Request) {
	repoName := r.PathValue("repo")
	data, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	stored, err := a.reg.PushBlob(r.Context(), tokenOf(r), repoName, data)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, stored)
}

func (a *api) pushManifest(w http.ResponseWriter, r *http.Request) {
	repoName := r.PathValue("repo")
	ref := r.PathValue("ref")
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reg.PushManifest(r.Context(), tokenOf(r), repoName, raw, ref); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"ref": ref})
}

func (a *api) pullManifest(w http.ResponseWriter, r *http.Request) {
	repoName := r.PathValue("repo")
	ref := r.PathValue("ref")
	m, err := a.reg.PullManifest(r.Context(), tokenOf(r), repoName, ref)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (a *api) pullBlob(w http.ResponseWriter, r *http.Request) {
	repoName := r.PathValue("repo")
	digest := r.PathValue("digest")
	data, err := a.reg.PullBlob(r.Context(), tokenOf(r), repoName, digest)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (a *api) createSession(w http.ResponseWriter, r *http.Request) {
	repoName := r.PathValue("repo")
	var req struct {
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		Chunk   int64  `json:"chunkSize"`
		Digest  string `json:"digest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	session, err := a.reg.CreateSession(r.Context(), tokenOf(r), repoName, req.Name, req.Size, req.Chunk, req.Digest)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (a *api) uploadChunk(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	index, err := strconv.Atoi(r.URL.Query().Get("index"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing index")
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reg.UploadChunk(r.Context(), tokenOf(r), sessionID, index, data); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"index": index})
}

func (a *api) sessionStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	session, err := a.reg.SessionStatus(r.Context(), tokenOf(r), sessionID)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	offset, err := a.reg.ResumeOffset(r.Context(), tokenOf(r), sessionID)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        session.ID,
		"state":     session.State.String(),
		"size":      session.Size,
		"written":   session.Written,
		"resumeAt":  offset,
	})
}

func (a *api) commitSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	stored, err := a.reg.CompleteSession(r.Context(), tokenOf(r), sessionID)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (a *api) runGC(w http.ResponseWriter, r *http.Request) {
	repoName := r.URL.Query().Get("repo")
	report, err := a.reg.RunGC(r.Context(), repoName)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

type repoView struct {
	Name      string   `json:"name"`
	Artifacts []string `json:"artifacts"`
	Tags      []string `json:"tags"`
}

func (a *api) reposJSON(w http.ResponseWriter, r *http.Request) {
	names := a.reg.ListRepos(r.Context())
	views := make([]repoView, 0, len(names))
	for _, name := range names {
		artifacts := make([]string, 0)
		for _, artifact := range a.reg.ListArtifacts(r.Context(), name) {
			artifacts = append(artifacts, artifact.Name)
		}
		tags := make([]string, 0, 8)
		for tagName := range a.reg.ListTags(r.Context(), name) {
			tags = append(tags, tagName)
		}
		views = append(views, repoView{Name: name, Artifacts: artifacts, Tags: tags})
	}
	writeJSON(w, http.StatusOK, map[string]any{"repositories": views, "count": len(views)})
}
