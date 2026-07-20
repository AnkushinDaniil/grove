package claude

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// readFixtureLines splits a testdata JSONL file into raw lines, dropping
// exactly one trailing newline (not a trailing blank line — fixtures that
// need a blank line put it mid-file).
func readFixtureLines(t *testing.T, name string) [][]byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	text := strings.TrimSuffix(string(data), "\n")
	parts := strings.Split(text, "\n")
	lines := make([][]byte, len(parts))
	for i, p := range parts {
		lines[i] = []byte(p)
	}
	return lines
}

type wantEvent struct {
	typ    core.EventType
	reason core.AwaitingReason
	detail string
}

func assertEvents(t *testing.T, got []core.EventInput, want []wantEvent) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d\ngot:  %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i].Type != want[i].typ || got[i].Reason != want[i].reason || got[i].Detail != want[i].detail {
			t.Errorf("event[%d] = {Type:%s Reason:%s Detail:%q}, want {Type:%s Reason:%s Detail:%q}",
				i, got[i].Type, got[i].Reason, got[i].Detail, want[i].typ, want[i].reason, want[i].detail)
		}
	}
}

func TestParserSuccessTurn(t *testing.T) {
	lines := readFixtureLines(t, "success_turn.jsonl")
	if len(lines) != 4 {
		t.Fatalf("fixture has %d lines, want 4", len(lines))
	}
	p := New().NewParser()

	perLine := [][]wantEvent{
		{{typ: core.EventSessionStarted}},
		{{typ: core.EventText}, {typ: core.EventToolCall}},
		{{typ: core.EventToolResult}},
		{{typ: core.EventTurnDone}, {typ: core.EventUsage}},
	}

	var all []core.EventInput
	for i, line := range lines {
		got, err := p.Feed(line)
		if err != nil {
			t.Fatalf("Feed(line %d) error = %v", i, err)
		}
		assertEvents(t, got, perLine[i])
		all = append(all, got...)
	}
	closeEvents, err := p.Close()
	if err != nil || len(closeEvents) != 0 {
		t.Fatalf("Close() = %v, %v, want nil, nil", closeEvents, err)
	}

	// Payload spot-checks, in emission order.
	started, err := core.UnmarshalPayload[core.SessionStartedPayload](all[0].Payload)
	if err != nil {
		t.Fatalf("unmarshal session_started payload: %v", err)
	}
	if started.DriverSessionID != "sess-abc123" || started.Model != "claude-opus-4-8" {
		t.Errorf("SessionStartedPayload = %+v", started)
	}

	text, err := core.UnmarshalPayload[core.TextPayload](all[1].Payload)
	if err != nil {
		t.Fatalf("unmarshal text payload: %v", err)
	}
	if text.Text != "Let me check the file." {
		t.Errorf("TextPayload.Text = %q", text.Text)
	}

	toolCall, err := core.UnmarshalPayload[core.ToolCallPayload](all[2].Payload)
	if err != nil {
		t.Fatalf("unmarshal tool_call payload: %v", err)
	}
	wantInputSummary := `{"file_path":"/tmp/example.go","limit":200}`
	if toolCall.Name != "Read" || toolCall.InputSummary != wantInputSummary {
		t.Errorf("ToolCallPayload = %+v, want Name=Read InputSummary=%q", toolCall, wantInputSummary)
	}

	toolResult, err := core.UnmarshalPayload[core.ToolResultPayload](all[3].Payload)
	if err != nil {
		t.Fatalf("unmarshal tool_result payload: %v", err)
	}
	if !toolResult.OK || toolResult.Summary != "package main\n\nfunc main() {}\n" {
		t.Errorf("ToolResultPayload = %+v", toolResult)
	}

	turnDone, err := core.UnmarshalPayload[core.TurnDonePayload](all[4].Payload)
	if err != nil {
		t.Fatalf("unmarshal turn_done payload: %v", err)
	}
	if turnDone.ResultText != "Done reading the file." || turnDone.DurationMS != 1500 {
		t.Errorf("TurnDonePayload = %+v", turnDone)
	}

	usage, err := core.UnmarshalPayload[core.UsagePayload](all[5].Payload)
	if err != nil {
		t.Fatalf("unmarshal usage payload: %v", err)
	}
	if usage.InputTokens != 120 || usage.OutputTokens != 45 || usage.CostUSD != 0.0123 {
		t.Errorf("UsagePayload = %+v", usage)
	}
}

func TestParserErrorTurn(t *testing.T) {
	lines := readFixtureLines(t, "error_turn.jsonl")
	if len(lines) != 2 {
		t.Fatalf("fixture has %d lines, want 2", len(lines))
	}
	p := New().NewParser()

	first, err := p.Feed(lines[0])
	if err != nil {
		t.Fatalf("Feed(line 0) error = %v", err)
	}
	assertEvents(t, first, []wantEvent{{typ: core.EventSessionStarted}})

	second, err := p.Feed(lines[1])
	if err != nil {
		t.Fatalf("Feed(line 1) error = %v", err)
	}
	assertEvents(t, second, []wantEvent{
		{typ: core.EventTurnDone}, {typ: core.EventUsage}, {typ: core.EventError},
	})

	usage, err := core.UnmarshalPayload[core.UsagePayload](second[1].Payload)
	if err != nil {
		t.Fatalf("unmarshal usage payload: %v", err)
	}
	if usage.InputTokens != 500 || usage.OutputTokens != 10 || usage.CostUSD != 0.05 {
		t.Errorf("UsagePayload = %+v", usage)
	}

	errPayload, err := core.UnmarshalPayload[core.ErrorPayload](second[2].Payload)
	if err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if errPayload.Message != "error_max_turns" {
		t.Errorf("ErrorPayload.Message = %q, want %q", errPayload.Message, "error_max_turns")
	}
}

// TestParserErrorMessageFallback pins down the "subtype or error text" rule:
// prefer a non-empty result string as the error message, falling back to the
// subtype only when result is empty.
func TestParserErrorMessageFallback(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantMsg string
	}{
		{
			name:    "uses result text when present",
			line:    `{"type":"result","subtype":"error_during_execution","result":"boom: disk full","duration_ms":10}`,
			wantMsg: "boom: disk full",
		},
		{
			name:    "falls back to subtype when result is empty",
			line:    `{"type":"result","subtype":"error_during_execution","result":"","duration_ms":10}`,
			wantMsg: "error_during_execution",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New().NewParser()
			events, err := p.Feed([]byte(tt.line))
			if err != nil {
				t.Fatalf("Feed() error = %v", err)
			}
			assertEvents(t, events, []wantEvent{
				{typ: core.EventTurnDone}, {typ: core.EventUsage}, {typ: core.EventError},
			})
			errPayload, err := core.UnmarshalPayload[core.ErrorPayload](events[2].Payload)
			if err != nil {
				t.Fatalf("unmarshal error payload: %v", err)
			}
			if errPayload.Message != tt.wantMsg {
				t.Errorf("ErrorPayload.Message = %q, want %q", errPayload.Message, tt.wantMsg)
			}
		})
	}
}

// TestParserGarbageRobustness feeds blank lines, non-JSON text, and unknown
// top-level/block types through Feed and asserts the stream never errors and
// only the one recognized line produces events.
func TestParserGarbageRobustness(t *testing.T) {
	lines := readFixtureLines(t, "garbage.jsonl")
	p := New().NewParser()
	var all []core.EventInput
	for i, line := range lines {
		got, err := p.Feed(line)
		if err != nil {
			t.Fatalf("Feed(line %d) = %q returned error = %v, want nil (must never error on garbage)", i, line, err)
		}
		all = append(all, got...)
	}
	assertEvents(t, all, []wantEvent{{typ: core.EventSessionStarted}})
}

// TestParserFeedCapsLineSize proves the 10 MiB cap is size-based, not just
// "invalid JSON": two structurally identical, valid JSON lines, one over the
// cap (skipped) and one under it (parsed).
func TestParserFeedCapsLineSize(t *testing.T) {
	makeTextLine := func(payloadLen int) []byte {
		prefix := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"`)
		suffix := []byte(`"}]}}`)
		line := make([]byte, 0, len(prefix)+payloadLen+len(suffix))
		line = append(line, prefix...)
		line = append(line, bytes.Repeat([]byte("x"), payloadLen)...)
		line = append(line, suffix...)
		return line
	}

	p := New().NewParser()

	oversized := makeTextLine(maxLineBytes + 1)
	got, err := p.Feed(oversized)
	if err != nil {
		t.Fatalf("Feed(oversized) error = %v, want nil (garbage never errors)", err)
	}
	if len(got) != 0 {
		t.Fatalf("Feed(oversized len=%d) = %d events, want 0 (must be skipped over the 10MiB cap)", len(oversized), len(got))
	}

	underCap := makeTextLine(1024)
	got, err = p.Feed(underCap)
	if err != nil {
		t.Fatalf("Feed(under cap) error = %v", err)
	}
	if len(got) != 1 || got[0].Type != core.EventText {
		t.Fatalf("Feed(under cap) = %+v, want single text event", got)
	}
}

// TestParserFeedTrimsCRLF covers a naive caller that splits CRLF-terminated
// output on '\n' alone, leaving a dangling '\r' on every line.
func TestParserFeedTrimsCRLF(t *testing.T) {
	p := New().NewParser()
	line := []byte(`{"type":"system","subtype":"init","session_id":"sess-crlf","model":"m"}` + "\r")
	got, err := p.Feed(line)
	if err != nil {
		t.Fatalf("Feed() error = %v", err)
	}
	assertEvents(t, got, []wantEvent{{typ: core.EventSessionStarted}})
	started, err := core.UnmarshalPayload[core.SessionStartedPayload](got[0].Payload)
	if err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if started.DriverSessionID != "sess-crlf" {
		t.Errorf("DriverSessionID = %q, want %q", started.DriverSessionID, "sess-crlf")
	}
}

func TestParserClose(t *testing.T) {
	p := New().NewParser()
	events, err := p.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("Close() = %v, want no events", events)
	}
}
