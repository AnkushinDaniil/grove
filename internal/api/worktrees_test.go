package api

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func TestCreateTaskProvisionsWorktrees(t *testing.T) {
	h := newHarness(t, nil)
	project := h.projectWithRepo("repo")

	task := h.createNode(core.NodeID(project.ID), core.KindTask, "Task", "")

	if task.WorkspaceDir == "" {
		t.Fatal("workspace_dir empty, want a provisioned workspace")
	}
	if fi, err := os.Stat(task.WorkspaceDir); err != nil || !fi.IsDir() {
		t.Fatalf("workspace dir %q missing: %v", task.WorkspaceDir, err)
	}

	wts, err := h.store.ListWorktrees(t.Context(), core.NodeID(task.ID))
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 1 || wts[0].Status != core.WorktreeActive {
		t.Fatalf("worktrees = %+v, want one active", wts)
	}
	if _, err := os.Stat(wts[0].Path); err != nil {
		t.Errorf("worktree checkout %q missing: %v", wts[0].Path, err)
	}
}

func TestArchiveRemovesCleanWorktree(t *testing.T) {
	h := newHarness(t, nil)
	project := h.projectWithRepo("repo")
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "Task", "")

	wts, _ := h.store.ListWorktrees(t.Context(), core.NodeID(task.ID))
	wtPath := wts[0].Path

	var archived archiveResponse
	h.decode(h.do(http.MethodPost, "/api/v1/nodes/"+task.ID+"/archive", nil), http.StatusOK, &archived)
	if !slices.Contains(archived.Archived, task.ID) {
		t.Errorf("archived = %v, want it to include %s", archived.Archived, task.ID)
	}

	wts, _ = h.store.ListWorktrees(t.Context(), core.NodeID(task.ID))
	if len(wts) != 1 || wts[0].Status != core.WorktreeRemoved {
		t.Fatalf("worktrees = %+v, want one removed", wts)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree dir %q still on disk (err=%v), want removed", wtPath, err)
	}
	if n, _ := h.tree.Get(core.NodeID(task.ID)); !n.Archived() {
		t.Error("node not archived")
	}
}

func TestArchiveOrphansDirtyWorktree(t *testing.T) {
	h := newHarness(t, nil)
	project := h.projectWithRepo("repo")
	task := h.createNode(core.NodeID(project.ID), core.KindTask, "Task", "")

	wts, _ := h.store.ListWorktrees(t.Context(), core.NodeID(task.ID))
	wtPath := wts[0].Path
	// Uncommitted change makes the worktree dirty, so archive must keep it.
	if err := os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	h.decode(h.do(http.MethodPost, "/api/v1/nodes/"+task.ID+"/archive", nil), http.StatusOK, nil)

	wts, _ = h.store.ListWorktrees(t.Context(), core.NodeID(task.ID))
	if len(wts) != 1 || wts[0].Status != core.WorktreeOrphaned {
		t.Fatalf("worktrees = %+v, want one orphaned", wts)
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("orphaned worktree dir removed (%v), want it kept", err)
	}
	if n, _ := h.tree.Get(core.NodeID(task.ID)); n.Attention != core.AttentionError {
		t.Errorf("node attention = %s, want error", n.Attention)
	}

	var inbox []EventDTO
	h.decode(h.do(http.MethodGet, "/api/v1/inbox", nil), http.StatusOK, &inbox)
	if !slices.ContainsFunc(inbox, func(e EventDTO) bool {
		return e.NodeID == task.ID && e.Type == string(core.EventError)
	}) {
		t.Error("no error event in inbox for the orphaned worktree")
	}
}
