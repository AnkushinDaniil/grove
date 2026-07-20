package core

import "time"

// EventType is the normalized event vocabulary every driver maps its native
// output into. Payloads are JSON and type-specific (see driver package docs).
type EventType string

const (
	EventSessionStarted EventType = "session_started" // {driver_session_id, transcript_path?}
	EventText           EventType = "text"            // {text, final}
	EventToolCall       EventType = "tool_call"       // {name, input_summary}
	EventToolResult     EventType = "tool_result"     // {name, ok, summary}
	EventAwaitingInput  EventType = "awaiting_input"  // {reason, detail} → attention
	EventTurnDone       EventType = "turn_done"       // {result_text, duration_ms}
	EventSessionEnded   EventType = "session_ended"   // {exit_code}
	EventError          EventType = "error"           // {message, fatal} → attention
	EventUsage          EventType = "usage"           // {input_tokens, output_tokens, cost_usd?}
)

func (t EventType) Valid() bool {
	switch t {
	case EventSessionStarted, EventText, EventToolCall, EventToolResult,
		EventAwaitingInput, EventTurnDone, EventSessionEnded, EventError, EventUsage:
		return true
	}
	return false
}

// AwaitingReason refines EventAwaitingInput payloads.
type AwaitingReason string

const (
	AwaitPermission AwaitingReason = "permission" // tool/permission prompt
	AwaitQuestion   AwaitingReason = "question"   // agent explicitly asked the user
	AwaitIdle       AwaitingReason = "idle"       // waiting at the input prompt
)

// AttentionFor maps a normalized event to the attention it should raise on the
// node, or AttentionNone. reason is only consulted for EventAwaitingInput.
func AttentionFor(t EventType, reason AwaitingReason) Attention {
	switch t {
	case EventAwaitingInput:
		if reason == AwaitPermission {
			return AttentionPermission
		}
		return AttentionQuestion
	case EventTurnDone:
		return AttentionDone
	case EventError:
		return AttentionError
	}
	return AttentionNone
}

// Event is one append-only record in a node's history. Events with
// RequiresAttention form the inbox until acknowledged.
type Event struct {
	ID                EventID
	NodeID            NodeID
	SessionID         SessionID // empty for node-level events (worktree, github, ...)
	Type              EventType
	Payload           string // JSON
	RequiresAttention bool
	AckedAt           time.Time // zero = unacked
	CreatedAt         time.Time
}
