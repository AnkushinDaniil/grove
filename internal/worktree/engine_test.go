package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{Clean, "clean"},
		{Dirty, "dirty"},
		{Unmerged, "unmerged"},
		{State(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestRemoveRefusesDirtyWithoutForce(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "repo")
	wt := createOne(t, engine, repo, "Dirty task")
	if err := os.WriteFile(filepath.Join(wt.Path, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	err := engine.Remove(context.Background(), repo, wt, false)
	if !errors.Is(err, ErrNotClean) {
		t.Fatalf("Remove(force=false) on dirty worktree error = %v, want ErrNotClean", err)
	}
	if _, statErr := os.Stat(wt.Path); statErr != nil {
		t.Errorf("worktree dir should still exist after refused remove: %v", statErr)
	}
}

func TestRemoveRefusesUnmergedWithoutForce(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "repo")
	wt := createOne(t, engine, repo, "Unmerged task")
	commitFile(t, wt.Path, "new.txt", "content", "a new commit")

	err := engine.Remove(context.Background(), repo, wt, false)
	if !errors.Is(err, ErrNotClean) {
		t.Fatalf("Remove(force=false) on unmerged worktree error = %v, want ErrNotClean", err)
	}
	if _, statErr := os.Stat(wt.Path); statErr != nil {
		t.Errorf("worktree dir should still exist after refused remove: %v", statErr)
	}
}

func TestRemoveForceRemovesDirty(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "repo")
	wt := createOne(t, engine, repo, "Dirty task")
	if err := os.WriteFile(filepath.Join(wt.Path, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	if err := engine.Remove(context.Background(), repo, wt, true); err != nil {
		t.Fatalf("Remove(force=true) error = %v", err)
	}
	if _, statErr := os.Stat(wt.Path); !os.IsNotExist(statErr) {
		t.Errorf("worktree dir still exists after forced remove")
	}
}

func TestRemoveCleanDeletesBranch(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "repo")
	wt := createOne(t, engine, repo, "Clean task")

	if err := engine.Remove(context.Background(), repo, wt, false); err != nil {
		t.Fatalf("Remove(force=false) on clean worktree error = %v", err)
	}
	if _, statErr := os.Stat(wt.Path); !os.IsNotExist(statErr) {
		t.Errorf("worktree dir still exists after clean remove")
	}

	branches := runIn(t, repo.SourcePath, "branch", "--list", wt.Branch)
	if strings.TrimSpace(branches) != "" {
		t.Errorf("branch %s still exists after clean remove: %q", wt.Branch, branches)
	}
}

func TestMergeToParentSquashLandsChanges(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "repo")

	parentWt := createOne(t, engine, repo, "Parent task")

	childNode := newTestNode("Child task")
	parentByRepo := map[core.RepoID]core.Worktree{repo.ID: parentWt}
	_, childWts, err := engine.Create(context.Background(), childNode, []core.Repo{repo}, parentByRepo)
	if err != nil {
		t.Fatalf("Create(child) error = %v", err)
	}
	childWt := childWts[0]
	commitFile(t, childWt.Path, "child.txt", "child content", "child commit")

	if err := engine.MergeToParent(context.Background(), childWt, parentWt); err != nil {
		t.Fatalf("MergeToParent() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(parentWt.Path, "child.txt"))
	if err != nil {
		t.Fatalf("child.txt not present in parent after merge: %v", err)
	}
	if string(got) != "child content" {
		t.Errorf("child.txt content = %q, want %q", got, "child content")
	}

	dirty := strings.TrimSpace(runIn(t, parentWt.Path, "status", "--porcelain"))
	if dirty != "" {
		t.Errorf("parent worktree dirty after merge commit: %q", dirty)
	}
}

func TestMergeToParentDirtyParentRefuses(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "repo")

	parentWt := createOne(t, engine, repo, "Parent task")

	childNode := newTestNode("Child task")
	parentByRepo := map[core.RepoID]core.Worktree{repo.ID: parentWt}
	_, childWts, err := engine.Create(context.Background(), childNode, []core.Repo{repo}, parentByRepo)
	if err != nil {
		t.Fatalf("Create(child) error = %v", err)
	}
	childWt := childWts[0]
	commitFile(t, childWt.Path, "child.txt", "child content", "child commit")

	if err := os.WriteFile(filepath.Join(parentWt.Path, "dirty.txt"), []byte("uncommitted"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	err = engine.MergeToParent(context.Background(), childWt, parentWt)
	if !errors.Is(err, ErrDirtyParent) {
		t.Fatalf("MergeToParent() on dirty parent error = %v, want ErrDirtyParent", err)
	}
	if _, statErr := os.Stat(filepath.Join(parentWt.Path, "child.txt")); !os.IsNotExist(statErr) {
		t.Errorf("child.txt should not exist in parent after a refused merge")
	}
}
