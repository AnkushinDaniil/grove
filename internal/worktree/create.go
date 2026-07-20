package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/gitcli"
)

// Create provisions a task workspace for node: one git worktree per repo in
// repos, all sharing a single branch name derived from node's id and title
// (grove/<short8>-<slug>). parentByRepo supplies, for repos involved in a
// stacked subtask, the parent node's worktree to branch from instead of the
// repo's default base.
//
// On any per-repo failure, Create removes the worktrees it already created
// (best-effort) before returning the error.
func (e *Engine) Create(
	ctx context.Context,
	node core.Node,
	repos []core.Repo,
	parentByRepo map[core.RepoID]core.Worktree,
) (string, []core.Worktree, error) {
	absRoot, err := filepath.Abs(e.root)
	if err != nil {
		return "", nil, fmt.Errorf("resolve workspace root: %w", err)
	}

	wsName := shortID(node.ID) + "-" + slugify(node.Title)
	branch := "grove/" + wsName
	ws := filepath.Join(absRoot, wsName)

	repoByID := make(map[core.RepoID]core.Repo, len(repos))
	wts := make([]core.Worktree, 0, len(repos))

	for _, repo := range repos {
		repoByID[repo.ID] = repo

		base, err := e.resolveBase(ctx, repo, parentByRepo)
		if err != nil {
			return "", nil, e.failCreate(ctx, wts, repoByID, fmt.Errorf("resolve base for repo %s: %w", repo.Name, err))
		}

		path := filepath.Join(ws, repo.Name)
		if err := e.addWorktree(ctx, repo, branch, base, path); err != nil {
			return "", nil, e.failCreate(ctx, wts, repoByID, fmt.Errorf("create worktree for repo %s: %w", repo.Name, err))
		}

		wts = append(wts, core.Worktree{
			ID:        core.NewWorktreeID(),
			NodeID:    node.ID,
			RepoID:    repo.ID,
			Path:      path,
			Branch:    branch,
			BaseRef:   base,
			Status:    core.WorktreeActive,
			CreatedAt: e.now(),
		})
	}

	if err := e.writeManifest(ws, node, wts, repoByID); err != nil {
		return "", nil, e.failCreate(ctx, wts, repoByID, fmt.Errorf("write manifest: %w", err))
	}

	return ws, wts, nil
}

// resolveBase picks the base ref for repo: the parent task's branch when
// stacking, else the repo's configured default, else the repo's detected
// default branch.
func (e *Engine) resolveBase(
	ctx context.Context,
	repo core.Repo,
	parentByRepo map[core.RepoID]core.Worktree,
) (string, error) {
	if parent, ok := parentByRepo[repo.ID]; ok {
		return parent.Branch, nil
	}
	if repo.DefaultBase != "" {
		return repo.DefaultBase, nil
	}
	base, err := e.git.DetectDefaultBase(ctx, repo.SourcePath)
	if err != nil {
		return "", fmt.Errorf("detect default base: %w", err)
	}
	return base, nil
}

// addWorktree creates one repo's worktree, holding the per-repo lock across
// the whole operation.
func (e *Engine) addWorktree(ctx context.Context, repo core.Repo, branch, base, path string) error {
	unlock := gitcli.Lock(repo.SourcePath)
	defer unlock()
	return e.git.WorktreeAdd(ctx, repo.SourcePath, branch, path, base)
}

// failCreate rolls back already-created worktrees (best-effort) and joins
// any rollback failure onto createErr, so a failed cleanup is reported
// rather than silently dropped.
func (e *Engine) failCreate(
	ctx context.Context,
	created []core.Worktree,
	repoByID map[core.RepoID]core.Repo,
	createErr error,
) error {
	if rbErr := e.rollback(ctx, created, repoByID); rbErr != nil {
		return errors.Join(createErr, rbErr)
	}
	return createErr
}

// rollback best-effort removes each already-created worktree. It returns a
// joined error of any removals that failed, or nil if all succeeded.
func (e *Engine) rollback(ctx context.Context, created []core.Worktree, repoByID map[core.RepoID]core.Repo) error {
	errs := make([]error, 0, len(created))
	for _, wt := range created {
		repo, ok := repoByID[wt.RepoID]
		if !ok {
			continue
		}
		if err := e.rollbackOne(ctx, repo, wt); err != nil {
			errs = append(errs, fmt.Errorf("rollback %s: %w", wt.Path, err))
		}
	}
	return errors.Join(errs...)
}

func (e *Engine) rollbackOne(ctx context.Context, repo core.Repo, wt core.Worktree) error {
	unlock := gitcli.Lock(repo.SourcePath)
	defer unlock()
	return e.git.WorktreeRemove(ctx, repo.SourcePath, wt.Path, true)
}

// shortID returns the first 8 characters of a node ID (its time-sortable
// UUIDv7 prefix), used as the task workspace directory prefix.
func shortID(id core.NodeID) string {
	s := string(id)
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

const groveManifestTemplate = `# {{.Title}}

**Node:** {{.NodeID}}
{{if .Brief}}
{{.Brief}}
{{end}}
| Repo | Branch | Base |
| --- | --- | --- |
{{range .Repos}}| {{.Name}} | {{.Branch}} | {{.Base}} |
{{end}}`

var groveManifestTmpl = template.Must(template.New("grove-manifest").Parse(groveManifestTemplate))

type manifestRepoRow struct {
	Name, Branch, Base string
}

type manifestData struct {
	Title, NodeID, Brief string
	Repos                []manifestRepoRow
}

// writeManifest renders and writes the GROVE.md manifest describing node's
// task workspace into ws.
func (e *Engine) writeManifest(ws string, node core.Node, wts []core.Worktree, repoByID map[core.RepoID]core.Repo) error {
	rows := make([]manifestRepoRow, 0, len(wts))
	for _, wt := range wts {
		repo := repoByID[wt.RepoID]
		rows = append(rows, manifestRepoRow{Name: repo.Name, Branch: wt.Branch, Base: wt.BaseRef})
	}
	data := manifestData{
		Title:  node.Title,
		NodeID: string(node.ID),
		Brief:  node.Brief,
		Repos:  rows,
	}

	var buf bytes.Buffer
	if err := groveManifestTmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("render GROVE.md: %w", err)
	}

	if err := os.MkdirAll(ws, 0o750); err != nil {
		return fmt.Errorf("create workspace dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(ws, "GROVE.md"), buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write GROVE.md: %w", err)
	}
	return nil
}
