package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func TestNormalizeWorkDir(t *testing.T) {
	home := "/Users/dev"
	tests := []struct {
		name, dir, want string
	}{
		{"empty stays empty", "", ""},
		{"tilde alone", "~", home},
		{"tilde path", "~/code/grove", "/Users/dev/code/grove"},
		{"bare relative is home-relative", "RiderProjects/nethermind/", "/Users/dev/RiderProjects/nethermind"},
		{"absolute cleaned", "/tmp//x/../y", "/tmp/y"},
		{"dot segments relative", "./code", "/Users/dev/code"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeWorkDir(home, tt.dir); got != tt.want {
				t.Fatalf("normalizeWorkDir(%q, %q) = %q, want %q", home, tt.dir, got, tt.want)
			}
		})
	}
	// Without a resolvable home, relative input passes through so validation
	// reports it rather than fabricating a path.
	if got := normalizeWorkDir("", "code"); got != "code" {
		t.Fatalf("normalizeWorkDir(no home) = %q, want passthrough", got)
	}
}

// TestWorkDirRelativeAcceptedEndToEnd proves the exact flow from the bug
// report: typing a home-relative path (what the completion UI shows) must be
// accepted by PATCH and stored absolute.
func TestWorkDirRelativeAcceptedEndToEnd(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	sub, err := os.MkdirTemp(home, "grove-workdir-test-*")
	if err != nil {
		t.Skipf("cannot create dir under home: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sub) })
	rel := filepath.Base(sub) + "/" // trailing slash, exactly as completion leaves it

	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "P", "fake")

	var got NodeDTO
	h.decode(h.do(http.MethodPatch, "/api/v1/nodes/"+project.ID, map[string]any{
		"work_dir": rel,
	}), http.StatusOK, &got)
	if got.WorkDir != filepath.Join(home, filepath.Base(sub)) {
		t.Fatalf("stored work_dir = %q, want absolute under home", got.WorkDir)
	}

	// Nonexistent relative still fails with the actionable message.
	resp := h.do(http.MethodPatch, "/api/v1/nodes/"+project.ID, map[string]any{
		"work_dir": "definitely-not-a-real-dir-xyz/",
	})
	if resp.status != http.StatusBadRequest {
		t.Fatalf("nonexistent relative: status = %d, want 400", resp.status)
	}
}

func TestSplitCompletionHomeRelative(t *testing.T) {
	parent, base := splitCompletion("RiderProjects/nether", "/Users/dev")
	if parent != "/Users/dev/RiderProjects" || base != "nether" {
		t.Fatalf("splitCompletion = (%q, %q), want home-joined", parent, base)
	}
	parent, base = splitCompletion("RiderProjects/", "/Users/dev")
	if parent != "/Users/dev/RiderProjects/" || base != "" {
		t.Fatalf("trailing slash: splitCompletion = (%q, %q)", parent, base)
	}
}
