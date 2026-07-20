package core

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Repo is a git repository registered on a project node. A project may own
// several repos; tasks get one worktree per involved repo.
type Repo struct {
	ID         RepoID
	ProjectID  NodeID
	Name       string // directory name inside task workspaces, e.g. "nethermind"
	SourcePath string // canonical local clone
	// DefaultBase is the base ref for new task branches; empty means
	// auto-detect origin/HEAD at worktree creation time.
	DefaultBase string
	CreatedAt   time.Time
}

func (r Repo) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("%w: repo id is empty", ErrInvalid)
	}
	if r.ProjectID == "" {
		return fmt.Errorf("%w: repo project id is empty", ErrInvalid)
	}
	if !validRepoName(r.Name) {
		return fmt.Errorf("%w: repo name %q must be a plain directory name", ErrInvalid, r.Name)
	}
	if !filepath.IsAbs(r.SourcePath) {
		return fmt.Errorf("%w: repo source path %q must be absolute", ErrInvalid, r.SourcePath)
	}
	return nil
}

// validRepoName accepts names usable as a single path element in workspaces.
func validRepoName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, "/\\") || strings.ContainsRune(name, 0) {
		return false
	}
	return true
}

type WorktreeStatus string

const (
	WorktreeActive WorktreeStatus = "active"
	// WorktreeOrphaned means cleanup found uncommitted or unmerged work; the
	// worktree is kept on disk and the node flagged for review — never silently deleted.
	WorktreeOrphaned WorktreeStatus = "orphaned"
	WorktreeRemoved  WorktreeStatus = "removed"
)

func (s WorktreeStatus) Valid() bool {
	return s == WorktreeActive || s == WorktreeOrphaned || s == WorktreeRemoved
}

// Worktree is one repo checkout inside a task node's workspace directory.
type Worktree struct {
	ID        WorktreeID
	NodeID    NodeID
	RepoID    RepoID
	Path      string
	Branch    string // grove/<short8>-<slug>, identical across repos of one task
	BaseRef   string // resolved at creation: parent's branch (stacking) or repo default
	Status    WorktreeStatus
	CreatedAt time.Time
	RemovedAt time.Time
}
