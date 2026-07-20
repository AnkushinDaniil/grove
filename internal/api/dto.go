package api

import (
	"encoding/json"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// The DTOs below are the JSON wire shapes from docs/API.md (the frozen
// contract): snake_case fields, RFC 3339 timestamps omitted when zero, and
// meta/payload serialized as raw JSON objects rather than strings. The mapping
// functions are exported so internal/ws reuses them instead of duplicating the
// contract.

// NodeDTO is the wire representation of a core.Node.
type NodeDTO struct {
	ID               string          `json:"id"`
	ParentID         string          `json:"parent_id"`
	Kind             string          `json:"kind"`
	Title            string          `json:"title"`
	Brief            string          `json:"brief"`
	Status           string          `json:"status"`
	Attention        string          `json:"attention"`
	AttentionReason  string          `json:"attention_reason"`
	AttentionSince   *string         `json:"attention_since,omitempty"`
	Driver           string          `json:"driver"`
	ProfileID        string          `json:"profile_id"`
	CurrentSessionID string          `json:"current_session_id"`
	WorkspaceDir     string          `json:"workspace_dir"`
	WorkDir          string          `json:"work_dir"`
	Meta             json.RawMessage `json:"meta"`
	Position         int             `json:"position"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
	ArchivedAt       *string         `json:"archived_at,omitempty"`
}

// NodeToDTO maps a node snapshot to its wire representation.
func NodeToDTO(n core.Node) NodeDTO {
	return NodeDTO{
		ID:               string(n.ID),
		ParentID:         string(n.ParentID),
		Kind:             string(n.Kind),
		Title:            n.Title,
		Brief:            n.Brief,
		Status:           string(n.Status),
		Attention:        string(n.Attention),
		AttentionReason:  n.AttentionReason,
		AttentionSince:   rfc3339Ptr(n.AttentionSince),
		Driver:           n.Driver,
		ProfileID:        string(n.ProfileID),
		CurrentSessionID: string(n.CurrentSessionID),
		WorkspaceDir:     n.WorkspaceDir,
		WorkDir:          n.WorkDir,
		Meta:             rawJSONObject(n.Meta),
		Position:         n.Position,
		CreatedAt:        rfc3339(n.CreatedAt),
		UpdatedAt:        rfc3339(n.UpdatedAt),
		ArchivedAt:       rfc3339Ptr(n.ArchivedAt),
	}
}

// NodesToDTO maps a slice of nodes, always returning a non-nil slice.
func NodesToDTO(nodes []core.Node) []NodeDTO {
	out := make([]NodeDTO, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, NodeToDTO(n))
	}
	return out
}

// SessionDTO is the wire representation of a core.Session. transcript_path and
// parent_driver_session_id are internal and deliberately not exposed.
type SessionDTO struct {
	ID              string  `json:"id"`
	NodeID          string  `json:"node_id"`
	Driver          string  `json:"driver"`
	ProfileID       string  `json:"profile_id"`
	Mode            string  `json:"mode"`
	DriverSessionID string  `json:"driver_session_id"`
	Status          string  `json:"status"`
	ExitCode        *int    `json:"exit_code,omitempty"`
	CWD             string  `json:"cwd"`
	StartedAt       string  `json:"started_at"`
	EndedAt         *string `json:"ended_at,omitempty"`
}

// SessionToDTO maps a session snapshot to its wire representation.
func SessionToDTO(s core.Session) SessionDTO {
	return SessionDTO{
		ID:              string(s.ID),
		NodeID:          string(s.NodeID),
		Driver:          s.Driver,
		ProfileID:       string(s.ProfileID),
		Mode:            string(s.Mode),
		DriverSessionID: s.DriverSessionID,
		Status:          string(s.Status),
		ExitCode:        s.ExitCode,
		CWD:             s.CWD,
		StartedAt:       rfc3339(s.StartedAt),
		EndedAt:         rfc3339Ptr(s.EndedAt),
	}
}

// SessionsToDTO maps a slice of sessions, always returning a non-nil slice.
func SessionsToDTO(sessions []core.Session) []SessionDTO {
	out := make([]SessionDTO, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, SessionToDTO(s))
	}
	return out
}

// EventDTO is the wire representation of a core.Event. payload is a raw JSON
// object (the type-specific payload), never a string.
type EventDTO struct {
	ID                string          `json:"id"`
	NodeID            string          `json:"node_id"`
	SessionID         string          `json:"session_id"`
	Type              string          `json:"type"`
	Payload           json.RawMessage `json:"payload"`
	RequiresAttention bool            `json:"requires_attention"`
	AckedAt           *string         `json:"acked_at,omitempty"`
	CreatedAt         string          `json:"created_at"`
}

// EventToDTO maps an event to its wire representation.
func EventToDTO(e core.Event) EventDTO {
	return EventDTO{
		ID:                string(e.ID),
		NodeID:            string(e.NodeID),
		SessionID:         string(e.SessionID),
		Type:              string(e.Type),
		Payload:           rawJSONObject(e.Payload),
		RequiresAttention: e.RequiresAttention,
		AckedAt:           rfc3339Ptr(e.AckedAt),
		CreatedAt:         rfc3339(e.CreatedAt),
	}
}

// EventsToDTO maps a slice of events, always returning a non-nil slice.
func EventsToDTO(events []core.Event) []EventDTO {
	out := make([]EventDTO, 0, len(events))
	for _, e := range events {
		out = append(out, EventToDTO(e))
	}
	return out
}

// rfc3339 formats a non-zero timestamp as an RFC 3339 UTC string.
func rfc3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// rfc3339Ptr formats t as RFC 3339, returning nil for the zero time so the
// field is omitted from JSON (per the contract's "omitted when zero" rule).
func rfc3339Ptr(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// rawJSONObject returns s as raw JSON, defaulting empty/blank stored values to
// an empty object so meta and payload always serialize as JSON objects.
func rawJSONObject(s string) json.RawMessage {
	if len(s) == 0 {
		return json.RawMessage("{}")
	}
	return json.RawMessage(s)
}
