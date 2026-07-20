package gitcli

import (
	"path/filepath"
	"sync"
)

// repoLocks stripes one mutex per canonical repository path. Concurrent
// worktree operations (add/remove/prune/branch delete) against the same
// repository are not safe to run in parallel; operations against different
// repositories are independent.
var repoLocks = struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}{m: make(map[string]*sync.Mutex)}

// Lock serializes access to repoPath and returns a function that releases
// it. Callers hold the lock across a whole multi-command sequence (e.g. a
// worktree add, or a remove+prune+branch-delete sequence), not just a single
// Run call:
//
//	unlock := gitcli.Lock(repoPath)
//	defer unlock()
//
// repoPath is canonicalized (made absolute, symlinks resolved) so that
// different spellings of the same repository share one mutex; if
// canonicalization fails, the best-effort resolved form is used as the key
// so the function still degrades to a usable (if slightly coarser) lock
// rather than panicking.
func Lock(repoPath string) func() {
	key := canonicalKey(repoPath)

	repoLocks.mu.Lock()
	mu, ok := repoLocks.m[key]
	if !ok {
		mu = &sync.Mutex{}
		repoLocks.m[key] = mu
	}
	repoLocks.mu.Unlock()

	mu.Lock()
	return mu.Unlock
}

func canonicalKey(repoPath string) string {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return repoPath
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs
	}
	return real
}
