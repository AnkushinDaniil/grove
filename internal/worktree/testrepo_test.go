package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/gitcli"
)

// newRepo creates a temporary git repository with one commit on branch
// "main" and returns its absolute path.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.name", "grove-test")
	run("config", "user.email", "grove-test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("init\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "initial commit")

	return dir
}

// runIn is a test-only shortcut to git for dir, used to set up or inspect
// scenarios the Engine/Runner API under test doesn't itself expose.
func runIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
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

// fixedNow returns a clock function that always reports the same instant.
func fixedNow() func() time.Time {
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

// newTestEngine builds an Engine backed by a real gitcli.Runner, rooted at a
// fresh temp directory.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	return NewEngine(gitcli.NewRunner(), t.TempDir(), fixedNow())
}

// newTestRepo registers a fresh git repo (see newRepo) as a core.Repo named
// name.
func newTestRepo(t *testing.T, name string) core.Repo {
	t.Helper()
	return core.Repo{
		ID:         core.NewRepoID(),
		ProjectID:  core.NewNodeID(),
		Name:       name,
		SourcePath: newRepo(t),
	}
}

// newTestNode builds a minimal core.Node with the given title, suitable for
// passing to Engine.Create.
func newTestNode(title string) core.Node {
	return core.Node{
		ID:     core.NewNodeID(),
		Kind:   core.KindTask,
		Title:  title,
		Status: core.StatusIdle,
	}
}
