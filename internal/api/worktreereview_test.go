package api

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/testutil/fakeagent"
)

// gitInDir runs one git command in dir, failing the test on error.
func gitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// writeInDir writes content to name inside dir.
func writeInDir(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// taskWorktreePath provisions a task under a project-with-repo and returns the
// task node and its worktree path on disk.
func taskWorktreePath(t *testing.T, h *harness, parent core.NodeID, title string) (NodeDTO, string) {
	t.Helper()
	task := h.createNode(parent, core.KindTask, title, "")
	wts, err := h.store.ListWorktrees(t.Context(), core.NodeID(task.ID))
	if err != nil || len(wts) != 1 {
		t.Fatalf("ListWorktrees(%s) = %+v, %v; want one worktree", task.ID, wts, err)
	}
	return task, wts[0].Path
}

func TestWorktreeReview(t *testing.T) {
	h := newHarness(t, nil)
	project := h.projectWithRepo("repo")
	task, wtPath := taskWorktreePath(t, h, core.NodeID(project.ID), "Task")

	// A committed added file plus an uncommitted modification of a tracked file.
	writeInDir(t, wtPath, "new.go", "package main\n")
	gitInDir(t, wtPath, "add", "-A")
	gitInDir(t, wtPath, "commit", "-q", "-m", "add new.go")
	writeInDir(t, wtPath, "README.md", "changed\n")

	var got worktreeReviewDTO
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/worktree?node="+task.ID+"&repo=repo", nil), http.StatusOK, &got)

	if got.NodeID != task.ID || got.Repo != "repo" || got.WorktreePath != wtPath || got.BaseRef != "main" {
		t.Errorf("review meta = %+v, want node/repo/path/base populated", got)
	}
	if !got.HasUncommitted {
		t.Error("has_uncommitted = false, want true (README.md modified)")
	}

	byPath := map[string]worktreeFileDTO{}
	for _, f := range got.Files {
		byPath[f.Path] = f
	}
	added, ok := byPath["new.go"]
	if !ok || added.Status != "added" || added.OriginalContent != "" || added.ModifiedContent != "package main\n" {
		t.Errorf("new.go = %+v, want added with empty original and head content", added)
	}
	modified, ok := byPath["README.md"]
	if !ok || modified.Status != "modified" || modified.OriginalContent != "init\n" || modified.ModifiedContent != "changed\n" {
		t.Errorf("README.md = %+v, want modified init->changed", modified)
	}
}

func TestWorktreeReviewRemovedFile(t *testing.T) {
	h := newHarness(t, nil)
	project := h.projectWithRepo("repo")
	task, wtPath := taskWorktreePath(t, h, core.NodeID(project.ID), "Task")

	// Remove the tracked README committed at the branch point.
	gitInDir(t, wtPath, "rm", "-q", "README.md")

	var got worktreeReviewDTO
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/worktree?node="+task.ID+"&repo=repo", nil), http.StatusOK, &got)

	var readme worktreeFileDTO
	for _, f := range got.Files {
		if f.Path == "README.md" {
			readme = f
		}
	}
	if readme.Status != "removed" || readme.OriginalContent != "init\n" || readme.ModifiedContent != "" {
		t.Errorf("README.md = %+v, want removed with base content and empty modified", readme)
	}
}

func TestWorktreeReviewGitError(t *testing.T) {
	h := newHarness(t, nil)
	project := h.projectWithRepo("repo")
	task, wtPath := taskWorktreePath(t, h, core.NodeID(project.ID), "Task")

	// The store still records the worktree, but its checkout is gone, so the git
	// diff commands fail and the review surfaces as a 502.
	if err := os.RemoveAll(wtPath); err != nil {
		t.Fatalf("remove worktree: %v", err)
	}
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/worktree?node="+task.ID+"&repo=repo", nil), http.StatusBadGateway, nil)
}

func TestMapWorktreeStatus(t *testing.T) {
	cases := map[string]string{
		"A": "added", "M": "modified", "D": "removed", "R100": "renamed",
		"C75": "added", "T": "modified", "": "modified", "X": "modified",
	}
	for in, want := range cases {
		if got := mapWorktreeStatus(in); got != want {
			t.Errorf("mapWorktreeStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorktreeReviewNotFound(t *testing.T) {
	h := newHarness(t, nil)
	// A repo whose name is not the usual "repo", so resolution is exercised by
	// its actual name rather than a hardcoded default.
	project := h.projectWithRepo("alpha")
	task, _ := taskWorktreePath(t, h, core.NodeID(project.ID), "Task")

	// Missing node → 400.
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/worktree?repo=alpha", nil), http.StatusBadRequest, nil)
	// Empty repo resolves the node's single worktree → 200.
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/worktree?node="+task.ID, nil), http.StatusOK, nil)
	// A repo the node has no worktree for → 404.
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/worktree?node="+task.ID+"&repo=nope", nil), http.StatusNotFound, nil)
	// The node's real repo resolves and returns a review.
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/worktree?node="+task.ID+"&repo=alpha", nil), http.StatusOK, nil)
}

func TestWorktreeComments(t *testing.T) {
	h := newHarness(t, nil)
	project := h.projectWithRepo("repo")
	task, _ := taskWorktreePath(t, h, core.NodeID(project.ID), "Task")

	var created worktreeCommentDTO
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/worktree/comments", map[string]any{
		"node": task.ID, "repo": "repo", "path": "new.go", "line": 3, "side": "RIGHT", "body": "rename this",
	}), http.StatusCreated, &created)
	if created.ID == "" || created.NodeID != task.ID || created.Repo != "repo" || created.Line != 3 || created.Side != "RIGHT" {
		t.Fatalf("created = %+v, want id/node/repo/line/side set", created)
	}

	var list struct {
		Comments []worktreeCommentDTO `json:"comments"`
	}
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/worktree/comments?node="+task.ID+"&repo=repo", nil), http.StatusOK, &list)
	if len(list.Comments) != 1 || list.Comments[0].ID != created.ID {
		t.Fatalf("list = %+v, want the created comment", list.Comments)
	}

	h.decode(h.do(http.MethodDelete, "/api/v1/reviews/worktree/comments/"+created.ID, nil), http.StatusNoContent, nil)
	h.decode(h.do(http.MethodGet, "/api/v1/reviews/worktree/comments?node="+task.ID+"&repo=repo", nil), http.StatusOK, &list)
	if len(list.Comments) != 0 {
		t.Errorf("list after delete = %+v, want empty", list.Comments)
	}
}

func TestWorktreeCommentValidation(t *testing.T) {
	h := newHarness(t, nil)
	project := h.projectWithRepo("repo")
	task, _ := taskWorktreePath(t, h, core.NodeID(project.ID), "Task")

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"missing node", map[string]any{"repo": "repo", "path": "x", "body": "b"}, http.StatusBadRequest},
		{"empty body", map[string]any{"node": task.ID, "repo": "repo", "path": "x", "body": ""}, http.StatusBadRequest},
		{"empty path", map[string]any{"node": task.ID, "repo": "repo", "path": "", "body": "b"}, http.StatusBadRequest},
		{"bad side", map[string]any{"node": task.ID, "repo": "repo", "path": "x", "body": "b", "side": "MID"}, http.StatusBadRequest},
		{"unknown repo", map[string]any{"node": task.ID, "repo": "ghost", "path": "x", "body": "b"}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h.decode(h.do(http.MethodPost, "/api/v1/reviews/worktree/comments", tc.body), tc.want, nil)
		})
	}
}

func TestWorktreeMergeClean(t *testing.T) {
	h := newHarness(t, nil)
	project := h.projectWithRepo("repo")
	parent, parentPath := taskWorktreePath(t, h, core.NodeID(project.ID), "Parent")
	child, childPath := taskWorktreePath(t, h, core.NodeID(parent.ID), "Child")

	// A committed change in the child, ready to merge into the parent.
	writeInDir(t, childPath, "child.go", "package child\n")
	gitInDir(t, childPath, "add", "-A")
	gitInDir(t, childPath, "commit", "-q", "-m", "child work")

	var resp mergeResponse
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/worktree/merge", map[string]any{
		"node": child.ID, "repo": "repo",
	}), http.StatusOK, &resp)
	if !resp.Merged {
		t.Fatalf("merge = %+v, want merged true", resp)
	}
	if _, err := os.Stat(filepath.Join(parentPath, "child.go")); err != nil {
		t.Errorf("child.go not merged into parent worktree: %v", err)
	}
}

func TestWorktreeMergeDirtyParent(t *testing.T) {
	h := newHarness(t, nil)
	project := h.projectWithRepo("repo")
	parent, parentPath := taskWorktreePath(t, h, core.NodeID(project.ID), "Parent")
	child, childPath := taskWorktreePath(t, h, core.NodeID(parent.ID), "Child")

	writeInDir(t, childPath, "child.go", "package child\n")
	gitInDir(t, childPath, "add", "-A")
	gitInDir(t, childPath, "commit", "-q", "-m", "child work")

	// Uncommitted work in the parent blocks the squash-merge.
	writeInDir(t, parentPath, "wip.txt", "in progress\n")

	var resp mergeResponse
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/worktree/merge", map[string]any{
		"node": child.ID, "repo": "repo",
	}), http.StatusConflict, &resp)
	if resp.Merged || resp.Message == "" {
		t.Errorf("merge = %+v, want merged false with a message", resp)
	}
}

func TestWorktreeMergeNoParentWorktree(t *testing.T) {
	h := newHarness(t, nil)
	project := h.projectWithRepo("repo")
	// A task directly under the project: its parent (the project) has no worktree.
	task, taskPath := taskWorktreePath(t, h, core.NodeID(project.ID), "Task")
	writeInDir(t, taskPath, "x.go", "package x\n")
	gitInDir(t, taskPath, "add", "-A")
	gitInDir(t, taskPath, "commit", "-q", "-m", "work")

	h.decode(h.do(http.MethodPost, "/api/v1/reviews/worktree/merge", map[string]any{
		"node": task.ID, "repo": "repo",
	}), http.StatusNotFound, nil)
}

func TestWorktreeActionValidation(t *testing.T) {
	h := newHarness(t, nil)
	// merge and address share the {node, repo} decoder: node is required (repo is
	// optional — an empty repo resolves the node's single worktree).
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/worktree/merge", map[string]any{"repo": "repo"}), http.StatusBadRequest, nil)
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/worktree/address", map[string]any{"repo": "repo"}), http.StatusBadRequest, nil)
	// A node with no worktree → 404, not 400.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/worktree/address", map[string]any{"node": "nope"}), http.StatusNotFound, nil)
}

func TestWorktreeMergeConflict(t *testing.T) {
	h := newHarness(t, nil)
	project := h.projectWithRepo("repo")
	parent, parentPath := taskWorktreePath(t, h, core.NodeID(project.ID), "Parent")
	child, childPath := taskWorktreePath(t, h, core.NodeID(parent.ID), "Child")

	// Both branches change the same file differently, so the squash-merge
	// conflicts. The parent stays clean (its change is committed), so this is a
	// git failure rather than ErrDirtyParent.
	writeInDir(t, parentPath, "README.md", "parent edit\n")
	gitInDir(t, parentPath, "commit", "-aqm", "parent edit")
	writeInDir(t, childPath, "README.md", "child edit\n")
	gitInDir(t, childPath, "commit", "-aqm", "child edit")

	h.decode(h.do(http.MethodPost, "/api/v1/reviews/worktree/merge", map[string]any{
		"node": child.ID, "repo": "repo",
	}), http.StatusBadGateway, nil)
}

func TestWorktreeAddressStartsSession(t *testing.T) {
	h := newHarness(t, []fakeagent.Step{{WaitStdinLine: true}, {ExitCode: intPtr(0)}})
	project := h.projectWithRepo("repo")
	task, _ := taskWorktreePath(t, h, core.NodeID(project.ID), "Task")

	// No comments yet → 400.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/worktree/address", map[string]any{
		"node": task.ID, "repo": "repo",
	}), http.StatusBadRequest, nil)

	// Add a comment, then address it — a PTY session starts on the node.
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/worktree/comments", map[string]any{
		"node": task.ID, "repo": "repo", "path": "new.go", "line": 1, "body": "fix the name",
	}), http.StatusCreated, nil)

	var sess SessionDTO
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/worktree/address", map[string]any{
		"node": task.ID, "repo": "repo",
	}), http.StatusCreated, &sess)
	if sess.NodeID != task.ID || sess.Mode != string(core.ModePTY) {
		t.Errorf("session = %+v, want a pty session on the node", sess)
	}
}

func TestComposeAddressPrompt(t *testing.T) {
	prompt := composeAddressPrompt([]store.ReviewDraft{
		{Path: "a.go", Line: 10, Body: "fix this"},
		{Path: "b.go", Line: 2, Body: "and this"},
	})
	if !strings.Contains(prompt, "Address these review comments on your changes:") {
		t.Errorf("prompt missing header:\n%s", prompt)
	}
	if !strings.Contains(prompt, "- a.go:10 — fix this") || !strings.Contains(prompt, "- b.go:2 — and this") {
		t.Errorf("prompt missing a comment line:\n%s", prompt)
	}
}

func TestAIDraftWorktreeLocalDiff(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	dir := newGitRepo(t)
	// An uncommitted change so the local diff has content for the prompt.
	writeInDir(t, dir, "README.md", "changed by the agent\n")

	var gotPrompt string
	h.h.aiDrafter = func(_ context.Context, _ string, prompt string) (string, error) {
		gotPrompt = prompt
		return "  a draft  ", nil
	}

	var resp struct {
		Text string `json:"text"`
	}
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/ai-draft", map[string]any{
		"dir": dir, "pr": 0, "kind": "comment", "path": "README.md", "instruction": "be terse",
	}), http.StatusOK, &resp)

	if resp.Text != "a draft" {
		t.Errorf("text = %q, want trimmed drafter output", resp.Text)
	}
	if !strings.Contains(gotPrompt, "changed by the agent") {
		t.Errorf("prompt missing the local diff:\n%s", gotPrompt)
	}
	if !strings.Contains(gotPrompt, "be terse") {
		t.Errorf("prompt missing the instruction:\n%s", gotPrompt)
	}
}
