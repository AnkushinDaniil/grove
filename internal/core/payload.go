package core

import (
	"encoding/json"
	"fmt"
)

// Typed event payloads. Drivers marshal these into EventInput.Payload and
// consumers unmarshal by EventType, so both sides share one wire shape.

// SessionStartedPayload accompanies EventSessionStarted.
type SessionStartedPayload struct {
	DriverSessionID string `json:"driver_session_id"`
	TranscriptPath  string `json:"transcript_path,omitzero"`
	Model           string `json:"model,omitzero"`
}

// TextPayload accompanies EventText.
type TextPayload struct {
	Text  string `json:"text"`
	Final bool   `json:"final,omitzero"` // end-of-turn assistant text
}

// ToolCallPayload accompanies EventToolCall.
type ToolCallPayload struct {
	Name         string `json:"name"`
	InputSummary string `json:"input_summary,omitzero"`
}

// ToolResultPayload accompanies EventToolResult.
type ToolResultPayload struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Summary string `json:"summary,omitzero"`
}

// AwaitingPayload accompanies EventAwaitingInput.
type AwaitingPayload struct {
	Reason AwaitingReason `json:"reason"`
	Detail string         `json:"detail,omitzero"`
}

// TurnDonePayload accompanies EventTurnDone.
type TurnDonePayload struct {
	ResultText string `json:"result_text,omitzero"`
	DurationMS int64  `json:"duration_ms,omitzero"`
}

// SessionEndedPayload accompanies EventSessionEnded.
type SessionEndedPayload struct {
	ExitCode int `json:"exit_code"`
}

// ErrorPayload accompanies EventError.
type ErrorPayload struct {
	Message string `json:"message"`
	Fatal   bool   `json:"fatal,omitzero"`
}

// UsagePayload accompanies EventUsage.
type UsagePayload struct {
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd,omitzero"`
}

// MarshalPayload encodes a typed payload for EventInput.Payload.
func MarshalPayload(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal event payload: %w", err)
	}
	return string(b), nil
}

// UnmarshalPayload decodes an event payload into a typed struct.
func UnmarshalPayload[T any](payload string) (T, error) {
	var v T
	if payload == "" {
		return v, nil
	}
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		return v, fmt.Errorf("unmarshal event payload: %w", err)
	}
	return v, nil
}
