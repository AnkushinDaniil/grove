package api

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// postRepo registers a repo on a project via POST /projects/{id}/repos.
func (h *harness) postRepo(projectID string, body map[string]any) response {
	h.t.Helper()
	return h.do(http.MethodPost, "/api/v1/projects/"+projectID+"/repos", body)
}

// listRepos reads GET /projects/{id}/repos.
func (h *harness) listRepos(projectID string) reposResponse {
	h.t.Helper()
	var out reposResponse
	h.decode(h.do(http.MethodGet, "/api/v1/projects/"+projectID+"/repos", nil), http.StatusOK, &out)
	return out
}

func TestListReposOnProject(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	if repos := h.listRepos(project.ID).Repos; len(repos) != 0 {
		t.Fatalf("repos = %+v, want none initially", repos)
	}

	var created repoDTO
	h.decode(h.postRepo(project.ID, map[string]any{
		"source_path": newGitRepo(t), "name": "grove",
	}), http.StatusCreated, &created)
	if created.Name != "grove" || created.ProjectID != project.ID {
		t.Fatalf("created = %+v, want name grove on project %s", created, project.ID)
	}

	listed := h.listRepos(project.ID).Repos
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("repos = %+v, want just %s", listed, created.ID)
	}
}

func TestCreateRepoDefaultsNameToBasename(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	dir := newGitRepo(t)
	var created repoDTO
	h.decode(h.postRepo(project.ID, map[string]any{"source_path": dir}), http.StatusCreated, &created)
	if created.Name != filepath.Base(dir) {
		t.Fatalf("name = %q, want basename %q", created.Name, filepath.Base(dir))
	}
}

// TestCreateRepoProvisionsWorktreeOnNewTask exercises the whole point of repos:
// once a project owns one, a task created under it afterwards auto-provisions a
// worktree (the existing provisionTask path, driven end to end).
func TestCreateRepoProvisionsWorktreeOnNewTask(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	h.decode(h.postRepo(project.ID, map[string]any{"source_path": newGitRepo(t)}), http.StatusCreated, nil)

	task := h.createNode(core.NodeID(project.ID), core.KindTask, "Task", "")
	if task.WorkspaceDir == "" {
		t.Fatal("task workspace_dir empty, want a provisioned worktree workspace")
	}
	wts, err := h.store.ListWorktrees(t.Context(), core.NodeID(task.ID))
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 1 || wts[0].Status != core.WorktreeActive {
		t.Fatalf("worktrees = %+v, want one active", wts)
	}
}

func TestCreateRepoRejectsNonGitPath(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	// An existing directory that is not a git work tree.
	h.decode(h.postRepo(project.ID, map[string]any{"source_path": t.TempDir()}), http.StatusBadRequest, nil)

	// A relative path is rejected before the git check.
	h.decode(h.postRepo(project.ID, map[string]any{"source_path": "relative/repo"}), http.StatusBadRequest, nil)

	// A nonexistent absolute path.
	h.decode(h.postRepo(project.ID, map[string]any{
		"source_path": filepath.Join(t.TempDir(), "does-not-exist"),
	}), http.StatusBadRequest, nil)
}

func TestCreateRepoRejectsInvalidName(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	// A name with a path separator is not a plain directory name -> 400.
	h.decode(h.postRepo(project.ID, map[string]any{
		"source_path": newGitRepo(t), "name": "nested/repo",
	}), http.StatusBadRequest, nil)
}

func TestCreateRepoRejectsDuplicateName(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	h.decode(h.postRepo(project.ID, map[string]any{
		"source_path": newGitRepo(t), "name": "dup",
	}), http.StatusCreated, nil)
	// Same (project, name) again -> 409, even from a different source path.
	h.decode(h.postRepo(project.ID, map[string]any{
		"source_path": newGitRepo(t), "name": "dup",
	}), http.StatusConflict, nil)
}

func TestCreateRepoRejectsNonProjectNode(t *testing.T) {
	h := newHarness(t, nil)

	// The root is a workspace, not a project.
	h.decode(h.postRepo(string(h.root.ID), map[string]any{"source_path": newGitRepo(t)}), http.StatusBadRequest, nil)

	// A task node is not a project either.
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "Task", "fake")
	h.decode(h.postRepo(task.ID, map[string]any{"source_path": newGitRepo(t)}), http.StatusBadRequest, nil)

	// An unknown node id -> 404.
	h.decode(h.postRepo("does-not-exist", map[string]any{"source_path": newGitRepo(t)}), http.StatusNotFound, nil)
}

// TestDeleteRepoInUseConflicts documents current behavior: a repo whose id is
// still referenced by a task's worktree row cannot be hard-deleted while the
// worktrees.repo_id foreign key is enforced, so the handler returns 409 instead
// of leaking a 500. This is expected to relax to 204 once the store supports
// removing an in-use repo (soft-delete or ON DELETE CASCADE) per the contract's
// "removing repos ... existing task worktrees are untouched".
func TestDeleteRepoInUseConflicts(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	var created repoDTO
	h.decode(h.postRepo(project.ID, map[string]any{"source_path": newGitRepo(t)}), http.StatusCreated, &created)

	// Creating a task provisions a worktree that references the repo.
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "Task", "")
	if task.WorkspaceDir == "" {
		t.Fatal("expected a provisioned worktree to reference the repo")
	}

	h.decode(h.do(http.MethodDelete, "/api/v1/repos/"+created.ID, nil), http.StatusConflict, nil)
}

func TestDeleteRepoIsIdempotent(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")

	var created repoDTO
	h.decode(h.postRepo(project.ID, map[string]any{
		"source_path": newGitRepo(t), "name": "grove",
	}), http.StatusCreated, &created)

	h.decode(h.do(http.MethodDelete, "/api/v1/repos/"+created.ID, nil), http.StatusNoContent, nil)
	// Deleting again is not an error.
	h.decode(h.do(http.MethodDelete, "/api/v1/repos/"+created.ID, nil), http.StatusNoContent, nil)

	if repos := h.listRepos(project.ID).Repos; len(repos) != 0 {
		t.Fatalf("repos after delete = %+v, want none", repos)
	}
}
