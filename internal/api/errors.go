package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/session"
)

// errorBody is the JSON error envelope: {"error": "..."} per docs/API.md.
type errorBody struct {
	Error string `json:"error"`
}

// statusForError maps a domain error to an HTTP status. core.ErrInvalid splits
// into 404 for not-found-flavored messages and 400 for every other validation
// failure; the substring heuristic is the documented, contract-permitted way to
// distinguish them without a dedicated sentinel (docs/API.md: unknown ids → 404,
// validation → 400).
func statusForError(err error) int {
	switch {
	case errors.Is(err, session.ErrSessionNotFound):
		return http.StatusNotFound
	case errors.Is(err, session.ErrBudgetExhausted):
		return http.StatusTooManyRequests
	case errors.Is(err, session.ErrNoDriver), errors.Is(err, session.ErrUnsupportedPrompt):
		return http.StatusBadRequest
	case errors.Is(err, core.ErrInvalid):
		if strings.Contains(err.Error(), "not found") {
			return http.StatusNotFound
		}
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// writeJSON encodes v as JSON with the given status. A marshal failure is logged
// and downgraded to a 500 so a handler never panics on a broken payload.
func writeJSON(w http.ResponseWriter, logger *slog.Logger, status int, v any) {
	buf, err := json.Marshal(v)
	if err != nil {
		logger.Error("marshal response", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}

// writeError maps err to a status and writes the JSON error envelope. The raw
// error text is returned for client (4xx) errors; server (5xx) errors are logged
// in full but reported generically so store internals never leak to the client.
func writeError(w http.ResponseWriter, logger *slog.Logger, err error) {
	status := statusForError(err)
	msg := err.Error()
	if status >= http.StatusInternalServerError {
		logger.Error("request failed", "status", status, "err", err)
		msg = "internal error"
	}
	writeJSON(w, logger, status, errorBody{Error: msg})
}

// writeErrorStatus writes an explicit status and message, for auth/host failures
// that are not domain errors.
func writeErrorStatus(w http.ResponseWriter, logger *slog.Logger, status int, msg string) {
	writeJSON(w, logger, status, errorBody{Error: msg})
}
