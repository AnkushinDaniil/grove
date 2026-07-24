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
	"strings"
)

// aiSessionFunc runs a headless claude turn bound to a resumable session: with
// resume=false it starts a new session under sessionID (the findings pass);
// with resume=true it continues that session (a chat turn), so claude still has
// the PR, the injected codebase context, and its own findings in view. A nil
// field falls back to defaultSessionDrafter; tests override it.
type aiSessionFunc func(ctx context.Context, dir, prompt, sessionID string, resume bool) (string, error)

// reviewSessionKey names the settings entry mapping one review workspace to its
// resumable claude session id, so chat survives daemon restarts (claude keeps
// the transcript on disk; grove only has to remember the id).
func reviewSessionKey(dir string, pr int) string {
	return fmt.Sprintf("review_session:%s:%d", dir, pr)
}

type reviewChatRequest struct {
	Dir     string `json:"dir"`
	PR      int    `json:"pr"`
	Message string `json:"message"`
}

type reviewChatResponse struct {
	Reply string `json:"reply"`
}

// handleReviewChat continues the review conversation: it resumes the session the
// last ai-review pass created for this (dir, pr) and asks the user's question, so
// the reviewer answers with the whole PR and its findings already in context
// (e.g. "why is the finding on line 92 a bug?"). Returns 409 when no review has
// been run yet, since there is no session to resume.
func (h *Handlers) handleReviewChat(w http.ResponseWriter, r *http.Request) {
	var req reviewChatRequest
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
	if strings.TrimSpace(req.Message) == "" {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "message must not be empty")
		return
	}

	sessionID, ok, err := h.store.GetSetting(r.Context(), reviewSessionKey(req.Dir, req.PR))
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	if !ok || sessionID == "" {
		writeErrorStatus(w, h.logger, http.StatusConflict, "run a review first, then chat with it")
		return
	}

	drafter := h.aiSession
	if drafter == nil {
		drafter = defaultSessionDrafter
	}
	reply, err := drafter(r.Context(), req.Dir, req.Message, sessionID, true)
	if err != nil {
		h.writeGHError(w, fmt.Errorf("review chat: %w", err))
		return
	}
	writeJSON(w, h.logger, http.StatusOK, reviewChatResponse{Reply: strings.TrimSpace(reply)})
}

// claudeSessionArgs builds the argv for a session-bound headless claude turn.
// It carries the same anti-agentic constraints as claudeDraftArgs (see there),
// plus session binding. On resume the system prompt is already fixed by the
// original turn, so it is not re-sent.
func claudeSessionArgs(prompt, model, sessionID string, resume bool) []string {
	args := []string{"-p", prompt, "--model", model, "--output-format", "text"}
	if resume {
		args = append(args, "--resume", sessionID)
	} else {
		args = append(args, "--system-prompt", draftSystemPrompt, "--session-id", sessionID)
	}
	args = append(args,
		"--strict-mcp-config",
		// Denylist last so its variadic value greedily takes exactly these tools.
		"--disallowedTools", "Bash", "Read", "Edit", "Write", "Glob", "Grep",
		"WebFetch", "WebSearch", "Task", "NotebookEdit", "TodoWrite",
	)
	return args
}

// defaultSessionDrafter runs claude bound to sessionID (start or resume) with a
// scrubbed PATH and the shared drafting timeout. It mirrors defaultAIDrafter.
func defaultSessionDrafter(ctx context.Context, dir, prompt, sessionID string, resume bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), aiDraftTimeout)
	defer cancel()

	model := aiDraftModel
	if m := os.Getenv("GROVE_AI_DRAFT_MODEL"); m != "" {
		model = m
	}
	//nolint:gosec // G204: binary is the fixed literal "claude"; prompt/model/session id are arguments.
	cmd := exec.CommandContext(ctx, "claude", claudeSessionArgs(prompt, model, sessionID, resume)...)
	cmd.Dir = dir
	cmd.Env = scrubClaudePATH(os.Environ())

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("claude timed out after %s", aiDraftTimeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", fmt.Errorf("claude -p: %w", err)
		}
		return "", fmt.Errorf("claude -p: %w: %s", err, msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}
