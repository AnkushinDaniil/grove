package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AnkushinDaniil/grove/internal/github"
	"github.com/AnkushinDaniil/grove/internal/store"
)

// aiDraftFunc runs a headless claude in dir with prompt and returns the drafted
// text. It is the swappable seam behind POST /reviews/pr/ai-draft.
type aiDraftFunc func(ctx context.Context, dir, prompt string) (string, error)

// aiDraftTimeout bounds one headless claude drafting call. Generous headroom:
// even a fast model over a sizable diff can take a minute-plus.
const aiDraftTimeout = 240 * time.Second

// maxPromptDiffBytes caps how much diff text is embedded in an ai-draft prompt.
// Kept modest on purpose: a review comment needs enough surrounding diff to be
// grounded, not the entire PR — and a smaller prompt drafts far faster (a 60KB
// prompt on a large model routinely blew past the old timeout).
const maxPromptDiffBytes = 20 * 1024

// aiDraftModel is the model used for drafting review text. A fast mid-tier
// model is the right tool: drafts are short, always human-edited, and speed
// matters for an interactive action. Overrideable via GROVE_AI_DRAFT_MODEL.
const aiDraftModel = "sonnet"

// prReviewGitHub is the gh-backed capability the interactive review workspace
// needs beyond the Review Radar's GitHubClient. It is asserted from the injected
// client (h.github) rather than widening GitHubClient, which belongs to another
// concern (internal/api/reviews.go).
type prReviewGitHub interface {
	PRDetail(ctx context.Context, dir string, pr int) (github.PRReview, error)
	SubmitReview(ctx context.Context, dir string, in github.SubmitInput) (string, error)
	ReplyToThread(ctx context.Context, dir string, in github.ReplyInput) error
}

// prGitHub asserts the injected client up to the PR-review capability.
func (h *Handlers) prGitHub() (prReviewGitHub, bool) {
	gh, ok := h.github.(prReviewGitHub)
	return gh, ok
}

// --- wire DTOs (docs/API.md "Interactive review workspace") ---

type prReviewDTO struct {
	Number         int         `json:"number"`
	Title          string      `json:"title"`
	Author         string      `json:"author"`
	URL            string      `json:"url"`
	State          string      `json:"state"`
	HeadSHA        string      `json:"head_sha"`
	BaseRef        string      `json:"base_ref"`
	Checks         string      `json:"checks"`
	ReviewDecision string      `json:"review_decision"`
	Body           string      `json:"body"`
	Files          []fileDTO   `json:"files"`
	Threads        []threadDTO `json:"threads"`
}

type fileDTO struct {
	Path            string    `json:"path"`
	OldPath         string    `json:"old_path"`
	Status          string    `json:"status"`
	Additions       int       `json:"additions"`
	Deletions       int       `json:"deletions"`
	Binary          bool      `json:"binary"`
	OriginalContent string    `json:"original_content"`
	ModifiedContent string    `json:"modified_content"`
	ContentOmitted  string    `json:"content_omitted"`
	Hunks           []hunkDTO `json:"hunks"`
}

type hunkDTO struct {
	Header string    `json:"header"`
	Lines  []lineDTO `json:"lines"`
}

type lineDTO struct {
	Op      string `json:"op"`
	OldLine int    `json:"old_line"`
	NewLine int    `json:"new_line"`
	Text    string `json:"text"`
}

type threadDTO struct {
	ID         string             `json:"id"`
	Path       string             `json:"path"`
	Line       int                `json:"line"`
	Side       string             `json:"side"`
	IsResolved bool               `json:"is_resolved"`
	DiffHunk   string             `json:"diff_hunk"`
	Comments   []threadCommentDTO `json:"comments"`
}

type threadCommentDTO struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	IsMine    bool   `json:"is_mine"`
}

// draftCommentDTO is the wire shape of a pending review comment.
type draftCommentDTO struct {
	ID        string `json:"id"`
	Dir       string `json:"dir"`
	PR        int    `json:"pr"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// prReviewToDTO maps the internal PR view to its wire shape.
func prReviewToDTO(pr github.PRReview) prReviewDTO {
	files := make([]fileDTO, 0, len(pr.Files))
	for _, f := range pr.Files {
		files = append(files, fileToDTO(f))
	}
	threads := make([]threadDTO, 0, len(pr.Threads))
	for _, t := range pr.Threads {
		threads = append(threads, threadToDTO(t))
	}
	return prReviewDTO{
		Number:         pr.Number,
		Title:          pr.Title,
		Author:         pr.Author,
		URL:            pr.URL,
		State:          pr.State,
		HeadSHA:        pr.HeadSHA,
		BaseRef:        pr.BaseRef,
		Checks:         pr.Checks,
		ReviewDecision: pr.ReviewDecision,
		Body:           pr.Body,
		Files:          files,
		Threads:        threads,
	}
}

func fileToDTO(f github.PRFile) fileDTO {
	hunks := make([]hunkDTO, 0, len(f.Hunks))
	for _, hk := range f.Hunks {
		lines := make([]lineDTO, 0, len(hk.Lines))
		for _, ln := range hk.Lines {
			lines = append(lines, lineDTO{Op: ln.Op, OldLine: ln.OldLine, NewLine: ln.NewLine, Text: ln.Text})
		}
		hunks = append(hunks, hunkDTO{Header: hk.Header, Lines: lines})
	}
	return fileDTO{
		Path:            f.Path,
		OldPath:         f.OldPath,
		Status:          f.Status,
		Additions:       f.Additions,
		Deletions:       f.Deletions,
		Binary:          f.Binary,
		OriginalContent: f.OriginalContent,
		ModifiedContent: f.ModifiedContent,
		ContentOmitted:  f.ContentOmitted,
		Hunks:           hunks,
	}
}

func threadToDTO(t github.Thread) threadDTO {
	comments := make([]threadCommentDTO, 0, len(t.Comments))
	for _, c := range t.Comments {
		comments = append(comments, threadCommentDTO{
			ID: c.ID, Author: c.Author, Body: c.Body, CreatedAt: c.CreatedAt, IsMine: c.IsMine,
		})
	}
	return threadDTO{
		ID:         t.ID,
		Path:       t.Path,
		Line:       t.Line,
		Side:       t.Side,
		IsResolved: t.IsResolved,
		DiffHunk:   t.DiffHunk,
		Comments:   comments,
	}
}

func draftToDTO(d store.ReviewDraft) draftCommentDTO {
	return draftCommentDTO{
		ID:        d.ID,
		Dir:       d.Dir,
		PR:        d.PR,
		Path:      d.Path,
		Line:      d.Line,
		Side:      d.Side,
		Body:      d.Body,
		CreatedAt: rfc3339(d.CreatedAt),
	}
}

// --- handlers ---

// handlePRReview returns the full review view of one PR: metadata, diff, and
// threads, assembled via gh.
func (h *Handlers) handlePRReview(w http.ResponseWriter, r *http.Request) {
	dir, pr, ok := h.queryDirPR(w, r)
	if !ok {
		return
	}
	gh, ok := h.prGitHub()
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusInternalServerError, "github client does not support PR review")
		return
	}
	detail, err := gh.PRDetail(r.Context(), dir, pr)
	if err != nil {
		h.writeGHError(w, err)
		return
	}
	writeJSON(w, h.logger, http.StatusOK, prReviewToDTO(detail))
}

// handleListDrafts returns the pending drafts for one review workspace.
func (h *Handlers) handleListDrafts(w http.ResponseWriter, r *http.Request) {
	dir, pr, ok := h.queryDirPR(w, r)
	if !ok {
		return
	}
	drafts, err := h.store.ListReviewDrafts(r.Context(), dir, pr)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	out := make([]draftCommentDTO, 0, len(drafts))
	for _, d := range drafts {
		out = append(out, draftToDTO(d))
	}
	writeJSON(w, h.logger, http.StatusOK, map[string][]draftCommentDTO{"drafts": out})
}

type createDraftRequest struct {
	Dir  string `json:"dir"`
	PR   int    `json:"pr"`
	Path string `json:"path"`
	Line int    `json:"line"`
	Side string `json:"side"`
	Body string `json:"body"`
}

// handleCreateDraft validates and stores one pending review comment.
func (h *Handlers) handleCreateDraft(w http.ResponseWriter, r *http.Request) {
	var req createDraftRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	dir, err := validateGitDir(req.Dir)
	if err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, err.Error())
		return
	}
	if req.PR <= 0 {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "pr must be a positive pull request number")
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "body must not be empty")
		return
	}
	if req.Path == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "path must not be empty")
		return
	}
	side, ok := normalizeSide(req.Side)
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "side must be RIGHT or LEFT")
		return
	}
	draft := store.ReviewDraft{
		ID:        uuid.Must(uuid.NewV7()).String(),
		Dir:       dir,
		PR:        req.PR,
		Path:      req.Path,
		Line:      req.Line,
		Side:      side,
		Body:      req.Body,
		CreatedAt: time.Now(),
	}
	if err := h.store.SaveReviewDraft(r.Context(), draft); err != nil {
		writeError(w, h.logger, err)
		return
	}
	writeJSON(w, h.logger, http.StatusCreated, draftToDTO(draft))
}

// handleDeleteDraft removes one pending draft. Deletion is idempotent: an
// unknown id still returns 204.
func (h *Handlers) handleDeleteDraft(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "draft id is required")
		return
	}
	if err := h.store.DeleteReviewDraft(r.Context(), id); err != nil {
		writeError(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type aiDraftRequest struct {
	Dir         string `json:"dir"`
	PR          int    `json:"pr"`
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	ThreadID    string `json:"thread_id"`
	Instruction string `json:"instruction"`
}

// handleAIDraft runs a headless claude over the PR context and returns editable
// suggested review text. The human always reviews it before it becomes a draft
// or reply. Worktree review reuses this endpoint with pr=0 and the worktree path
// as dir, grounding the prompt in the local diff instead of a PR's.
func (h *Handlers) handleAIDraft(w http.ResponseWriter, r *http.Request) {
	var req aiDraftRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	if !filepath.IsAbs(req.Dir) {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "dir must be an absolute path")
		return
	}
	if req.PR < 0 {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "pr must not be negative")
		return
	}
	if req.Kind != "comment" && req.Kind != "reply" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "kind must be comment or reply")
		return
	}

	prompt, ok := h.aiDraftPrompt(w, r, req)
	if !ok {
		return
	}

	drafter := h.aiDrafter
	if drafter == nil {
		drafter = defaultAIDrafter
	}
	text, err := drafter(r.Context(), req.Dir, prompt)
	if err != nil {
		h.writeGHError(w, fmt.Errorf("ai draft: %w", err))
		return
	}
	writeJSON(w, h.logger, http.StatusOK, map[string]string{"text": strings.TrimSpace(text)})
}

// aiDraftPrompt builds the drafting prompt for one request. When pr==0 the diff
// context comes from the local worktree (dir) via git; otherwise it comes from
// the PR's assembled diff via gh. It writes the appropriate error response and
// returns ok=false on failure.
func (h *Handlers) aiDraftPrompt(w http.ResponseWriter, r *http.Request, req aiDraftRequest) (string, bool) {
	if req.PR == 0 {
		diff := h.localDiffForPath(r.Context(), req.Dir, req.Path)
		return buildWorktreeDraftPrompt(req, diff), true
	}
	gh, ok := h.prGitHub()
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusInternalServerError, "github client does not support PR review")
		return "", false
	}
	detail, err := gh.PRDetail(r.Context(), req.Dir, req.PR)
	if err != nil {
		h.writeGHError(w, err)
		return "", false
	}
	return buildAIDraftPrompt(req, detail), true
}

type submitRequest struct {
	Dir      string   `json:"dir"`
	PR       int      `json:"pr"`
	Event    string   `json:"event"`
	Body     string   `json:"body"`
	DraftIDs []string `json:"draft_ids"`
}

// handleSubmitReview posts one batch review (event + body + the referenced
// drafts as inline comments) via gh, then clears those drafts. Drafts whose path
// is not in the PR's changed files are unanchorable and rejected with 400 before
// anything is posted.
func (h *Handlers) handleSubmitReview(w http.ResponseWriter, r *http.Request) {
	var req submitRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	if !filepath.IsAbs(req.Dir) {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "dir must be an absolute path")
		return
	}
	if req.PR <= 0 {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "pr must be a positive pull request number")
		return
	}
	if !validSubmitEvent(req.Event) {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "event must be APPROVE, REQUEST_CHANGES or COMMENT")
		return
	}
	gh, ok := h.prGitHub()
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusInternalServerError, "github client does not support PR review")
		return
	}
	ctx := r.Context()
	drafts, err := h.store.ListReviewDraftsByIDs(ctx, req.DraftIDs)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}

	// Anchor validation only needs the PR files when there are drafts to place.
	if len(drafts) > 0 {
		detail, err := gh.PRDetail(ctx, req.Dir, req.PR)
		if err != nil {
			h.writeGHError(w, err)
			return
		}
		if unanchorable := unanchorableDrafts(drafts, detail); len(unanchorable) > 0 {
			writeErrorStatus(w, h.logger, http.StatusBadRequest,
				"drafts do not anchor to a changed file: "+strings.Join(unanchorable, ", "))
			return
		}
	}

	comments := make([]github.ReviewComment, 0, len(drafts))
	for _, d := range drafts {
		comments = append(comments, github.ReviewComment{Path: d.Path, Line: d.Line, Side: d.Side, Body: d.Body})
	}
	url, err := gh.SubmitReview(ctx, req.Dir, github.SubmitInput{
		PR: req.PR, Event: req.Event, Body: req.Body, Comments: comments,
	})
	if err != nil {
		h.writeGHError(w, err)
		return
	}
	for _, d := range drafts {
		if err := h.store.DeleteReviewDraft(ctx, d.ID); err != nil {
			h.logger.Warn("delete submitted draft", "id", d.ID, "err", err)
		}
	}
	writeJSON(w, h.logger, http.StatusOK, map[string]string{"url": url})
}

type replyRequest struct {
	Dir      string `json:"dir"`
	PR       int    `json:"pr"`
	ThreadID string `json:"thread_id"`
	Body     string `json:"body"`
	Resolve  bool   `json:"resolve"`
}

// handleReplyThread posts a reply to an existing review thread, optionally
// resolving it.
func (h *Handlers) handleReplyThread(w http.ResponseWriter, r *http.Request) {
	var req replyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	if !filepath.IsAbs(req.Dir) {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "dir must be an absolute path")
		return
	}
	if req.ThreadID == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "thread_id must not be empty")
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "body must not be empty")
		return
	}
	gh, ok := h.prGitHub()
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusInternalServerError, "github client does not support PR review")
		return
	}
	if err := gh.ReplyToThread(r.Context(), req.Dir, github.ReplyInput{
		ThreadID: req.ThreadID, Body: req.Body, Resolve: req.Resolve,
	}); err != nil {
		h.writeGHError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

// queryDirPR parses and validates the shared ?dir=&pr= query used by the read
// endpoints, writing a 400 and returning ok=false on failure.
func (h *Handlers) queryDirPR(w http.ResponseWriter, r *http.Request) (string, int, bool) {
	dir := r.URL.Query().Get("dir")
	if !filepath.IsAbs(dir) {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "dir must be an absolute path")
		return "", 0, false
	}
	pr, err := strconv.Atoi(r.URL.Query().Get("pr"))
	if err != nil || pr <= 0 {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "pr must be a positive pull request number")
		return "", 0, false
	}
	return dir, pr, true
}

// writeGHError reports an upstream gh/claude failure as 502 Bad Gateway with the
// underlying message, logging it for the operator.
func (h *Handlers) writeGHError(w http.ResponseWriter, err error) {
	h.logger.Warn("gh request failed", "err", err)
	writeErrorStatus(w, h.logger, http.StatusBadGateway, err.Error())
}

// normalizeSide defaults an empty side to RIGHT and rejects anything other than
// RIGHT/LEFT.
func normalizeSide(side string) (string, bool) {
	switch side {
	case "":
		return "RIGHT", true
	case "RIGHT", "LEFT":
		return side, true
	default:
		return "", false
	}
}

// validSubmitEvent reports whether event is one of the review verdicts GitHub
// accepts.
func validSubmitEvent(event string) bool {
	switch event {
	case "APPROVE", "REQUEST_CHANGES", "COMMENT":
		return true
	default:
		return false
	}
}

// unanchorableDrafts returns the ids of drafts whose path is not among the PR's
// changed files, so submit can reject them with a clear message instead of a
// confusing gh error.
func unanchorableDrafts(drafts []store.ReviewDraft, detail github.PRReview) []string {
	changed := make(map[string]bool, len(detail.Files))
	for _, f := range detail.Files {
		changed[f.Path] = true
	}
	var bad []string
	for _, d := range drafts {
		if !changed[d.Path] {
			bad = append(bad, d.ID)
		}
	}
	return bad
}

// buildAIDraftPrompt builds the headless-claude prompt for one ai-draft request,
// embedding the target file's diff (kind=comment) or the thread's context
// (kind=reply) plus any user instruction. It is pure so it can be unit-tested.
func buildAIDraftPrompt(req aiDraftRequest, pr github.PRReview) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are helping review pull request #%d %q.\n", pr.Number, pr.Title)

	if req.Kind == "reply" {
		fmt.Fprintf(&b, "Write a reply to the review thread at %s:%d.\n", req.Path, req.Line)
		if thread := findThread(pr, req.ThreadID); thread != nil {
			b.WriteString("\nThe thread so far:\n")
			for _, c := range thread.Comments {
				fmt.Fprintf(&b, "%s: %s\n", c.Author, c.Body)
			}
			if thread.DiffHunk != "" {
				b.WriteString("\nThe anchored diff hunk:\n")
				b.WriteString(capText(thread.DiffHunk, maxPromptDiffBytes))
				b.WriteString("\n")
			}
		}
	} else {
		fmt.Fprintf(&b, "Write a concise, specific code review comment for %s", req.Path)
		if req.Line > 0 {
			fmt.Fprintf(&b, ":%d", req.Line)
		}
		b.WriteString(".\n\nThe diff under review:\n")
		b.WriteString(capText(diffForPrompt(pr, req.Path), maxPromptDiffBytes))
		b.WriteString("\n")
	}

	if strings.TrimSpace(req.Instruction) != "" {
		fmt.Fprintf(&b, "\nAdditional instruction: %s\n", req.Instruction)
	}
	b.WriteString("\nRespond with only the comment text, no preamble.")
	return b.String()
}

// diffForPrompt renders the diff to embed: the single target file when path
// names one of the PR's files, otherwise every file's diff concatenated.
func diffForPrompt(pr github.PRReview, path string) string {
	for _, f := range pr.Files {
		if f.Path == path {
			return renderFileDiff(f)
		}
	}
	var b strings.Builder
	for _, f := range pr.Files {
		b.WriteString(renderFileDiff(f))
	}
	return b.String()
}

// renderFileDiff reconstructs a file's unified diff text from its parsed hunks.
func renderFileDiff(f github.PRFile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n", f.Path)
	for _, hk := range f.Hunks {
		b.WriteString(hk.Header)
		b.WriteString("\n")
		for _, ln := range hk.Lines {
			b.WriteString(ln.Op)
			b.WriteString(ln.Text)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// findThread returns the thread with id in pr, or nil.
func findThread(pr github.PRReview, id string) *github.Thread {
	for i := range pr.Threads {
		if pr.Threads[i].ID == id {
			return &pr.Threads[i]
		}
	}
	return nil
}

// capText truncates s to at most maxBytes, appending a marker when it was cut.
func capText(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes] + "\n… (diff truncated)"
}

// claudeShimMarkers identify PATH entries belonging to third-party CLI shims
// that intercept `claude`. Kept in sync with internal/session/env.go, which
// scrubs the same markers so grove always talks to the real CLI (duplicated
// here to avoid coupling the api package to internal/session).
var claudeShimMarkers = []string{"cmux-cli-shims", "/cmux.app/"}

// defaultAIDrafter runs the real `claude -p` in dir with a scrubbed PATH and a
// bounded timeout, returning trimmed stdout.
func defaultAIDrafter(ctx context.Context, dir, prompt string) (string, error) {
	// Detach from the caller's request context so a brief client disconnect (a
	// re-render, a navigation, a flaky socket) doesn't SIGKILL claude mid-draft;
	// only our own timeout bounds it.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), aiDraftTimeout)
	defer cancel()

	model := aiDraftModel
	if m := os.Getenv("GROVE_AI_DRAFT_MODEL"); m != "" {
		model = m
	}
	//nolint:gosec // G204: binary is the fixed literal "claude"; prompt and model are arguments, not the command.
	cmd := exec.CommandContext(ctx, "claude", claudeDraftArgs(prompt, model)...)
	cmd.Dir = dir
	cmd.Env = scrubClaudePATH(os.Environ())

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// A deadline hit surfaces from cmd.Run as "signal: killed" (CommandContext
		// SIGKILLs the child); translate it to something actionable instead.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("claude timed out after %s — the diff may be too large to review in one pass", aiDraftTimeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", fmt.Errorf("claude -p: %w", err)
		}
		return "", fmt.Errorf("claude -p: %w: %s", err, msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// draftSystemPrompt replaces claude's default system prompt for headless
// drafting/review calls. The default one, plus the user's global CLAUDE.md
// (e.g. an orchestration layer that says "delegate to agents, report findings
// via tools"), makes the model behave like an agent: it narrates ("Findings
// reported above…") or tries to delegate instead of just printing the answer,
// so the output isn't the JSON/text the endpoint parses. A focused system
// prompt strips all that.
const draftSystemPrompt = "You are a precise code-review assistant with no tools and no ability to delegate. " +
	"Analyze only what the user provides and reply with exactly the output they request — raw, " +
	"with no preamble, no narration, and no tool use."

// claudeDraftArgs builds the argv for a headless drafting/review call. These
// flags are load-bearing — they keep claude answering directly from the prompt
// instead of going agentic, which is what killed ai-review before:
//   - --system-prompt: a focused reviewer prompt (see draftSystemPrompt), so the
//     user's CLAUDE.md can't turn the call into an agent that narrates or
//     delegates instead of emitting the answer.
//   - --strict-mcp-config (with no --mcp-config): load no MCP servers, so an
//     open-ended prompt can't explore them for minutes until the deadline
//     SIGKILLs the process ("claude -p: signal: killed").
//   - --disallowedTools: no repo/web exploration.
//
// Notably there is NO --max-turns cap: capping at 1 turn made claude exit 1
// (empty stderr) whenever the model needed a second turn on a real PR. With MCP
// and tools already gone there is nothing to explore, so the turns it takes are
// just reasoning, bounded by aiDraftTimeout.
func claudeDraftArgs(prompt, model string) []string {
	return []string{
		"-p", prompt,
		"--model", model,
		"--output-format", "text",
		"--system-prompt", draftSystemPrompt,
		"--strict-mcp-config",
		// Denylist last so its variadic value greedily takes exactly these tools.
		"--disallowedTools", "Bash", "Read", "Edit", "Write", "Glob", "Grep",
		"WebFetch", "WebSearch", "Task", "NotebookEdit", "TodoWrite",
	}
}

// scrubClaudePATH drops shim directories from the PATH entry of env so the real
// claude binary is invoked (see claudeShimMarkers).
func scrubClaudePATH(env []string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "PATH=") {
			out = append(out, e)
			continue
		}
		entries := strings.Split(strings.TrimPrefix(e, "PATH="), string(os.PathListSeparator))
		kept := entries[:0]
		for _, entry := range entries {
			if containsShimMarker(entry) {
				continue
			}
			kept = append(kept, entry)
		}
		out = append(out, "PATH="+strings.Join(kept, string(os.PathListSeparator)))
	}
	return out
}

func containsShimMarker(entry string) bool {
	for _, marker := range claudeShimMarkers {
		if strings.Contains(entry, marker) {
			return true
		}
	}
	return false
}
