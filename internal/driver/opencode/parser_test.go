package opencode

import (
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
	typ core.EventType
}

func assertEvents(t *testing.T, got []core.EventInput, want []wantEvent) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d\ngot:  %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i].Type != want[i].typ {
			t.Errorf("event[%d].Type = %s, want %s", i, got[i].Type, want[i].typ)
		}
	}
}

// TestParserSuccessTurn covers a text-only turn: step_start (no-op),
// completed text, step_finish (usage). SYNTHETIC, authored from
// packages/sdk/js/src/v2/gen/types.gen.ts (Part/StepFinishPart shapes) and
// packages/opencode/src/cli/cmd/run.ts (emit() envelope and gating) rather
// than a capture — opencode is not installed in this environment.
func TestParserSuccessTurn(t *testing.T) {
	lines := readFixtureLines(t, "success_turn.jsonl")
	if len(lines) != 3 {
		t.Fatalf("fixture has %d lines, want 3", len(lines))
	}
	p := New().NewParser()

	perLine := [][]wantEvent{
		{}, // step_start carries nothing grove surfaces
		{{core.EventText}},
		{}, // step_finish is accumulated, not emitted per line
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
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertEvents(t, closeEvents, []wantEvent{{core.EventTurnDone}, {core.EventUsage}})
	all = append(all, closeEvents...)

	text, err := core.UnmarshalPayload[core.TextPayload](all[0].Payload)
	if err != nil {
		t.Fatalf("unmarshal text payload: %v", err)
	}
	if text.Text != "Done reading the file." {
		t.Errorf("TextPayload.Text = %q", text.Text)
	}

	turnDone, err := core.UnmarshalPayload[core.TurnDonePayload](all[1].Payload)
	if err != nil {
		t.Fatalf("unmarshal turn_done payload: %v", err)
	}
	if turnDone.ResultText != "Done reading the file." {
		t.Errorf("TurnDonePayload.ResultText = %q, want %q (from the preceding text part)", turnDone.ResultText, "Done reading the file.")
	}

	usage, err := core.UnmarshalPayload[core.UsagePayload](all[2].Payload)
	if err != nil {
		t.Fatalf("unmarshal usage payload: %v", err)
	}
	if usage.InputTokens != 120 || usage.OutputTokens != 45 {
		t.Errorf("UsagePayload = %+v, want InputTokens=120 OutputTokens=45", usage)
	}
	if usage.CostUSD != 0.0123 {
		t.Errorf("UsagePayload.CostUSD = %v, want 0.0123", usage.CostUSD)
	}
}

// TestParserToolCallTurn covers a completed tool call, which
// TestParserSuccessTurn does not exercise. SYNTHETIC, same provenance as
// TestParserSuccessTurn.
func TestParserToolCallTurn(t *testing.T) {
	lines := readFixtureLines(t, "tool_call_turn.jsonl")
	if len(lines) != 4 {
		t.Fatalf("fixture has %d lines, want 4", len(lines))
	}
	p := New().NewParser()

	perLine := [][]wantEvent{
		{},
		{{core.EventToolCall}, {core.EventToolResult}},
		{{core.EventText}},
		{},
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

	toolCall, err := core.UnmarshalPayload[core.ToolCallPayload](all[0].Payload)
	if err != nil {
		t.Fatalf("unmarshal tool_call payload: %v", err)
	}
	wantInputSummary := `{"command":"go test ./..."}`
	if toolCall.Name != "bash" || toolCall.InputSummary != wantInputSummary {
		t.Errorf("ToolCallPayload = %+v, want Name=bash InputSummary=%q", toolCall, wantInputSummary)
	}

	toolResult, err := core.UnmarshalPayload[core.ToolResultPayload](all[1].Payload)
	if err != nil {
		t.Fatalf("unmarshal tool_result payload: %v", err)
	}
	if toolResult.Name != "bash" || !toolResult.OK || !strings.Contains(toolResult.Summary, "ok") {
		t.Errorf("ToolResultPayload = %+v", toolResult)
	}

	closeEvents, err := p.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertEvents(t, closeEvents, []wantEvent{{core.EventTurnDone}, {core.EventUsage}})
	turnDone, err := core.UnmarshalPayload[core.TurnDonePayload](closeEvents[0].Payload)
	if err != nil {
		t.Fatalf("unmarshal turn_done payload: %v", err)
	}
	if turnDone.ResultText != "Tests pass." {
		t.Errorf("TurnDonePayload.ResultText = %q, want %q", turnDone.ResultText, "Tests pass.")
	}
}

// TestParserErrorTurn covers a failed tool call plus a stream-level
// "error" line. SYNTHETIC, same provenance as TestParserSuccessTurn.
func TestParserErrorTurn(t *testing.T) {
	lines := readFixtureLines(t, "error_turn.jsonl")
	if len(lines) != 3 {
		t.Fatalf("fixture has %d lines, want 3", len(lines))
	}
	p := New().NewParser()

	perLine := [][]wantEvent{
		{},
		{{core.EventToolCall}, {core.EventToolResult}},
		{{core.EventError}},
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

	toolResult, err := core.UnmarshalPayload[core.ToolResultPayload](all[1].Payload)
	if err != nil {
		t.Fatalf("unmarshal tool_result payload: %v", err)
	}
	if toolResult.OK {
		t.Errorf("ToolResultPayload.OK = true, want false for a failed command")
	}
	if toolResult.Summary != "Operation not permitted" {
		t.Errorf("ToolResultPayload.Summary = %q, want the state.error text", toolResult.Summary)
	}

	errPayload, err := core.UnmarshalPayload[core.ErrorPayload](all[2].Payload)
	if err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	want := "the model provider returned a 500"
	if errPayload.Message != want {
		t.Errorf("ErrorPayload.Message = %q, want %q (data.message preferred over name)", errPayload.Message, want)
	}

	// No step_finish occurred, so Close must not emit a usage event.
	closeEvents, err := p.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertEvents(t, closeEvents, []wantEvent{{core.EventTurnDone}})
}

// TestParserGarbageRobustness feeds step_start, a blank line, non-JSON
// text, an unrecognized future event type, a reasoning part (only ever
// emitted when --thinking is passed, which this driver never does, but the
// parser must still tolerate it), and malformed tool_use/text/step_finish
// lines missing their "part" through Feed, and asserts the stream never
// errors and produces no events at all — only Close's synthesized turn_done
// follows, with an empty result text since nothing usable was seen.
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
	if len(all) != 0 {
		t.Fatalf("events from Feed = %+v, want none", all)
	}

	closeEvents, err := p.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertEvents(t, closeEvents, []wantEvent{{core.EventTurnDone}})
	turnDone, err := core.UnmarshalPayload[core.TurnDonePayload](closeEvents[0].Payload)
	if err != nil {
		t.Fatalf("unmarshal turn_done payload: %v", err)
	}
	if turnDone.ResultText != "" {
		t.Errorf("TurnDonePayload.ResultText = %q, want empty", turnDone.ResultText)
	}
}

// TestParserFeedCapsLineSize proves the 10 MiB cap is size-based, not just
// "invalid JSON": two structurally identical, valid JSON lines, one over the
// cap (skipped) and one under it (parsed).
func TestParserFeedCapsLineSize(t *testing.T) {
	makeTextLine := func(payloadLen int) []byte {
		prefix := []byte(`{"type":"text","sessionID":"s","part":{"id":"p","sessionID":"s","messageID":"m","type":"text","text":"`)
		suffix := []byte(`"}}`)
		line := make([]byte, 0, len(prefix)+payloadLen+len(suffix))
		line = append(line, prefix...)
		line = append(line, strings.Repeat("x", payloadLen)...)
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
	line := []byte(`{"type":"text","sessionID":"s","part":{"id":"p","sessionID":"s","messageID":"m","type":"text","text":"hi"}}` + "\r")
	got, err := p.Feed(line)
	if err != nil {
		t.Fatalf("Feed() error = %v", err)
	}
	if len(got) != 1 || got[0].Type != core.EventText {
		t.Fatalf("Feed() = %+v, want single text event", got)
	}
}

func TestParserCloseWithNoInput(t *testing.T) {
	p := New().NewParser()
	events, err := p.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertEvents(t, events, []wantEvent{{core.EventTurnDone}})
}

// TestStreamAPIErrorMessage covers every shape streamAPIError.message must
// tolerate: a nil error object (malformed "error" line with no error
// field), one with no data at all, one with an empty data.message, and the
// normal case where data.message is preferred over name.
func TestStreamAPIErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  *streamAPIError
		want string
	}{
		{"nil receiver", nil, ""},
		{"no data falls back to name", &streamAPIError{Name: "UnknownError"}, "UnknownError"},
		{
			"empty data.message falls back to name",
			&streamAPIError{Name: "UnknownError", Data: &struct {
				Message string `json:"message"`
			}{Message: ""}},
			"UnknownError",
		},
		{
			"data.message preferred over name",
			&streamAPIError{Name: "APIError", Data: &struct {
				Message string `json:"message"`
			}{Message: "rate limited"}},
			"rate limited",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.message(); got != tt.want {
				t.Errorf("message() = %q, want %q", got, tt.want)
			}
		})
	}
}
