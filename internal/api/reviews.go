package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/github"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// Setting keys backing the Review Radar. review_dirs is the JSON []string of
// watched repositories; review_heads is a JSON map "owner/repo#pr" -> head oid,
// the per-PR state that drives re-review detection.
const (
	settingReviewDirs  = "review_dirs"
	settingReviewHeads = "review_heads"
)

// reviewConcurrency bounds how many repositories are queried in parallel; each
// query shells out to gh, so a small fixed fan-out keeps load predictable.
const reviewConcurrency = 4

// GitHubClient is the subset of the gh CLI wrapper the review handlers need. It
// is an interface so tests inject a client backed by a fake gh runner.
type GitHubClient interface {
	Login(ctx context.Context) (string, error)
	ListPRs(ctx context.Context, dir string) ([]github.PR, error)
	RepoName(ctx context.Context, dir string) (string, error)
}

// prDTO is the wire PR shape (docs/API.md): a projection of github.PR that omits
// the classification-only fields.
type prDTO struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	Author         string `json:"author"`
	URL            string `json:"url"`
	IsDraft        bool   `json:"is_draft"`
	UpdatedAt      string `json:"updated_at"`
	ReviewDecision string `json:"review_decision"`
	Checks         string `json:"checks"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
}

// bucketsDTO is the classified-PR grouping for one repository.
type bucketsDTO struct {
	NeedsReview []prDTO `json:"needs_review"`
	ReReview    []prDTO `json:"re_review"`
	Reviewed    []prDTO `json:"reviewed"`
	Mine        []prDTO `json:"mine"`
}

// reviewRepoDTO is one watched repository's classified PRs.
type reviewRepoDTO struct {
	Dir           string     `json:"dir"`
	NameWithOwner string     `json:"name_with_owner"`
	Buckets       bucketsDTO `json:"buckets"`
}

// reviewsResponse is the GET /reviews body.
type reviewsResponse struct {
	Login  string          `json:"login"`
	Repos  []reviewRepoDTO `json:"repos"`
	Errors []string        `json:"errors"`
}

// sourcesResponse is the body for both GET and POST /reviews/sources.
type sourcesResponse struct {
	Dirs []string `json:"dirs"`
}

// handleReviews lists open PRs across every watched repository, classified by
// what they need from the user. GitHub failures are surfaced in errors[] rather
// than failing the whole request: a single unreachable repo never 500s.
func (h *Handlers) handleReviews(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dirs, err := h.reviewSources(ctx)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}

	var errs []string
	login, err := h.github.Login(ctx)
	if err != nil {
		errs = append(errs, fmt.Sprintf("gh login: %v", err))
		login = ""
	}

	seenHeads, err := h.loadReviewHeads(ctx)
	if err != nil {
		h.logger.Warn("load review heads", "err", err)
		seenHeads = map[string]string{}
	}

	repos, repoErrs, newHeads := h.gatherRepos(ctx, dirs, login, seenHeads)
	errs = append(errs, repoErrs...)

	if err := h.saveReviewHeads(ctx, newHeads); err != nil {
		h.logger.Warn("save review heads", "err", err)
	}

	writeJSON(w, h.logger, http.StatusOK, reviewsResponse{
		Login:  login,
		Repos:  repos,
		Errors: orEmptyStrings(errs),
	})
}

// repoResult is one repository's outcome from the parallel gather.
type repoResult struct {
	repo  *reviewRepoDTO
	err   string            // "dir: message" when the repo could not be read
	heads map[string]string // "owner/repo#pr" -> current head oid
}

// gatherRepos queries each watched repository concurrently (bounded by
// reviewConcurrency) and assembles results in the input order for a stable
// response. It returns the classified repos, per-repo error strings, and the
// merged head-oid state to persist.
func (h *Handlers) gatherRepos(
	ctx context.Context, dirs []string, login string, seenHeads map[string]string,
) ([]reviewRepoDTO, []string, map[string]string) {
	results := make([]repoResult, len(dirs))
	sem := make(chan struct{}, reviewConcurrency)
	var wg sync.WaitGroup
	for i, dir := range dirs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = h.fetchRepo(ctx, dir, login, seenHeads)
		}()
	}
	wg.Wait()

	repos := make([]reviewRepoDTO, 0, len(dirs))
	var errs []string
	newHeads := make(map[string]string, len(seenHeads))
	for k, v := range seenHeads {
		newHeads[k] = v
	}
	for _, res := range results {
		if res.err != "" {
			errs = append(errs, res.err)
			continue
		}
		repos = append(repos, *res.repo)
		for k, v := range res.heads {
			newHeads[k] = v
		}
	}
	return repos, errs, newHeads
}

// fetchRepo resolves one repository's name and open PRs, classifies them against
// the user's prior head-oid sightings, and reports the current heads back for
// persistence.
func (h *Handlers) fetchRepo(ctx context.Context, dir, login string, seenHeads map[string]string) repoResult {
	name, err := h.github.RepoName(ctx, dir)
	if err != nil {
		return repoResult{err: fmt.Sprintf("%s: %v", dir, err)}
	}
	prs, err := h.github.ListPRs(ctx, dir)
	if err != nil {
		return repoResult{err: fmt.Sprintf("%s: %v", dir, err)}
	}

	perRepo := make(map[int]string)
	heads := make(map[string]string, len(prs))
	for _, pr := range prs {
		key := headKey(name, pr.Number)
		if oid, ok := seenHeads[key]; ok {
			perRepo[pr.Number] = oid
		}
		heads[key] = pr.HeadRefOid
	}

	repo := reviewRepoDTO{
		Dir:           dir,
		NameWithOwner: name,
		Buckets:       bucketsToDTO(github.Classify(prs, login, perRepo)),
	}
	return repoResult{repo: &repo, heads: heads}
}

// handleReviewSources returns the current watched repositories (seeding them on
// first read).
func (h *Handlers) handleReviewSources(w http.ResponseWriter, r *http.Request) {
	dirs, err := h.reviewSources(r.Context())
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, sourcesResponse{Dirs: orEmptyStrings(dirs)})
}

// handleSetReviewSources replaces the watched repositories. Each entry must be
// an absolute path to an existing git repository; the first invalid one yields a
// 400 naming it.
func (h *Handlers) handleSetReviewSources(w http.ResponseWriter, r *http.Request) {
	var req sourcesResponse
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	normalized := make([]string, 0, len(req.Dirs))
	seen := make(map[string]bool, len(req.Dirs))
	for _, dir := range req.Dirs {
		clean, err := validateGitDir(dir)
		if err != nil {
			writeErrorStatus(w, h.logger, http.StatusBadRequest, err.Error())
			return
		}
		if seen[clean] {
			continue
		}
		seen[clean] = true
		normalized = append(normalized, clean)
	}
	if err := h.saveReviewDirs(r.Context(), normalized); err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, sourcesResponse{Dirs: normalized})
}

// startReviewRequest is the POST /reviews/start body.
type startReviewRequest struct {
	Dir   string `json:"dir"`
	PR    int    `json:"pr"`
	Title string `json:"title"`
}

// handleReviewStart spawns a review task node for one PR: it finds (or creates)
// a project for the repository under the workspace root and hangs a read-only
// review task off it. A task cannot parent onto the workspace directly, hence
// the intermediate project.
func (h *Handlers) handleReviewStart(w http.ResponseWriter, r *http.Request) {
	var req startReviewRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	if !filepath.IsAbs(req.Dir) {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, fmt.Sprintf("dir must be an absolute path: %q", req.Dir))
		return
	}
	if req.PR <= 0 {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "pr must be a positive pull request number")
		return
	}
	root, ok := h.tree.Root()
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusInternalServerError, "workspace root not found")
		return
	}
	ctx := r.Context()
	name, err := h.github.RepoName(ctx, req.Dir)
	if err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, fmt.Sprintf("resolve repo name: %v", err))
		return
	}
	project, err := h.findOrCreateProject(ctx, root, name, req.Dir)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	title := req.Title
	if title == "" {
		title = fmt.Sprintf("Review %s#%d", name, req.PR)
	}
	task, err := h.tree.CreateNode(ctx, tree.CreateSpec{
		ParentID: project.ID,
		Kind:     core.KindTask,
		Title:    title,
		Brief:    reviewBrief(name, req.PR),
		WorkDir:  req.Dir,
	})
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusCreated, NodeToDTO(task))
}

// findOrCreateProject returns the project child of root whose title matches the
// repository name, creating one rooted at dir when none exists.
func (h *Handlers) findOrCreateProject(ctx context.Context, root core.Node, name, dir string) (core.Node, error) {
	for _, child := range h.tree.Children(root.ID) {
		if child.Kind == core.KindProject && child.Title == name {
			return child, nil
		}
	}
	return h.tree.CreateNode(ctx, tree.CreateSpec{
		ParentID: root.ID,
		Kind:     core.KindProject,
		Title:    name,
		WorkDir:  dir,
	})
}

// reviewBrief is the read-only review instruction briefed onto a review task.
func reviewBrief(name string, pr int) string {
	return fmt.Sprintf(
		"Read-only review of pull request %s#%d. Do not modify files, commit, or push. "+
			"Start with `gh pr diff %d` to read the changes, assess them for correctness, "+
			"clarity, and risk, and summarize your findings.",
		name, pr, pr,
	)
}

// reviewSources returns the watched repositories, seeding them on first use from
// the distinct git work_dir values across the tree.
func (h *Handlers) reviewSources(ctx context.Context) ([]string, error) {
	raw, ok, err := h.store.GetSetting(ctx, settingReviewDirs)
	if err != nil {
		return nil, err
	}
	if ok && raw != "" {
		var dirs []string
		if err := json.Unmarshal([]byte(raw), &dirs); err != nil {
			return nil, fmt.Errorf("parse %s setting: %w", settingReviewDirs, err)
		}
		if len(dirs) > 0 {
			return dirs, nil
		}
	}
	seeded := h.seedReviewDirs()
	if len(seeded) == 0 {
		// Nothing to seed yet; do not persist an empty set so a later git
		// work_dir can still seed it.
		return seeded, nil
	}
	if err := h.saveReviewDirs(ctx, seeded); err != nil {
		return nil, err
	}
	return seeded, nil
}

// seedReviewDirs collects the distinct, git-backed work_dir values set on nodes
// across the tree, preserving snapshot order.
func (h *Handlers) seedReviewDirs() []string {
	snap := h.tree.Snapshot()
	seen := make(map[string]bool)
	var dirs []string
	for _, n := range snap.Nodes {
		if n.WorkDir == "" || seen[n.WorkDir] {
			continue
		}
		seen[n.WorkDir] = true
		if isGitRepo(n.WorkDir) {
			dirs = append(dirs, n.WorkDir)
		}
	}
	return dirs
}

// saveReviewDirs persists the watched repositories as a JSON array.
func (h *Handlers) saveReviewDirs(ctx context.Context, dirs []string) error {
	buf, err := json.Marshal(dirs)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", settingReviewDirs, err)
	}
	return h.store.SetSetting(ctx, settingReviewDirs, string(buf))
}

// loadReviewHeads reads the persisted per-PR head-oid state.
func (h *Handlers) loadReviewHeads(ctx context.Context) (map[string]string, error) {
	raw, ok, err := h.store.GetSetting(ctx, settingReviewHeads)
	if err != nil {
		return nil, err
	}
	heads := map[string]string{}
	if ok && raw != "" {
		if err := json.Unmarshal([]byte(raw), &heads); err != nil {
			return nil, fmt.Errorf("parse %s setting: %w", settingReviewHeads, err)
		}
	}
	return heads, nil
}

// saveReviewHeads persists the per-PR head-oid state.
func (h *Handlers) saveReviewHeads(ctx context.Context, heads map[string]string) error {
	buf, err := json.Marshal(heads)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", settingReviewHeads, err)
	}
	return h.store.SetSetting(ctx, settingReviewHeads, string(buf))
}

// headKey is the review_heads map key for one repository's PR.
func headKey(nameWithOwner string, pr int) string {
	return nameWithOwner + "#" + strconv.Itoa(pr)
}

// isGitRepo reports whether dir is a git repository, i.e. it contains a .git
// entry (a directory for a normal clone, a file for a linked worktree).
func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// validateGitDir enforces the POST /reviews/sources rule for one source: an
// absolute path to an existing git repository. It returns the cleaned path.
func validateGitDir(dir string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("path is not absolute: %q", dir)
	}
	clean := filepath.Clean(dir)
	info, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("not an existing directory: %q", dir)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %q", dir)
	}
	if !isGitRepo(clean) {
		return "", fmt.Errorf("not a git repository: %q", dir)
	}
	return clean, nil
}

// bucketsToDTO maps classified PRs to their wire grouping, always returning
// non-nil bucket slices.
func bucketsToDTO(b github.Buckets) bucketsDTO {
	return bucketsDTO{
		NeedsReview: prsToDTO(b.NeedsReview),
		ReReview:    prsToDTO(b.ReReview),
		Reviewed:    prsToDTO(b.Reviewed),
		Mine:        prsToDTO(b.Mine),
	}
}

// prsToDTO maps a slice of PRs to the wire shape, always returning a non-nil slice.
func prsToDTO(prs []github.PR) []prDTO {
	out := make([]prDTO, 0, len(prs))
	for _, pr := range prs {
		out = append(out, prDTO{
			Number:         pr.Number,
			Title:          pr.Title,
			Author:         pr.Author,
			URL:            pr.URL,
			IsDraft:        pr.IsDraft,
			UpdatedAt:      pr.UpdatedAt,
			ReviewDecision: pr.ReviewDecision,
			Checks:         pr.Checks,
			Additions:      pr.Additions,
			Deletions:      pr.Deletions,
		})
	}
	return out
}

// orEmptyStrings returns a non-nil slice so JSON encodes [] rather than null.
func orEmptyStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
