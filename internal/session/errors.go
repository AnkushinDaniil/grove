package session

import "errors"

// Typed errors callers branch on with errors.Is.
var (
	// ErrNoDriver means the node resolves to no driver, or to one that is not
	// registered.
	ErrNoDriver = errors.New("no driver for node")
	// ErrBudgetExhausted means the live-session cap (MaxRunning) is reached.
	ErrBudgetExhausted = errors.New("session budget exhausted")
	// ErrUnsupportedPrompt means the session cannot accept a follow-up prompt
	// (driver lacks streaming, or the process is no longer accepting input).
	ErrUnsupportedPrompt = errors.New("prompt unsupported for session")
	// ErrSessionNotFound means no live session has the given id.
	ErrSessionNotFound = errors.New("session not found")
)
