package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func TestCreateSingleRepo(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "myrepo")
	node := newTestNode("Optimize RPC handler")
	node.Brief = "Make the RPC layer faster."

	ws, wts, err := engine.Create(context.Background(), node, []core.Repo{repo}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if info, statErr := os.Stat(ws); statErr != nil || !info.IsDir() {
		t.Fatalf("workspace dir %s not created: %v", ws, statErr)
	}
	if len(wts) != 1 {
		t.Fatalf("Create() returned %d worktrees, want 1", len(wts))
	}

	wt := wts[0]
	if wt.NodeID != node.ID {
		t.Errorf("Worktree.NodeID = %q, want %q", wt.NodeID, node.ID)
	}
	if wt.RepoID != repo.ID {
		t.Errorf("Worktree.RepoID = %q, want %q", wt.RepoID, repo.ID)
	}
	wantPath := filepath.Join(ws, "myrepo")
	if wt.Path != wantPath {
		t.Errorf("Worktree.Path = %q, want %q", wt.Path, wantPath)
	}
	if !strings.HasPrefix(wt.Branch, "grove/"+string(node.ID)[:8]+"-") {
		t.Errorf("Worktree.Branch = %q, want prefix %q", wt.Branch, "grove/"+string(node.ID)[:8]+"-")
	}
	if !strings.Contains(wt.Branch, "optimize-rpc-handler") {
		t.Errorf("Worktree.Branch = %q, want it to contain the title slug", wt.Branch)
	}
	if wt.BaseRef != "main" {
		t.Errorf("Worktree.BaseRef = %q, want %q (detected default)", wt.BaseRef, "main")
	}
	if wt.Status != core.WorktreeActive {
		t.Errorf("Worktree.Status = %q, want %q", wt.Status, core.WorktreeActive)
	}

	if info, statErr := os.Stat(wt.Path); statErr != nil || !info.IsDir() {
		t.Fatalf("worktree dir %s not created: %v", wt.Path, statErr)
	}
	branch := strings.TrimSpace(runIn(t, wt.Path, "rev-parse", "--abbrev-ref", "HEAD"))
	if branch != wt.Branch {
		t.Errorf("checked out branch = %q, want %q", branch, wt.Branch)
	}

	manifest, err := os.ReadFile(filepath.Join(ws, "GROVE.md"))
	if err != nil {
		t.Fatalf("GROVE.md not written: %v", err)
	}
	for _, want := range []string{node.Title, string(node.ID), node.Brief, "myrepo", wt.Branch, "main"} {
		if !strings.Contains(string(manifest), want) {
			t.Errorf("GROVE.md missing %q; content:\n%s", want, manifest)
		}
	}
}

func TestCreateMultiRepoSharesBranch(t *testing.T) {
	engine := newTestEngine(t)
	repoA := newTestRepo(t, "repoA")
	repoB := newTestRepo(t, "repoB")
	node := newTestNode("Multi repo task")

	ws, wts, err := engine.Create(context.Background(), node, []core.Repo{repoA, repoB}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(wts) != 2 {
		t.Fatalf("Create() returned %d worktrees, want 2", len(wts))
	}
	if wts[0].Branch != wts[1].Branch {
		t.Errorf("branch differs across repos: %q vs %q", wts[0].Branch, wts[1].Branch)
	}
	if wts[0].Path == wts[1].Path {
		t.Errorf("both worktrees share the same path %q", wts[0].Path)
	}
	if _, statErr := os.Stat(filepath.Join(ws, "repoA")); statErr != nil {
		t.Errorf("repoA worktree missing: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(ws, "repoB")); statErr != nil {
		t.Errorf("repoB worktree missing: %v", statErr)
	}
}

func TestCreateStackedBase(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "repo")

	parentNode := newTestNode("Parent task")
	_, parentWts, err := engine.Create(context.Background(), parentNode, []core.Repo{repo}, nil)
	if err != nil {
		t.Fatalf("Create(parent) error = %v", err)
	}
	parentWt := parentWts[0]
	commitFile(t, parentWt.Path, "parent.txt", "parent content", "parent commit")

	childNode := newTestNode("Child task")
	parentByRepo := map[core.RepoID]core.Worktree{repo.ID: parentWt}
	_, childWts, err := engine.Create(context.Background(), childNode, []core.Repo{repo}, parentByRepo)
	if err != nil {
		t.Fatalf("Create(child) error = %v", err)
	}
	childWt := childWts[0]

	if childWt.BaseRef != parentWt.Branch {
		t.Errorf("child BaseRef = %q, want parent branch %q", childWt.BaseRef, parentWt.Branch)
	}
	if _, statErr := os.Stat(filepath.Join(childWt.Path, "parent.txt")); statErr != nil {
		t.Errorf("child worktree missing parent.txt inherited from stacked base: %v", statErr)
	}
}

func TestCreateRepoDefaultBaseOverride(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "repo")
	runIn(t, repo.SourcePath, "branch", "develop")
	repo.DefaultBase = "develop"

	node := newTestNode("Task on develop")
	_, wts, err := engine.Create(context.Background(), node, []core.Repo{repo}, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if wts[0].BaseRef != "develop" {
		t.Errorf("Worktree.BaseRef = %q, want %q (repo.DefaultBase override)", wts[0].BaseRef, "develop")
	}
}

func TestCreateRollbackOnSecondRepoFailure(t *testing.T) {
	engine := newTestEngine(t)
	repoA := newTestRepo(t, "repoA")
	repoB := core.Repo{
		ID:         core.NewRepoID(),
		ProjectID:  core.NewNodeID(),
		Name:       "repoB",
		SourcePath: filepath.Join(t.TempDir(), "does-not-exist"),
	}
	node := newTestNode("Rollback task")

	ws, wts, err := engine.Create(context.Background(), node, []core.Repo{repoA, repoB}, nil)
	if err == nil {
		t.Fatal("Create() with an invalid second repo error = nil, want error")
	}
	if ws != "" {
		t.Errorf("Create() ws = %q on failure, want empty", ws)
	}
	if wts != nil {
		t.Errorf("Create() wts = %v on failure, want nil", wts)
	}

	out := runIn(t, repoA.SourcePath, "worktree", "list", "--porcelain")
	if strings.Count(out, "worktree ") != 1 {
		t.Errorf("repoA still has extra worktrees after rollback:\n%s", out)
	}
}

func TestCreateConcurrentSameRepo(t *testing.T) {
	engine := newTestEngine(t)
	repo := newTestRepo(t, "shared-repo")

	nodes := []core.Node{newTestNode("Task Alpha"), newTestNode("Task Beta")}
	branches := make([]string, len(nodes))
	errs := make([]error, len(nodes))

	var wg sync.WaitGroup
	for i, node := range nodes {
		wg.Add(1)
		go func(i int, node core.Node) {
			defer wg.Done()
			_, wts, err := engine.Create(context.Background(), node, []core.Repo{repo}, nil)
			errs[i] = err
			if err == nil {
				branches[i] = wts[0].Branch
			}
		}(i, node)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Create() for node %d error = %v, want nil", i, err)
		}
	}
	if branches[0] == "" || branches[1] == "" {
		t.Fatal("one or both concurrent Create calls did not produce a branch")
	}
	if branches[0] == branches[1] {
		t.Errorf("both concurrent creates produced the same branch %q", branches[0])
	}
}
