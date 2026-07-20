package gitcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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

// runIn is a small test-only shell to git for repo, used to set up or
// inspect scenarios the Runner API itself doesn't expose (e.g. checking out
// a branch, or reading a ref that a helper under test just wrote).
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
