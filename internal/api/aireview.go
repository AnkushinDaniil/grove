package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/AnkushinDaniil/grove/internal/github"
)

// AiFinding is one AI-proposed review comment anchored to a changed line,
// optionally carrying a single-line code suggestion. Findings are transient:
// they are never persisted server-side. The review UI holds the set returned by
// one ai-review pass and turns each accepted finding into a normal review draft
// (the suggestion, if any, becoming a GitHub ```suggestion block in the draft
// body), so the existing batch-submit path posts them unchanged.
type AiFinding struct {
	Path       string `json:"path"`
	Line       int    `json:"line"`
	Side       string `json:"side"`       // RIGHT (new file) or LEFT (old file)
	Severity   string `json:"severity"`   // issue | suggestion | nit
	Body       string `json:"body"`       // markdown comment, without a suggestion fence
	Suggestion string `json:"suggestion"` // replacement for the anchored line; "" when none
}

type aiReviewRequest struct {
	Dir string `json:"dir"`
	PR  int    `json:"pr"`
}

type aiReviewResponse struct {
	Findings []AiFinding `json:"findings"`
}

const (
	// maxAIReviewDiffBytes caps the whole-PR diff embedded in the review prompt.
	// Larger than a single-comment draft (maxPromptDiffBytes) because the pass
	// reasons over every file at once, but still bounded so a huge PR cannot blow
	// past the model's context window or the drafting timeout.
	maxAIReviewDiffBytes = 60 * 1024
	// maxAIReviewFindings caps how many findings one pass returns, so a model
	// that over-produces cannot flood the review with low-signal noise.
	maxAIReviewFindings = 40
	// maxAIFindingBodyBytes caps one finding's comment body.
	maxAIFindingBodyBytes = 4000
)

// validAIFindingSeverity is the closed set of severities the UI renders badges
// for; anything else normalizes to "suggestion".
var validAIFindingSeverity = map[string]bool{"issue": true, "suggestion": true, "nit": true}

// handleAIReview runs one headless claude pass over a PR's whole diff and returns
// structured, line-anchored findings (each an editable proposed comment, some
// with a code suggestion). Unlike ai-draft -- which drafts the text of a single
// comment the human already placed -- this proposes the comments themselves, so
// the reviewer accepts/edits/dismisses each in the findings panel. Findings are
// validated to anchor to a real changed line and never auto-post: turning one
// into a draft (and ultimately submitting it) is always the human's action.
//
// PR-only for now: worktree review (pr=0) computes its diff client-side and has
// no server-side hunks to anchor against, so its per-line "Draft with AI"
// (ai-draft) remains the path there.
func (h *Handlers) handleAIReview(w http.ResponseWriter, r *http.Request) {
	var req aiReviewRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "invalid request body")
		return
	}
	if !filepath.IsAbs(req.Dir) {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "dir must be an absolute path")
		return
	}
	if req.PR <= 0 {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "ai review requires a pull request number")
		return
	}
	gh, ok := h.prGitHub()
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusInternalServerError, "github client does not support PR review")
		return
	}
	detail, err := gh.PRDetail(r.Context(), req.Dir, req.PR)
	if err != nil {
		h.writeGHError(w, err)
		return
	}

	drafter := h.aiDrafter
	if drafter == nil {
		drafter = defaultAIDrafter
	}
	text, err := drafter(r.Context(), req.Dir, buildAIReviewPrompt(detail))
	if err != nil {
		h.writeGHError(w, fmt.Errorf("ai review: %w", err))
		return
	}

	findings, err := parseFindings(text)
	if err != nil {
		writeErrorStatus(w, h.logger, http.StatusBadGateway, "ai review returned unparseable output")
		return
	}
	writeJSON(w, h.logger, http.StatusOK, aiReviewResponse{Findings: anchorFindings(findings, detail)})
}

// buildAIReviewPrompt asks the model to review the whole PR diff and return a
// JSON array of line-anchored findings. It is deliberately strict about the
// output shape (JSON only) and about anchoring to lines that appear in the diff,
// because parseFindings/anchorFindings drop anything that does not conform.
func buildAIReviewPrompt(pr github.PRReview) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are a senior engineer reviewing pull request #%d %q.\n", pr.Number, pr.Title)
	b.WriteString("Review the diff below and report concrete, high-signal findings: correctness bugs, ")
	b.WriteString("missed edge cases, security or performance problems, and worthwhile simplifications. ")
	b.WriteString("Skip trivia and do not restate what the code does.\n\n")

	b.WriteString("The diff under review (unified format; each file starts with '--- <path>'):\n")
	b.WriteString(capText(fullDiffForPrompt(pr), maxAIReviewDiffBytes))
	b.WriteString("\n\n")

	b.WriteString("Respond with ONLY a JSON array (no prose, no markdown fences) of findings. Each element is:\n")
	b.WriteString(`{"path": string, "line": number, "side": "RIGHT"|"LEFT", "severity": "issue"|"suggestion"|"nit", "body": string, "suggestion": string}`)
	b.WriteString("\n")
	b.WriteString("- path: the exact file path from a '--- <path>' header.\n")
	b.WriteString("- line: a line number that appears in the diff. Use the NEW-file number for side RIGHT, the OLD-file number for side LEFT.\n")
	b.WriteString("- side: RIGHT for an added or context line (the default); LEFT only to comment on a removed line.\n")
	b.WriteString("- severity: issue (a bug or risk), suggestion (an improvement), or nit (minor/style).\n")
	b.WriteString("- body: a concise markdown explanation. Be specific; name the symbol rather than the line number.\n")
	b.WriteString("- suggestion: when a concrete one-line fix fits, the exact replacement text for that single line (code only, no ``` fences, no +/- diff markers). Otherwise an empty string.\n")
	fmt.Fprintf(&b, "Only comment on lines present in the diff. Return at most %d findings, highest-signal first. If nothing is worth flagging, return [].", maxAIReviewFindings)
	return b.String()
}

// fullDiffForPrompt renders every file's diff, concatenated.
func fullDiffForPrompt(pr github.PRReview) string {
	var b strings.Builder
	for _, f := range pr.Files {
		b.WriteString(renderFileDiff(f))
	}
	return b.String()
}

// parseFindings extracts the JSON array of findings from the model's raw output.
// Models sometimes wrap the array in ```json fences or add a sentence of
// preamble, so this strips fences and falls back to the first '[' … last ']'
// span before unmarshalling.
func parseFindings(text string) ([]AiFinding, error) {
	s := stripCodeFences(strings.TrimSpace(text))
	var findings []AiFinding
	if err := json.Unmarshal([]byte(s), &findings); err == nil {
		return findings, nil
	}
	start := strings.IndexByte(s, '[')
	end := strings.LastIndexByte(s, ']')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON array in model output")
	}
	if err := json.Unmarshal([]byte(s[start:end+1]), &findings); err != nil {
		return nil, fmt.Errorf("parse findings: %w", err)
	}
	return findings, nil
}

// stripCodeFences removes a single wrapping ```… fence (e.g. ```json) when the
// whole string is fenced, so a model that wraps its JSON in a code block still
// parses.
func stripCodeFences(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if nl := strings.IndexByte(s, '\n'); nl >= 0 {
		s = s[nl+1:]
	}
	s = strings.TrimRight(s, " \n\t")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// anchorFindings keeps only findings that anchor to a real changed line, and
// normalizes their side/severity and caps their body. A model can hallucinate
// paths or line numbers, so an unanchored finding is dropped rather than shown
// (it would fail at submit time anyway). A finding carrying a suggestion is
// forced onto the RIGHT side, since a GitHub suggestion replaces a line in the
// new file.
func anchorFindings(findings []AiFinding, pr github.PRReview) []AiFinding {
	anchors := buildAnchorSet(pr)
	out := make([]AiFinding, 0, len(findings))
	seen := make(map[string]bool)
	for _, f := range findings {
		body := strings.TrimSpace(f.Body)
		if body == "" || f.Line <= 0 {
			continue
		}
		side, ok := normalizeSide(f.Side)
		if !ok {
			side = "RIGHT"
		}
		suggestion := strings.TrimSpace(f.Suggestion)
		if suggestion != "" {
			side = "RIGHT"
		}
		if !anchors[anchorKey(f.Path, side, f.Line)] {
			continue
		}
		key := anchorKey(f.Path, side, f.Line) + "\x00" + body
		if seen[key] {
			continue
		}
		seen[key] = true
		severity := f.Severity
		if !validAIFindingSeverity[severity] {
			severity = "suggestion"
		}
		out = append(out, AiFinding{
			Path:       f.Path,
			Line:       f.Line,
			Side:       side,
			Severity:   severity,
			Body:       capText(body, maxAIFindingBodyBytes),
			Suggestion: suggestion,
		})
		if len(out) >= maxAIReviewFindings {
			break
		}
	}
	return out
}

// buildAnchorSet indexes every (path, side, line) a review comment could anchor
// to: RIGHT for added/context lines (by new-file number), LEFT for
// removed/context lines (by old-file number).
func buildAnchorSet(pr github.PRReview) map[string]bool {
	set := make(map[string]bool)
	for _, f := range pr.Files {
		for _, hk := range f.Hunks {
			for _, ln := range hk.Lines {
				if ln.NewLine > 0 {
					set[anchorKey(f.Path, "RIGHT", ln.NewLine)] = true
				}
				if ln.OldLine > 0 {
					set[anchorKey(f.Path, "LEFT", ln.OldLine)] = true
				}
			}
		}
	}
	return set
}

func anchorKey(path, side string, line int) string {
	return fmt.Sprintf("%s\x00%s\x00%d", path, side, line)
}
