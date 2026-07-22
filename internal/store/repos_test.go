package store

import (
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func testRepo(id core.RepoID, projectID core.NodeID, name string) core.Repo {
	return core.Repo{
		ID:          id,
		ProjectID:   projectID,
		Name:        name,
		SourcePath:  "/home/user/code/" + name,
		DefaultBase: "main",
		CreatedAt:   msTime(1_700_000_000_000),
	}
}

func TestRepoCRUD(t *testing.T) {
	s := newTestStore(t)
	project := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, project)

	r := testRepo(core.NewRepoID(), project.ID, "grove")
	if err := s.SaveRepo(t.Context(), r); err != nil {
		t.Fatalf("SaveRepo: %v", err)
	}

	repos, err := s.ListRepos(t.Context(), project.ID)
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 1 || repos[0] != r {
		t.Fatalf("ListRepos = %+v, want [%+v]", repos, r)
	}

	r.DefaultBase = "develop"
	if err := s.SaveRepo(t.Context(), r); err != nil {
		t.Fatalf("SaveRepo (update): %v", err)
	}
	repos, err = s.ListRepos(t.Context(), project.ID)
	if err != nil {
		t.Fatalf("ListRepos after update: %v", err)
	}
	if len(repos) != 1 || repos[0] != r {
		t.Fatalf("ListRepos after update = %+v, want [%+v]", repos, r)
	}

	if err := s.DeleteRepo(t.Context(), r.ID); err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}
	repos, err = s.ListRepos(t.Context(), project.ID)
	if err != nil {
		t.Fatalf("ListRepos after delete: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("ListRepos after delete = %+v, want none", repos)
	}
}

func TestDeleteRepoMissingIsNotError(t *testing.T) {
	s := newTestStore(t)
	if err := s.DeleteRepo(t.Context(), core.NewRepoID()); err != nil {
		t.Errorf("DeleteRepo(missing): %v", err)
	}
}

func TestRepoUniqueProjectName(t *testing.T) {
	s := newTestStore(t)
	project := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, project)

	r1 := testRepo(core.NewRepoID(), project.ID, "grove")
	if err := s.SaveRepo(t.Context(), r1); err != nil {
		t.Fatalf("SaveRepo r1: %v", err)
	}

	r2 := testRepo(core.NewRepoID(), project.ID, "grove") // same (project_id, name), different id
	assertUniqueViolation(t, s.SaveRepo(t.Context(), r2))
}

func TestDeleteRepoIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	project := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, project)
	r := testRepo(core.NewRepoID(), project.ID, "grove")
	if err := s.SaveRepo(t.Context(), r); err != nil {
		t.Fatalf("SaveRepo: %v", err)
	}

	if err := s.DeleteRepo(t.Context(), r.ID); err != nil {
		t.Fatalf("DeleteRepo (first): %v", err)
	}
	if err := s.DeleteRepo(t.Context(), r.ID); err != nil {
		t.Fatalf("DeleteRepo (second): %v", err)
	}

	repos, err := s.ListRepos(t.Context(), project.ID)
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("ListRepos after double delete = %+v, want none", repos)
	}
}

// TestDeleteRepoAllowsNameReuse guards the reason DeleteRepo is a soft
// delete rather than a table-recreate that drops the UNIQUE(project_id,
// name) constraint: the deleted repo's name must be freed for a new repo to
// reuse, without ever hitting the UNIQUE constraint against the tombstoned
// row.
func TestDeleteRepoAllowsNameReuse(t *testing.T) {
	s := newTestStore(t)
	project := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, project)

	original := testRepo(core.NewRepoID(), project.ID, "grove")
	if err := s.SaveRepo(t.Context(), original); err != nil {
		t.Fatalf("SaveRepo original: %v", err)
	}
	if err := s.DeleteRepo(t.Context(), original.ID); err != nil {
		t.Fatalf("DeleteRepo: %v", err)
	}

	replacement := testRepo(core.NewRepoID(), project.ID, "grove") // same (project_id, name), new id
	if err := s.SaveRepo(t.Context(), replacement); err != nil {
		t.Fatalf("SaveRepo replacement after delete: %v", err)
	}

	repos, err := s.ListRepos(t.Context(), project.ID)
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 1 || repos[0] != replacement {
		t.Errorf("ListRepos = %+v, want only [%+v]", repos, replacement)
	}
}

// TestDeleteRepoLeavesWorktreesIntact is the regression test for the
// reported bug: a hard DELETE FROM repos violated worktrees.repo_id's
// foreign key for any repo that had ever provisioned a task worktree, even
// after the owning task was archived (archiving marks worktrees removed, it
// does not delete the row). DeleteRepo must succeed and worktree rows must
// stay exactly as they were, so worktree review keeps working for existing
// tasks.
func TestDeleteRepoLeavesWorktreesIntact(t *testing.T) {
	s := newTestStore(t)
	assertMigrationApplied(t, s, 5)

	project := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, project)
	repo := mustSaveRepoFixture(t, s, project.ID)
	task := testNode(core.NewNodeID(), project.ID)
	mustSaveNode(t, s, task)
	wt := testWorktree(core.NewWorktreeID(), task.ID, repo.ID)
	if err := s.SaveWorktree(t.Context(), wt); err != nil {
		t.Fatalf("SaveWorktree: %v", err)
	}

	if err := s.DeleteRepo(t.Context(), repo.ID); err != nil {
		t.Fatalf("DeleteRepo with an in-use repo: %v", err)
	}

	repos, err := s.ListRepos(t.Context(), project.ID)
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("ListRepos after delete = %+v, want none (repo is gone from the API)", repos)
	}

	worktrees, err := s.ListWorktrees(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(worktrees) != 1 || worktrees[0] != wt {
		t.Errorf("ListWorktrees after repo delete = %+v, want unchanged [%+v]", worktrees, wt)
	}
}

func TestListReposScopedToProject(t *testing.T) {
	s := newTestStore(t)
	p1 := testNode(core.NewNodeID(), "")
	p2 := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, p1)
	mustSaveNode(t, s, p2)

	r1 := testRepo(core.NewRepoID(), p1.ID, "repo-a")
	r2 := testRepo(core.NewRepoID(), p2.ID, "repo-b")
	if err := s.SaveRepo(t.Context(), r1); err != nil {
		t.Fatalf("SaveRepo r1: %v", err)
	}
	if err := s.SaveRepo(t.Context(), r2); err != nil {
		t.Fatalf("SaveRepo r2: %v", err)
	}

	repos, err := s.ListRepos(t.Context(), p1.ID)
	if err != nil {
		t.Fatalf("ListRepos(p1): %v", err)
	}
	if len(repos) != 1 || repos[0].ID != r1.ID {
		t.Errorf("ListRepos(p1) = %+v, want only [%+v]", repos, r1)
	}
}
