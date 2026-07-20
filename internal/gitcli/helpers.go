package gitcli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// WorktreeAdd creates a new worktree at path on a new branch checked out
// from base: `git worktree add -b branch path base`, run in repo.
func (r *Runner) WorktreeAdd(ctx context.Context, repo, branch, path, base string) error {
	if _, err := r.Run(ctx, repo, "worktree", "add", "-b", branch, path, base); err != nil {
		return fmt.Errorf("worktree add %s (branch %s from %s): %w", path, branch, base, err)
	}
	return nil
}

// WorktreeRemove removes the worktree at path, run in repo. With force it
// removes even with uncommitted changes or untracked files.
func (r *Runner) WorktreeRemove(ctx context.Context, repo, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	if _, err := r.Run(ctx, repo, args...); err != nil {
		return fmt.Errorf("worktree remove %s: %w", path, err)
	}
	return nil
}

// WorktreePrune removes administrative files for worktrees whose directories
// no longer exist on disk.
func (r *Runner) WorktreePrune(ctx context.Context, repo string) error {
	if _, err := r.Run(ctx, repo, "worktree", "prune"); err != nil {
		return fmt.Errorf("worktree prune: %w", err)
	}
	return nil
}

// BranchDelete deletes branch, run in repo. With force it uses -D (delete
// even if not fully merged) instead of -d.
func (r *Runner) BranchDelete(ctx context.Context, repo, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	if _, err := r.Run(ctx, repo, "branch", flag, branch); err != nil {
		return fmt.Errorf("branch delete %s: %w", branch, err)
	}
	return nil
}

// IsDirty reports whether dir has uncommitted changes, tracked or untracked.
func (r *Runner) IsDirty(ctx context.Context, dir string) (bool, error) {
	out, err := r.Run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("status: %w", err)
	}
	return out != "", nil
}

// HasUnmerged reports whether dir's HEAD has commits not reachable from
// base, i.e. whether `base..HEAD` is non-empty.
func (r *Runner) HasUnmerged(ctx context.Context, dir, base string) (bool, error) {
	out, err := r.Run(ctx, dir, "rev-list", "--count", base+"..HEAD")
	if err != nil {
		return false, fmt.Errorf("rev-list %s..HEAD: %w", base, err)
	}
	count, err := strconv.Atoi(out)
	if err != nil {
		return false, fmt.Errorf("parse rev-list count %q: %w", out, err)
	}
	return count > 0, nil
}

// DetectDefaultBase resolves the short name of repo's default branch, so it
// can be used as a worktree add base. It tries, in order:
//
//  1. the remote-tracking symbolic ref origin/HEAD, stripped of its
//     "refs/remotes/origin/" prefix (e.g. "refs/remotes/origin/main" → "main")
//  2. a local "main" branch
//  3. a local "master" branch
//
// and returns an error if none resolve, since a worktree cannot be created
// without a base commit-ish.
func (r *Runner) DetectDefaultBase(ctx context.Context, repo string) (string, error) {
	if ref, err := r.Run(ctx, repo, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		if short := strings.TrimPrefix(ref, "refs/remotes/origin/"); short != "" {
			return short, nil
		}
	}
	for _, candidate := range []string{"main", "master"} {
		if _, err := r.Run(ctx, repo, "show-ref", "--verify", "refs/heads/"+candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("detect default base in %s: no origin/HEAD, main, or master branch found", repo)
}

// MergeSquash stages branch's changes onto dir's current branch via
// `git merge --squash branch`, without committing. The caller is responsible
// for reviewing and committing the staged result, e.g. via Commit.
func (r *Runner) MergeSquash(ctx context.Context, dir, branch string) error {
	if _, err := r.Run(ctx, dir, "merge", "--squash", branch); err != nil {
		return fmt.Errorf("merge --squash %s: %w", branch, err)
	}
	return nil
}

// CurrentBranch returns the short name of dir's checked-out branch.
func (r *Runner) CurrentBranch(ctx context.Context, dir string) (string, error) {
	out, err := r.Run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("current branch: %w", err)
	}
	return out, nil
}

// Commit stages all changes in dir and commits them with message. It is
// typically used after MergeSquash to finalize a staged squash merge.
func (r *Runner) Commit(ctx context.Context, dir, message string) error {
	if _, err := r.Run(ctx, dir, "add", "-A"); err != nil {
		return fmt.Errorf("add -A: %w", err)
	}
	if _, err := r.Run(ctx, dir, "commit", "-m", message); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
