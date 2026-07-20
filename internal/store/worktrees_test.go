package store

import (
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func testWorktree(id core.WorktreeID, nodeID core.NodeID, repoID core.RepoID) core.Worktree {
	return core.Worktree{
		ID:        id,
		NodeID:    nodeID,
		RepoID:    repoID,
		Path:      "/home/user/.grove/worktrees/" + string(id),
		Branch:    "grove/abc123-task",
		BaseRef:   "main",
		Status:    core.WorktreeActive,
		CreatedAt: msTime(1_700_000_000_000),
	}
}

func mustSaveRepoFixture(t *testing.T, s *Store, projectID core.NodeID) core.Repo {
	t.Helper()
	r := testRepo(core.NewRepoID(), projectID, "grove")
	if err := s.SaveRepo(t.Context(), r); err != nil {
		t.Fatalf("SaveRepo: %v", err)
	}
	return r
}

func TestWorktreeCRUD(t *testing.T) {
	s := newTestStore(t)
	project := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, project)
	repo := mustSaveRepoFixture(t, s, project.ID)
	task := testNode(core.NewNodeID(), project.ID)
	mustSaveNode(t, s, task)

	w := testWorktree(core.NewWorktreeID(), task.ID, repo.ID)
	if err := s.SaveWorktree(t.Context(), w); err != nil {
		t.Fatalf("SaveWorktree: %v", err)
	}

	worktrees, err := s.ListWorktrees(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(worktrees) != 1 || worktrees[0] != w {
		t.Fatalf("ListWorktrees = %+v, want [%+v]", worktrees, w)
	}

	w.Status = core.WorktreeRemoved
	w.RemovedAt = msTime(1_700_000_050_000)
	if err := s.SaveWorktree(t.Context(), w); err != nil {
		t.Fatalf("SaveWorktree (update): %v", err)
	}
	worktrees, err = s.ListWorktrees(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("ListWorktrees after update: %v", err)
	}
	if len(worktrees) != 1 || worktrees[0] != w {
		t.Fatalf("ListWorktrees after update = %+v, want [%+v]", worktrees, w)
	}
}

func TestWorktreeUniqueNodeRepo(t *testing.T) {
	s := newTestStore(t)
	project := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, project)
	repo := mustSaveRepoFixture(t, s, project.ID)
	task := testNode(core.NewNodeID(), project.ID)
	mustSaveNode(t, s, task)

	w1 := testWorktree(core.NewWorktreeID(), task.ID, repo.ID)
	if err := s.SaveWorktree(t.Context(), w1); err != nil {
		t.Fatalf("SaveWorktree w1: %v", err)
	}

	w2 := testWorktree(core.NewWorktreeID(), task.ID, repo.ID) // same (node_id, repo_id), different id
	assertUniqueViolation(t, s.SaveWorktree(t.Context(), w2))
}

func TestListWorktreesByStatus(t *testing.T) {
	s := newTestStore(t)
	project := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, project)
	repo := mustSaveRepoFixture(t, s, project.ID)
	task1 := testNode(core.NewNodeID(), project.ID)
	task2 := testNode(core.NewNodeID(), project.ID)
	mustSaveNode(t, s, task1)
	mustSaveNode(t, s, task2)

	active := testWorktree(core.NewWorktreeID(), task1.ID, repo.ID)
	orphaned := testWorktree(core.NewWorktreeID(), task2.ID, repo.ID)
	orphaned.Status = core.WorktreeOrphaned
	if err := s.SaveWorktree(t.Context(), active); err != nil {
		t.Fatalf("SaveWorktree active: %v", err)
	}
	if err := s.SaveWorktree(t.Context(), orphaned); err != nil {
		t.Fatalf("SaveWorktree orphaned: %v", err)
	}

	got, err := s.ListWorktreesByStatus(t.Context(), core.WorktreeOrphaned)
	if err != nil {
		t.Fatalf("ListWorktreesByStatus: %v", err)
	}
	if len(got) != 1 || got[0].ID != orphaned.ID {
		t.Errorf("ListWorktreesByStatus(orphaned) = %+v, want only [%+v]", got, orphaned)
	}
}
