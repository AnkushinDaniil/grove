package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/github"
)

func TestParseFindings(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantLen int
		wantErr bool
	}{
		{"plain array", `[{"path":"a.go","line":1,"body":"x"}]`, 1, false},
		{"fenced json", "```json\n[{\"path\":\"a.go\",\"line\":1,\"body\":\"x\"}]\n```", 1, false},
		{"bare fence", "```\n[{\"path\":\"a.go\",\"line\":2,\"body\":\"y\"}]\n```", 1, false},
		{"preamble then array", "Here are the findings:\n[{\"path\":\"a.go\",\"line\":1,\"body\":\"x\"}]", 1, false},
		{"empty array", `[]`, 0, false},
		{"not json", "I could not review this.", 0, true},
		{"no array", `{"path":"a.go"}`, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseFindings(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseFindings(%q) = %v, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFindings(%q): %v", tc.in, err)
			}
			if len(got) != tc.wantLen {
				t.Errorf("len = %d, want %d", len(got), tc.wantLen)
			}
		})
	}
}

// anchoredPR is a PR whose single file has a RIGHT line 1 and a LEFT line 1.
func anchoredPR() github.PRReview {
	return github.PRReview{Number: 3, Title: "T", Files: []github.PRFile{{
		Path: "a.go",
		Hunks: []github.Hunk{{
			Header: "@@ -1 +1,2 @@",
			Lines: []github.DiffLine{
				{Op: "-", OldLine: 1, Text: "old"},
				{Op: "+", NewLine: 1, Text: "new one"},
				{Op: "+", NewLine: 2, Text: "new two"},
			},
		}},
	}}}
}

func TestAnchorFindingsDropsUnanchored(t *testing.T) {
	pr := anchoredPR()
	findings := []AiFinding{
		{Path: "a.go", Line: 1, Side: "RIGHT", Body: "keep, anchors"},
		{Path: "a.go", Line: 99, Side: "RIGHT", Body: "drop, no such line"},
		{Path: "ghost.go", Line: 1, Side: "RIGHT", Body: "drop, no such file"},
		{Path: "a.go", Line: 0, Side: "RIGHT", Body: "drop, no line"},
		{Path: "a.go", Line: 2, Side: "RIGHT", Body: "   "},
	}
	got := anchorFindings(findings, pr)
	if len(got) != 1 {
		t.Fatalf("kept %d findings, want 1: %+v", len(got), got)
	}
	if got[0].Line != 1 || got[0].Body != "keep, anchors" {
		t.Errorf("kept the wrong finding: %+v", got[0])
	}
}

func TestAnchorFindingsNormalizesAndSuggestionForcesRight(t *testing.T) {
	pr := anchoredPR()
	findings := []AiFinding{
		// Bad severity normalizes to "suggestion"; a suggestion forces side RIGHT
		// even though the model said LEFT, and RIGHT line 2 anchors.
		{Path: "a.go", Line: 2, Side: "LEFT", Severity: "wat", Body: "fix it", Suggestion: "new two fixed"},
	}
	got := anchorFindings(findings, pr)
	if len(got) != 1 {
		t.Fatalf("kept %d, want 1", len(got))
	}
	if got[0].Side != "RIGHT" {
		t.Errorf("side = %q, want RIGHT (suggestion edits the new file)", got[0].Side)
	}
	if got[0].Severity != "suggestion" {
		t.Errorf("severity = %q, want normalized to suggestion", got[0].Severity)
	}
}

func TestAnchorFindingsDedupes(t *testing.T) {
	pr := anchoredPR()
	dup := AiFinding{Path: "a.go", Line: 1, Side: "RIGHT", Body: "same"}
	got := anchorFindings([]AiFinding{dup, dup}, pr)
	if len(got) != 1 {
		t.Errorf("kept %d, want 1 after dedupe", len(got))
	}
}

func TestAnchorFindingsLeftSide(t *testing.T) {
	pr := anchoredPR()
	// A comment on the removed line anchors on LEFT:1 (and has no suggestion).
	got := anchorFindings([]AiFinding{{Path: "a.go", Line: 1, Side: "LEFT", Body: "why remove?"}}, pr)
	if len(got) != 1 || got[0].Side != "LEFT" {
		t.Fatalf("want one LEFT finding, got %+v", got)
	}
}

func TestHandleAIReviewHappyPath(t *testing.T) {
	gh := &fakePRGH{detail: anchoredPR()}
	h := newPRHarness(t, gh)
	dir := t.TempDir()

	var gotPrompt string
	h.h.aiDrafter = func(_ context.Context, _ string, prompt string) (string, error) {
		gotPrompt = prompt
		return `[
			{"path":"a.go","line":1,"side":"RIGHT","severity":"issue","body":"real finding","suggestion":"new one fixed"},
			{"path":"a.go","line":42,"side":"RIGHT","severity":"nit","body":"hallucinated line"}
		]`, nil
	}

	var resp aiReviewResponse
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/ai-review", map[string]any{
		"dir": dir, "pr": 3,
	}), http.StatusOK, &resp)

	if len(resp.Findings) != 1 {
		t.Fatalf("findings = %d, want 1 (the hallucinated line dropped): %+v", len(resp.Findings), resp.Findings)
	}
	f := resp.Findings[0]
	if f.Path != "a.go" || f.Line != 1 || f.Severity != "issue" || f.Suggestion != "new one fixed" {
		t.Errorf("finding = %+v, want the anchored issue with its suggestion", f)
	}
	if !strings.Contains(gotPrompt, "new two") {
		t.Errorf("prompt missing the diff text:\n%s", gotPrompt)
	}
}

func TestClaudeDraftArgsConstrainsClaude(t *testing.T) {
	args := claudeDraftArgs("PROMPT TEXT", "sonnet")
	// The prompt is an argument to -p (never interpolated into a shell).
	if len(args) < 2 || args[0] != "-p" || args[1] != "PROMPT TEXT" {
		t.Fatalf("prompt is not the -p argument: %v", args)
	}
	joined := strings.Join(args, " ")
	// These flags keep claude from going agentic (the "signal: killed" fix).
	for _, want := range []string{"--model sonnet", "--strict-mcp-config", "--max-turns 1", "--disallowedTools", "Bash", "Read"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %v", want, args)
		}
	}
}

func TestHandleAIReviewValidation(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	cases := []map[string]any{
		{"dir": "relative", "pr": 1},
		{"dir": "/abs", "pr": 0},
	}
	for _, body := range cases {
		h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/ai-review", body), http.StatusBadRequest, nil)
	}
}

func TestHandleAIReviewUnparseable(t *testing.T) {
	gh := &fakePRGH{detail: anchoredPR()}
	h := newPRHarness(t, gh)
	dir := t.TempDir()
	h.h.aiDrafter = func(context.Context, string, string) (string, error) {
		return "I refuse to output JSON.", nil
	}
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/ai-review", map[string]any{
		"dir": dir, "pr": 3,
	}), http.StatusBadGateway, nil)
}
