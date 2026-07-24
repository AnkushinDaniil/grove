package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestClaudeSessionArgs(t *testing.T) {
	// Starting a session binds --session-id and sets the focused system prompt.
	start := strings.Join(claudeSessionArgs("PROMPT", "sonnet", "sid-1", false), " ")
	for _, want := range []string{"--session-id sid-1", "--system-prompt", "--strict-mcp-config", "--disallowedTools"} {
		if !strings.Contains(start, want) {
			t.Errorf("start args missing %q: %s", want, start)
		}
	}
	if strings.Contains(start, "--resume") {
		t.Errorf("start args should not resume: %s", start)
	}

	// Resuming carries --resume and must NOT re-send the system prompt (it is
	// fixed by the original turn) or a fresh --session-id.
	resume := strings.Join(claudeSessionArgs("Q", "sonnet", "sid-1", true), " ")
	if !strings.Contains(resume, "--resume sid-1") {
		t.Errorf("resume args missing --resume: %s", resume)
	}
	if strings.Contains(resume, "--system-prompt") || strings.Contains(resume, "--session-id") {
		t.Errorf("resume args should not re-send system prompt or session id: %s", resume)
	}
}

func TestReviewChatResumesStoredSession(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	dir := t.TempDir()
	// Simulate a prior review having stored a session id.
	if err := h.store.SetSetting(t.Context(), reviewSessionKey(dir, 7), "sess-42"); err != nil {
		t.Fatal(err)
	}

	var gotSession, gotMsg string
	var gotResume bool
	h.h.aiSession = func(_ context.Context, _ string, prompt, sessionID string, resume bool) (string, error) {
		gotSession, gotMsg, gotResume = sessionID, prompt, resume
		return "  because it drops the lock early.  ", nil
	}

	var resp reviewChatResponse
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/chat", map[string]any{
		"dir": dir, "pr": 7, "message": "why is the finding on line 92 a bug?",
	}), http.StatusOK, &resp)

	if resp.Reply != "because it drops the lock early." {
		t.Errorf("reply = %q, want trimmed answer", resp.Reply)
	}
	if gotSession != "sess-42" || !gotResume {
		t.Errorf("drafter called with session=%q resume=%v, want sess-42/true", gotSession, gotResume)
	}
	if !strings.Contains(gotMsg, "line 92") {
		t.Errorf("message not forwarded: %q", gotMsg)
	}
}

func TestReviewChatWithoutReviewIs409(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/chat", map[string]any{
		"dir": "/abs/repo", "pr": 7, "message": "hi",
	}), http.StatusConflict, nil)
}

func TestReviewChatValidation(t *testing.T) {
	h := newPRHarness(t, &fakePRGH{})
	cases := []map[string]any{
		{"dir": "relative", "pr": 7, "message": "hi"},
		{"dir": "/abs", "pr": 0, "message": "hi"},
		{"dir": "/abs", "pr": 7, "message": "  "},
	}
	for _, body := range cases {
		h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/chat", body), http.StatusBadRequest, nil)
	}
}

func TestAIReviewStoresSessionForChat(t *testing.T) {
	gh := &fakePRGH{detail: anchoredPR()}
	h := newPRHarness(t, gh)
	dir := t.TempDir()

	var startedResume bool
	h.h.aiSession = func(_ context.Context, _ string, _ string, _ string, resume bool) (string, error) {
		startedResume = resume
		return "[]", nil
	}
	h.decode(h.do(http.MethodPost, "/api/v1/reviews/pr/ai-review", map[string]any{
		"dir": dir, "pr": 3,
	}), http.StatusOK, nil)

	if startedResume {
		t.Error("the findings pass must start a session (resume=false), not resume one")
	}
	// A session id is now stored, so chat is available.
	sid, ok, err := h.store.GetSetting(t.Context(), reviewSessionKey(dir, 3))
	if err != nil || !ok || sid == "" {
		t.Errorf("review did not persist a session id (ok=%v id=%q err=%v)", ok, sid, err)
	}
}
