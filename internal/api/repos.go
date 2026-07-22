package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"modernc.org/sqlite"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// repoDTO is the wire representation of a core.Repo (docs/API.md "Repos").
type repoDTO struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	SourcePath  string `json:"source_path"`
	DefaultBase string `json:"default_base"`
	CreatedAt   string `json:"created_at"`
}

// repoToDTO maps a core.Repo to its wire shape.
func repoToDTO(r core.Repo) repoDTO {
	return repoDTO{
		ID:          string(r.ID),
		ProjectID:   string(r.ProjectID),
		Name:        r.Name,
		SourcePath:  r.SourcePath,
		DefaultBase: r.DefaultBase,
		CreatedAt:   rfc3339(r.CreatedAt),
	}
}

// reposResponse is the GET /projects/{id}/repos body.
type reposResponse struct {
	Repos []repoDTO `json:"repos"`
}

// handleListRepos returns the repos registered on a project node.
func (h *Handlers) handleListRepos(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requireProject(w, r)
	if !ok {
		return
	}
	repos, err := h.store.ListRepos(r.Context(), projectID)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	out := make([]repoDTO, 0, len(repos))
	for _, repo := range repos {
		out = append(out, repoToDTO(repo))
	}
	writeJSON(w, h.logger, http.StatusOK, reposResponse{Repos: out})
}

// createRepoRequest is the POST /projects/{id}/repos body.
type createRepoRequest struct {
	Name        string `json:"name"`
	SourcePath  string `json:"source_path"`
	DefaultBase string `json:"default_base"`
}

// handleCreateRepo registers a git repo on a project node. Once a project has
// repos, task nodes created under it afterwards auto-provision one worktree per
// repo (see provisionTask); existing tasks are untouched.
func (h *Handlers) handleCreateRepo(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.requireProject(w, r)
	if !ok {
		return
	}
	var req createRepoRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}

	sourcePath := strings.TrimSpace(req.SourcePath)
	if !filepath.IsAbs(sourcePath) {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "source_path must be an absolute path")
		return
	}
	if info, err := os.Stat(sourcePath); err != nil || !info.IsDir() {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "source_path must be an existing directory")
		return
	}
	if isRepo, err := h.git.IsGitRepo(r.Context(), sourcePath); err != nil || !isRepo {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "source_path is not a git repository")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = filepath.Base(sourcePath)
	}
	if !validRepoName(name) {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "name must be a plain directory name (no slash, not '.' or '..')")
		return
	}

	repo := core.Repo{
		ID:          core.NewRepoID(),
		ProjectID:   projectID,
		Name:        name,
		SourcePath:  sourcePath,
		DefaultBase: strings.TrimSpace(req.DefaultBase),
		CreatedAt:   time.Now(),
	}
	if err := h.store.SaveRepo(r.Context(), repo); err != nil {
		if isUniqueViolation(err) {
			writeErrorStatus(w, h.logger, http.StatusConflict,
				fmt.Sprintf("a repo named %q is already registered on this project", name))
			return
		}
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusCreated, repoToDTO(repo))
}

// handleDeleteRepo removes a repo by id. Deletion is idempotent: an unknown id
// still returns 204. The store soft-deletes (the repo stops being listed and
// its name slot is freed), so task worktrees already cut from the repo keep a
// valid parent and are untouched.
func (h *Handlers) handleDeleteRepo(w http.ResponseWriter, r *http.Request) {
	id := core.RepoID(r.PathValue("id"))
	if id == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "repo id is required")
		return
	}
	if err := h.store.DeleteRepo(r.Context(), id); err != nil {
		writeError(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requireProject resolves the {id} path wildcard to a project node, writing a
// 404 (unknown node) or 400 (node exists but is not a project) and returning
// ok=false on failure. Repos live only on project nodes.
func (h *Handlers) requireProject(w http.ResponseWriter, r *http.Request) (core.NodeID, bool) {
	id := pathID(r)
	node, ok := h.tree.Get(id)
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusNotFound, "project not found")
		return "", false
	}
	if node.Kind != core.KindProject {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "node is not a project")
		return "", false
	}
	return id, true
}

// validRepoName mirrors core's rule for a name usable as a single workspace
// path element: non-empty, not "." or "..", free of path separators and NUL.
func validRepoName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`) && !strings.ContainsRune(name, 0)
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure,
// which the repos table raises on a duplicate (project_id, name). It mirrors
// the store test helper's detection so the handler maps it to 409 rather than
// leaking a generic 500.
func isUniqueViolation(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && strings.Contains(se.Error(), "UNIQUE")
}
