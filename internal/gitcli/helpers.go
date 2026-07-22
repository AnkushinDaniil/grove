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

// MergeBase returns the best common ancestor commit of a and b, resolved in
// dir (`git merge-base a b`). It is the branch point a worktree review diffs
// its working tree against.
func (r *Runner) MergeBase(ctx context.Context, dir, a, b string) (string, error) {
	out, err := r.Run(ctx, dir, "merge-base", a, b)
	if err != nil {
		return "", fmt.Errorf("merge-base %s %s: %w", a, b, err)
	}
	return out, nil
}

// ShowFile returns the raw bytes of path as of ref in dir (`git show ref:path`).
// Contents are returned untrimmed so file bytes survive exactly. A path absent
// at ref yields a *GitError, which callers treat as an absent side (e.g. the
// base side of an added file).
func (r *Runner) ShowFile(ctx context.Context, dir, ref, path string) ([]byte, error) {
	out, err := r.output(ctx, dir, "show", ref+":"+path)
	if err != nil {
		return nil, fmt.Errorf("show %s:%s: %w", ref, path, err)
	}
	return out, nil
}

// NameStatus is one file's change between a base tree and the working tree:
// Status is git's raw letter (A|M|D|T, or R/C followed by a similarity score),
// Path is the current path, and OldPath is the pre-rename/copy path, set only
// when Status begins with R or C.
type NameStatus struct {
	Status  string
	Path    string
	OldPath string
}

// DiffNameStatus returns the name-status of every tracked change between ref and
// dir's working tree (`git diff --name-status --find-renames ref`). Rename and
// copy entries carry the new path in Path and the old path in OldPath. Untracked
// files are not reported here (see UntrackedFiles).
func (r *Runner) DiffNameStatus(ctx context.Context, dir, ref string) ([]NameStatus, error) {
	out, err := r.Run(ctx, dir, "diff", "--name-status", "--find-renames", ref)
	if err != nil {
		return nil, fmt.Errorf("diff --name-status %s: %w", ref, err)
	}
	if out == "" {
		return nil, nil
	}
	var changes []NameStatus
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		if status != "" && (status[0] == 'R' || status[0] == 'C') && len(fields) >= 3 {
			changes = append(changes, NameStatus{Status: status, OldPath: fields[1], Path: fields[2]})
			continue
		}
		changes = append(changes, NameStatus{Status: status, Path: fields[1]})
	}
	return changes, nil
}

// NumStatEntry is one file's added/deleted line counts. Binary files report -1
// for both, matching git's `-` numstat markers.
type NumStatEntry struct {
	Additions int
	Deletions int
}

// NumStat returns per-path added/deleted line counts between ref and dir's
// working tree (`git diff --numstat ref`), keyed by the file path. Binary files
// (git prints `-\t-`) are recorded as -1/-1.
func (r *Runner) NumStat(ctx context.Context, dir, ref string) (map[string]NumStatEntry, error) {
	out, err := r.Run(ctx, dir, "diff", "--numstat", ref)
	if err != nil {
		return nil, fmt.Errorf("diff --numstat %s: %w", ref, err)
	}
	stats := make(map[string]NumStatEntry)
	if out == "" {
		return stats, nil
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		stats[fields[2]] = NumStatEntry{Additions: numStatCount(fields[0]), Deletions: numStatCount(fields[1])}
	}
	return stats, nil
}

// numStatCount parses one numstat count, mapping git's binary `-` marker to -1
// and any unparseable value to 0.
func numStatCount(s string) int {
	if s == "-" {
		return -1
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// UntrackedFiles lists dir's untracked, non-ignored files
// (`git ls-files --others --exclude-standard`), one repo-relative path per
// entry. These are the additions a base-tree diff misses.
func (r *Runner) UntrackedFiles(ctx context.Context, dir string) ([]string, error) {
	out, err := r.Run(ctx, dir, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("ls-files --others: %w", err)
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}
