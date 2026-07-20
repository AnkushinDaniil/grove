package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWorktreeAddCreatesBranchAndDir(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()
	ctx := context.Background()

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(ctx, repo, "feature", wtPath, "main"); err != nil {
		t.Fatalf("WorktreeAdd() error = %v", err)
	}

	if info, err := os.Stat(wtPath); err != nil || !info.IsDir() {
		t.Fatalf("worktree dir %s not created: %v", wtPath, err)
	}

	branch, err := r.CurrentBranch(ctx, wtPath)
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if branch != "feature" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "feature")
	}
}

func TestWorktreeAddInvalidBase(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()
	ctx := context.Background()

	wtPath := filepath.Join(t.TempDir(), "wt")
	err := r.WorktreeAdd(ctx, repo, "feature", wtPath, "does-not-exist")
	if err == nil {
		t.Fatal("WorktreeAdd() with invalid base error = nil, want error")
	}
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("WorktreeAdd() error = %T, want *GitError", err)
	}
}

func TestWorktreeRemoveCleanSucceeds(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()
	ctx := context.Background()

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(ctx, repo, "feature", wtPath, "main"); err != nil {
		t.Fatalf("WorktreeAdd() error = %v", err)
	}

	if err := r.WorktreeRemove(ctx, repo, wtPath, false); err != nil {
		t.Fatalf("WorktreeRemove() error = %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree dir %s still exists after remove", wtPath)
	}

	if err := r.WorktreePrune(ctx, repo); err != nil {
		t.Fatalf("WorktreePrune() error = %v", err)
	}
}

func TestWorktreeRemoveRefusesDirtyWithoutForce(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()
	ctx := context.Background()

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(ctx, repo, "feature", wtPath, "main"); err != nil {
		t.Fatalf("WorktreeAdd() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	if err := r.WorktreeRemove(ctx, repo, wtPath, false); err == nil {
		t.Fatal("WorktreeRemove(force=false) on dirty worktree error = nil, want error")
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree dir %s should still exist after refused remove: %v", wtPath, err)
	}

	if err := r.WorktreeRemove(ctx, repo, wtPath, true); err != nil {
		t.Fatalf("WorktreeRemove(force=true) error = %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree dir %s still exists after forced remove", wtPath)
	}
}

func TestBranchDeleteMergedSucceedsWithoutForce(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()
	ctx := context.Background()

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(ctx, repo, "feature", wtPath, "main"); err != nil {
		t.Fatalf("WorktreeAdd() error = %v", err)
	}
	if err := r.WorktreeRemove(ctx, repo, wtPath, false); err != nil {
		t.Fatalf("WorktreeRemove() error = %v", err)
	}

	// feature has no commits beyond main, so a safe (-d) delete succeeds.
	if err := r.BranchDelete(ctx, repo, "feature", false); err != nil {
		t.Fatalf("BranchDelete(force=false) on merged branch error = %v", err)
	}
}

func TestBranchDeleteRefusesUnmergedWithoutForce(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()
	ctx := context.Background()

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(ctx, repo, "feature", wtPath, "main"); err != nil {
		t.Fatalf("WorktreeAdd() error = %v", err)
	}
	commitFile(t, wtPath, "new.txt", "content", "unmerged change")

	// A committed-but-unmerged worktree is still clean, so a plain remove succeeds.
	if err := r.WorktreeRemove(ctx, repo, wtPath, false); err != nil {
		t.Fatalf("WorktreeRemove() error = %v", err)
	}

	if err := r.BranchDelete(ctx, repo, "feature", false); err == nil {
		t.Fatal("BranchDelete(force=false) on unmerged branch error = nil, want error")
	}
	if err := r.BranchDelete(ctx, repo, "feature", true); err != nil {
		t.Fatalf("BranchDelete(force=true) error = %v", err)
	}
}

func TestIsDirty(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()
	ctx := context.Background()

	dirty, err := r.IsDirty(ctx, repo)
	if err != nil {
		t.Fatalf("IsDirty() error = %v", err)
	}
	if dirty {
		t.Error("IsDirty() = true on freshly committed repo, want false")
	}

	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	dirty, err = r.IsDirty(ctx, repo)
	if err != nil {
		t.Fatalf("IsDirty() error = %v", err)
	}
	if !dirty {
		t.Error("IsDirty() = false with an untracked file present, want true")
	}
}

func TestHasUnmerged(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()
	ctx := context.Background()

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(ctx, repo, "feature", wtPath, "main"); err != nil {
		t.Fatalf("WorktreeAdd() error = %v", err)
	}

	unmerged, err := r.HasUnmerged(ctx, wtPath, "main")
	if err != nil {
		t.Fatalf("HasUnmerged() error = %v", err)
	}
	if unmerged {
		t.Error("HasUnmerged() = true for a freshly branched worktree, want false")
	}

	commitFile(t, wtPath, "new.txt", "content", "a new commit")

	unmerged, err = r.HasUnmerged(ctx, wtPath, "main")
	if err != nil {
		t.Fatalf("HasUnmerged() error = %v", err)
	}
	if !unmerged {
		t.Error("HasUnmerged() = false after committing ahead of base, want true")
	}
}

func TestDetectDefaultBase(t *testing.T) {
	t.Run("local main, no origin", func(t *testing.T) {
		repo := newRepo(t)
		r := NewRunner()
		base, err := r.DetectDefaultBase(context.Background(), repo)
		if err != nil {
			t.Fatalf("DetectDefaultBase() error = %v", err)
		}
		if base != "main" {
			t.Errorf("DetectDefaultBase() = %q, want %q", base, "main")
		}
	})

	t.Run("local master only", func(t *testing.T) {
		repo := newBareInitRepo(t, "master")
		r := NewRunner()
		base, err := r.DetectDefaultBase(context.Background(), repo)
		if err != nil {
			t.Fatalf("DetectDefaultBase() error = %v", err)
		}
		if base != "master" {
			t.Errorf("DetectDefaultBase() = %q, want %q", base, "master")
		}
	})

	t.Run("origin/HEAD symref takes precedence", func(t *testing.T) {
		repo := newRepo(t) // has local "main"
		runIn(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")

		r := NewRunner()
		base, err := r.DetectDefaultBase(context.Background(), repo)
		if err != nil {
			t.Fatalf("DetectDefaultBase() error = %v", err)
		}
		if base != "trunk" {
			t.Errorf("DetectDefaultBase() = %q, want %q (from origin/HEAD, ignoring local main)", base, "trunk")
		}
	})

	t.Run("no origin, no main, no master", func(t *testing.T) {
		repo := newBareInitRepo(t, "develop")
		r := NewRunner()
		_, err := r.DetectDefaultBase(context.Background(), repo)
		if err == nil {
			t.Fatal("DetectDefaultBase() error = nil, want error when no base can be resolved")
		}
	})
}

func TestMergeSquashAndCommit(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()
	ctx := context.Background()

	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := r.WorktreeAdd(ctx, repo, "feature", wtPath, "main"); err != nil {
		t.Fatalf("WorktreeAdd() error = %v", err)
	}
	commitFile(t, wtPath, "feature.txt", "feature content", "feature commit")

	if err := r.MergeSquash(ctx, repo, "feature"); err != nil {
		t.Fatalf("MergeSquash() error = %v", err)
	}
	if err := r.Commit(ctx, repo, "merge feature"); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(repo, "feature.txt"))
	if err != nil {
		t.Fatalf("feature.txt not present in repo after squash merge: %v", err)
	}
	if string(got) != "feature content" {
		t.Errorf("feature.txt content = %q, want %q", got, "feature content")
	}

	count := commitCount(t, repo)
	if count != 2 {
		t.Errorf("commit count after squash merge = %d, want 2", count)
	}
}

func TestCommitStagesUntrackedFiles(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := r.Commit(ctx, repo, "add new.txt"); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	if count := commitCount(t, repo); count != 2 {
		t.Errorf("commit count = %d, want 2", count)
	}
	dirty, err := r.IsDirty(ctx, repo)
	if err != nil {
		t.Fatalf("IsDirty() error = %v", err)
	}
	if dirty {
		t.Error("IsDirty() = true after Commit, want false")
	}
}

func TestCurrentBranch(t *testing.T) {
	repo := newRepo(t)
	r := NewRunner()
	ctx := context.Background()

	branch, err := r.CurrentBranch(ctx, repo)
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if branch != "main" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "main")
	}
}

// newBareInitRepo creates a fresh repo on the given initial branch name with
// one commit, and no origin remote configured.
func newBareInitRepo(t *testing.T, initialBranch string) string {
	t.Helper()
	dir := t.TempDir()
	runIn(t, dir, "init", "-q", "-b", initialBranch)
	runIn(t, dir, "config", "user.name", "grove-test")
	runIn(t, dir, "config", "user.email", "grove-test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}
	runIn(t, dir, "add", "-A")
	runIn(t, dir, "commit", "-q", "-m", "initial commit")
	return dir
}

// commitFile writes name with content into dir and commits it.
func commitFile(t *testing.T, dir, name, content, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	runIn(t, dir, "add", "-A")
	runIn(t, dir, "commit", "-q", "-m", message)
}

// commitCount returns the number of commits reachable from HEAD in dir.
func commitCount(t *testing.T, dir string) int {
	t.Helper()
	out := strings.TrimSpace(runIn(t, dir, "rev-list", "--count", "HEAD"))
	n, err := strconv.Atoi(out)
	if err != nil {
		t.Fatalf("parse commit count %q: %v", out, err)
	}
	return n
}
