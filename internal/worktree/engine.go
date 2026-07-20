package worktree

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/gitcli"
)

// State is a worktree's status relative to its base branch, as reported by
// Check.
type State int

const (
	// Clean means no uncommitted changes and no commits ahead of BaseRef.
	Clean State = iota
	// Dirty means the working tree has uncommitted changes, tracked or
	// untracked. Dirty takes precedence over Unmerged.
	Dirty
	// Unmerged means the working tree is clean but HEAD has commits not
	// present on BaseRef.
	Unmerged
)

func (s State) String() string {
	switch s {
	case Clean:
		return "clean"
	case Dirty:
		return "dirty"
	case Unmerged:
		return "unmerged"
	default:
		return "unknown"
	}
}

// ErrNotClean is returned by Remove when the worktree has uncommitted or
// unmerged work and the caller did not request force removal.
var ErrNotClean = errors.New("worktree not clean")

// ErrDirtyParent is returned by MergeToParent when the parent worktree has
// uncommitted changes, since squash-merging into a dirty tree would mix the
// child's changes with the parent's in-progress edits.
var ErrDirtyParent = errors.New("parent worktree not clean")

// Engine creates, inspects, merges, and removes the per-task git worktrees
// that back grove's task workspaces.
type Engine struct {
	git  *gitcli.Runner
	root string
	now  func() time.Time
}

// NewEngine constructs an Engine. root is the directory under which every
// task workspace is created, one subdirectory per node; it need not exist
// yet. now supplies the clock used to timestamp created worktrees (inject
// time.Now in production, a fixed function in tests).
func NewEngine(git *gitcli.Runner, root string, now func() time.Time) *Engine {
	return &Engine{git: git, root: root, now: now}
}

// Remove deletes wt's worktree and branch. Unless force is true, Remove
// refuses with ErrNotClean when Check reports anything other than Clean.
//
// repo must be the core.Repo that wt belongs to. Its SourcePath (the
// canonical clone) is where the removal commands are run: git requires
// `worktree remove` and `branch -D` to be run from a working tree other than
// the one being removed, so they cannot be run from inside wt.Path itself.
//
// On success, Remove also prunes stale worktree metadata. Branch deletion is
// forced (-D) only when force was requested, since a worktree that passed
// the Clean check is by definition fully merged into its base already.
func (e *Engine) Remove(ctx context.Context, repo core.Repo, wt core.Worktree, force bool) error {
	if !force {
		state, err := e.Check(ctx, wt)
		if err != nil {
			return fmt.Errorf("remove %s: %w", wt.Path, err)
		}
		if state != Clean {
			return fmt.Errorf("%w: %s is %s", ErrNotClean, wt.Path, state)
		}
	}

	unlock := gitcli.Lock(repo.SourcePath)
	defer unlock()

	if err := e.git.WorktreeRemove(ctx, repo.SourcePath, wt.Path, force); err != nil {
		return fmt.Errorf("remove %s: %w", wt.Path, err)
	}
	if err := e.git.WorktreePrune(ctx, repo.SourcePath); err != nil {
		return fmt.Errorf("prune after removing %s: %w", wt.Path, err)
	}
	if err := e.git.BranchDelete(ctx, repo.SourcePath, wt.Branch, force); err != nil {
		return fmt.Errorf("delete branch %s: %w", wt.Branch, err)
	}
	return nil
}

// MergeToParent squash-merges child's branch into parent's branch, staging
// and committing the result in parent's worktree directory. It returns
// ErrDirtyParent without changing anything if parent already has
// uncommitted changes.
func (e *Engine) MergeToParent(ctx context.Context, child, parent core.Worktree) error {
	dirty, err := e.git.IsDirty(ctx, parent.Path)
	if err != nil {
		return fmt.Errorf("check parent %s: %w", parent.Path, err)
	}
	if dirty {
		return fmt.Errorf("%w: %s", ErrDirtyParent, parent.Path)
	}

	if err := e.git.MergeSquash(ctx, parent.Path, child.Branch); err != nil {
		return fmt.Errorf("merge %s into %s: %w", child.Branch, parent.Path, err)
	}
	if err := e.git.Commit(ctx, parent.Path, "merge child branch "+child.Branch); err != nil {
		return fmt.Errorf("commit merge of %s: %w", child.Branch, err)
	}
	return nil
}
