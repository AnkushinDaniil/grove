package codex

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

// TestParserSuccessTurn replays a REAL capture from `codex exec --json
// "reply with the single word: ok"` on codex-cli 0.143.0 (thread_id,
// token counts and text are all genuine, not fabricated).
func TestParserSuccessTurn(t *testing.T) {
	lines := readFixtureLines(t, "success_turn.jsonl")
	if len(lines) != 4 {
		t.Fatalf("fixture has %d lines, want 4", len(lines))
	}
	p := New().NewParser()

	perLine := [][]wantEvent{
		{{typ: core.EventSessionStarted}},
		{}, // turn.started carries nothing grove surfaces
		{{typ: core.EventText}},
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

	started, err := core.UnmarshalPayload[core.SessionStartedPayload](all[0].Payload)
	if err != nil {
		t.Fatalf("unmarshal session_started payload: %v", err)
	}
	if started.DriverSessionID != "019f806a-817f-7521-8282-7b6fa368e212" {
		t.Errorf("SessionStartedPayload = %+v", started)
	}

	text, err := core.UnmarshalPayload[core.TextPayload](all[1].Payload)
	if err != nil {
		t.Fatalf("unmarshal text payload: %v", err)
	}
	if text.Text != "ok" {
		t.Errorf("TextPayload.Text = %q, want %q", text.Text, "ok")
	}

	turnDone, err := core.UnmarshalPayload[core.TurnDonePayload](all[2].Payload)
	if err != nil {
		t.Fatalf("unmarshal turn_done payload: %v", err)
	}
	if turnDone.ResultText != "ok" {
		t.Errorf("TurnDonePayload.ResultText = %q, want %q (from the preceding agent_message)", turnDone.ResultText, "ok")
	}

	usage, err := core.UnmarshalPayload[core.UsagePayload](all[3].Payload)
	if err != nil {
		t.Fatalf("unmarshal usage payload: %v", err)
	}
	if usage.InputTokens != 14662 || usage.OutputTokens != 5 {
		t.Errorf("UsagePayload = %+v", usage)
	}
}

// TestParserToolCallTurn covers command_execution and mcp_tool_call, which
// the trivial live probe never exercised (it needed no tools). This fixture
// is SYNTHETIC, authored from codex-rs/exec/src/exec_events.rs (source-read,
// not another live probe) rather than a capture.
func TestParserToolCallTurn(t *testing.T) {
	lines := readFixtureLines(t, "tool_call_turn.jsonl")
	if len(lines) != 7 {
		t.Fatalf("fixture has %d lines, want 7", len(lines))
	}
	p := New().NewParser()

	perLine := [][]wantEvent{
		{{typ: core.EventSessionStarted}},
		{{typ: core.EventToolCall}},
		{{typ: core.EventToolResult}},
		{{typ: core.EventToolCall}},
		{{typ: core.EventToolResult}},
		{{typ: core.EventText}},
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

	shellCall, err := core.UnmarshalPayload[core.ToolCallPayload](all[1].Payload)
	if err != nil {
		t.Fatalf("unmarshal shell tool_call payload: %v", err)
	}
	if shellCall.Name != "shell" || shellCall.InputSummary != "go test ./..." {
		t.Errorf("shell ToolCallPayload = %+v", shellCall)
	}

	shellResult, err := core.UnmarshalPayload[core.ToolResultPayload](all[2].Payload)
	if err != nil {
		t.Fatalf("unmarshal shell tool_result payload: %v", err)
	}
	if !shellResult.OK || !strings.Contains(shellResult.Summary, "ok") {
		t.Errorf("shell ToolResultPayload = %+v", shellResult)
	}

	mcpCall, err := core.UnmarshalPayload[core.ToolCallPayload](all[3].Payload)
	if err != nil {
		t.Fatalf("unmarshal mcp tool_call payload: %v", err)
	}
	if mcpCall.Name != "ctx7.query-docs" || mcpCall.InputSummary != `{"library":"go"}` {
		t.Errorf("mcp ToolCallPayload = %+v", mcpCall)
	}

	mcpResult, err := core.UnmarshalPayload[core.ToolResultPayload](all[4].Payload)
	if err != nil {
		t.Fatalf("unmarshal mcp tool_result payload: %v", err)
	}
	if !mcpResult.OK || mcpResult.Summary != `{"text":"docs..."}` {
		t.Errorf("mcp ToolResultPayload = %+v", mcpResult)
	}

	turnDone, err := core.UnmarshalPayload[core.TurnDonePayload](all[6].Payload)
	if err != nil {
		t.Fatalf("unmarshal turn_done payload: %v", err)
	}
	if turnDone.ResultText != "Tests pass." {
		t.Errorf("TurnDonePayload.ResultText = %q, want %q", turnDone.ResultText, "Tests pass.")
	}
}

// TestParserErrorTurn covers a failed command_execution, an item-typed
// "error", turn.failed and a top-level "error" event. SYNTHETIC, authored
// from codex-rs/exec/src/exec_events.rs rather than a capture (the live
// probes never triggered a failure).
func TestParserErrorTurn(t *testing.T) {
	lines := readFixtureLines(t, "error_turn.jsonl")
	if len(lines) != 6 {
		t.Fatalf("fixture has %d lines, want 6", len(lines))
	}
	p := New().NewParser()

	perLine := [][]wantEvent{
		{{typ: core.EventSessionStarted}},
		{{typ: core.EventToolCall}},
		{{typ: core.EventToolResult}},
		{{typ: core.EventError}},
		{{typ: core.EventTurnDone}, {typ: core.EventError}},
		{{typ: core.EventError}},
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

	cmdResult, err := core.UnmarshalPayload[core.ToolResultPayload](all[2].Payload)
	if err != nil {
		t.Fatalf("unmarshal tool_result payload: %v", err)
	}
	if cmdResult.OK {
		t.Errorf("ToolResultPayload.OK = true, want false for a failed command")
	}

	itemErr, err := core.UnmarshalPayload[core.ErrorPayload](all[3].Payload)
	if err != nil {
		t.Fatalf("unmarshal item error payload: %v", err)
	}
	if itemErr.Message != "tool crashed" {
		t.Errorf("item ErrorPayload.Message = %q, want %q", itemErr.Message, "tool crashed")
	}

	turnFailedErr, err := core.UnmarshalPayload[core.ErrorPayload](all[5].Payload)
	if err != nil {
		t.Fatalf("unmarshal turn.failed error payload: %v", err)
	}
	if turnFailedErr.Message != "command failed: rm -rf /" {
		t.Errorf("turn.failed ErrorPayload.Message = %q, want %q", turnFailedErr.Message, "command failed: rm -rf /")
	}

	topLevelErr, err := core.UnmarshalPayload[core.ErrorPayload](all[6].Payload)
	if err != nil {
		t.Fatalf("unmarshal top-level error payload: %v", err)
	}
	if topLevelErr.Message != "stream disconnected" {
		t.Errorf("top-level ErrorPayload.Message = %q, want %q", topLevelErr.Message, "stream disconnected")
	}
}

// TestParserGarbageRobustness feeds blank lines, non-JSON text, an
// unrecognized top-level type, item.updated, and every item type this
// driver deliberately does not map (reasoning, file_change, web_search,
// todo_list, collab_tool_call, an unknown item type, and a missing item)
// through Feed, and asserts the stream never errors and only the one
// recognized line produces an event.
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
		prefix := []byte(`{"type":"item.completed","item":{"id":"i","type":"agent_message","text":"`)
		suffix := []byte(`"}}`)
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
	line := []byte(`{"type":"thread.started","thread_id":"sess-crlf"}` + "\r")
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
