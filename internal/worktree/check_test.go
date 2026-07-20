package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func createOne(t *testing.T, engine *Engine, repo core.Repo, title string) core.Worktree {
	t.Helper()
	_, wts, err := engine.Create(context.Background(), newTestNode(title), []core.Repo{repo}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return wts[0]
}

func TestCheckClean(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "repo")
	wt := createOne(t, engine, repo, "Clean task")

	state, err := engine.Check(context.Background(), wt)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state != Clean {
		t.Errorf("Check() = %s, want %s", state, Clean)
	}
}

func TestCheckDirty(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "repo")
	wt := createOne(t, engine, repo, "Dirty task")

	if err := os.WriteFile(filepath.Join(wt.Path, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	state, err := engine.Check(context.Background(), wt)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state != Dirty {
		t.Errorf("Check() = %s, want %s", state, Dirty)
	}
}

func TestCheckUnmerged(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "repo")
	wt := createOne(t, engine, repo, "Unmerged task")
	commitFile(t, wt.Path, "new.txt", "content", "a new commit")

	state, err := engine.Check(context.Background(), wt)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state != Unmerged {
		t.Errorf("Check() = %s, want %s", state, Unmerged)
	}
}

func TestCheckDirtyPrecedenceOverUnmerged(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "repo")
	wt := createOne(t, engine, repo, "Dirty and unmerged task")
	commitFile(t, wt.Path, "new.txt", "content", "a new commit")
	if err := os.WriteFile(filepath.Join(wt.Path, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	state, err := engine.Check(context.Background(), wt)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state != Dirty {
		t.Errorf("Check() = %s, want %s (dirty must win over unmerged)", state, Dirty)
	}
}
