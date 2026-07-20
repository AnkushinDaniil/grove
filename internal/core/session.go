package core

import (
	"fmt"
	"time"
)

// SessionMode selects how the CLI process runs for a session.
type SessionMode string

const (
	ModePTY      SessionMode = "pty"      // interactive TUI the user can attach to
	ModeHeadless SessionMode = "headless" // -p / exec style, JSONL events on stdout
)

func (m SessionMode) Valid() bool { return m == ModePTY || m == ModeHeadless }

type SessionStatus string

const (
	SessionStarting      SessionStatus = "starting"
	SessionRunning       SessionStatus = "running"
	SessionAwaitingInput SessionStatus = "awaiting_input"
	SessionExited        SessionStatus = "exited"
	SessionFailed        SessionStatus = "failed"
	SessionInterrupted   SessionStatus = "interrupted" // process died with the daemon
)

func (s SessionStatus) Valid() bool {
	switch s {
	case SessionStarting, SessionRunning, SessionAwaitingInput,
		SessionExited, SessionFailed, SessionInterrupted:
		return true
	}
	return false
}

// Terminal reports whether no further transitions are possible.
func (s SessionStatus) Terminal() bool {
	return s == SessionExited || s == SessionFailed || s == SessionInterrupted
}

// CanTransition encodes the session lifecycle state machine:
//
//	starting → running | failed
//	running ⇄ awaiting_input
//	running | awaiting_input → exited | failed
//	any non-terminal → interrupted (daemon crash recovery)
func CanTransition(from, to SessionStatus) bool {
	if from.Terminal() {
		return false
	}
	switch to {
	case SessionInterrupted:
		return true
	case SessionRunning:
		return from == SessionStarting || from == SessionAwaitingInput
	case SessionAwaitingInput:
		return from == SessionRunning
	case SessionExited:
		return from == SessionRunning || from == SessionAwaitingInput
	case SessionFailed:
		return from == SessionStarting || from == SessionRunning || from == SessionAwaitingInput
	}
	return false
}

// NodeStatusFor maps a session status onto the owning node's status.
func NodeStatusFor(s SessionStatus, exitOK bool) NodeStatus {
	switch s {
	case SessionStarting:
		return StatusStarting
	case SessionRunning:
		return StatusRunning
	case SessionAwaitingInput:
		return StatusAwaitingInput
	case SessionExited:
		if exitOK {
			return StatusDone
		}
		return StatusFailed
	case SessionFailed:
		return StatusFailed
	case SessionInterrupted:
		return StatusInterrupted
	}
	return StatusIdle
}

// Session is an immutable snapshot of one CLI run bound to a node.
type Session struct {
	ID        SessionID
	NodeID    NodeID
	Driver    string
	ProfileID ProfileID
	Mode      SessionMode

	// DriverSessionID is the CLI's own session identifier (Claude session UUID,
	// Codex thread id, ...) used for --resume across grove sessions.
	DriverSessionID       string
	ParentDriverSessionID string // resume/fork chain

	Status         SessionStatus
	ExitCode       *int // nil until the process exited
	TranscriptPath string
	CWD            string

	StartedAt time.Time
	EndedAt   time.Time // zero while live
}

func (s Session) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("%w: session id is empty", ErrInvalid)
	}
	if s.NodeID == "" {
		return fmt.Errorf("%w: session node id is empty", ErrInvalid)
	}
	if s.Driver == "" {
		return fmt.Errorf("%w: session driver is empty", ErrInvalid)
	}
	if !s.Mode.Valid() {
		return fmt.Errorf("%w: unknown session mode %q", ErrInvalid, s.Mode)
	}
	if !s.Status.Valid() {
		return fmt.Errorf("%w: unknown session status %q", ErrInvalid, s.Status)
	}
	if s.CWD == "" {
		return fmt.Errorf("%w: session cwd is empty", ErrInvalid)
	}
	return nil
}
