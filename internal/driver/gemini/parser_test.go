package gemini

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// feedFixture reads a testdata file, feeds it to a fresh parser line by
// line (as the session runtime would, splitting the raw process output on
// '\n'), and returns whatever Close reports at EOF. Every Feed call in
// between must report no events: gemini's headless result is not
// guaranteed to be one line, so this driver defers all parsing to Close
// (see the parser doc comment).
func feedFixture(t *testing.T, name string) []core.EventInput {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	text := strings.TrimSuffix(string(data), "\n")
	p := New().NewParser()
	for i, line := range strings.Split(text, "\n") {
		got, err := p.Feed([]byte(line))
		if err != nil {
			t.Fatalf("Feed(line %d) error = %v", i, err)
		}
		if len(got) != 0 {
			t.Fatalf("Feed(line %d) = %v, want no events (gemini only parses at Close)", i, got)
		}
	}
	events, err := p.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return events
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

func TestParserSuccessTurn(t *testing.T) {
	events := feedFixture(t, "success_turn.txt")
	assertEvents(t, events, []wantEvent{
		{core.EventText}, {core.EventTurnDone}, {core.EventUsage},
	})

	text, err := core.UnmarshalPayload[core.TextPayload](events[0].Payload)
	if err != nil {
		t.Fatalf("unmarshal text payload: %v", err)
	}
	if text.Text != "Done reading the file." || !text.Final {
		t.Errorf("TextPayload = %+v, want Text=%q Final=true", text, "Done reading the file.")
	}

	turnDone, err := core.UnmarshalPayload[core.TurnDonePayload](events[1].Payload)
	if err != nil {
		t.Fatalf("unmarshal turn_done payload: %v", err)
	}
	if turnDone.ResultText != "Done reading the file." {
		t.Errorf("TurnDonePayload.ResultText = %q", turnDone.ResultText)
	}

	usage, err := core.UnmarshalPayload[core.UsagePayload](events[2].Payload)
	if err != nil {
		t.Fatalf("unmarshal usage payload: %v", err)
	}
	if usage.InputTokens != 120 || usage.OutputTokens != 45 || usage.Model != "gemini-2.5-pro" {
		t.Errorf("UsagePayload = %+v, want InputTokens=120 OutputTokens=45 Model=gemini-2.5-pro", usage)
	}
	if usage.CostUSD != 0 {
		t.Errorf("UsagePayload.CostUSD = %v, want 0 (gemini's stats schema has no cost field)", usage.CostUSD)
	}
}

func TestParserFatalError(t *testing.T) {
	events := feedFixture(t, "fatal_error.txt")
	assertEvents(t, events, []wantEvent{{core.EventError}})

	errPayload, err := core.UnmarshalPayload[core.ErrorPayload](events[0].Payload)
	if err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	want := "Please run `gemini auth login` to authenticate."
	if errPayload.Message != want {
		t.Errorf("ErrorPayload.Message = %q, want %q", errPayload.Message, want)
	}
}

func TestParserGarbageRobustness(t *testing.T) {
	events := feedFixture(t, "garbage.txt")
	if len(events) != 0 {
		t.Fatalf("events = %+v, want none for a buffer with no complete JSON object", events)
	}
}

// TestParserResultWithError covers the invalidStreamError case
// (nonInteractiveCli.ts): a turn that still produced stats/response but also
// carries a per-turn error, distinct from the fatal-startup-error case in
// TestParserFatalError which has no stats at all.
func TestParserResultWithError(t *testing.T) {
	p := New().NewParser()
	line := []byte(`{"response":"partial output","stats":{"models":{"gemini-2.5-flash":{"tokens":{"input":10,"prompt":10,"candidates":3}}}},"error":{"type":"INVALID_STREAM","message":"stream closed unexpectedly"}}`)
	if _, err := p.Feed(line); err != nil {
		t.Fatalf("Feed() error = %v", err)
	}
	events, err := p.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertEvents(t, events, []wantEvent{
		{core.EventText}, {core.EventTurnDone}, {core.EventUsage}, {core.EventError},
	})
	errPayload, err := core.UnmarshalPayload[core.ErrorPayload](events[3].Payload)
	if err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if errPayload.Message != "stream closed unexpectedly" {
		t.Errorf("ErrorPayload.Message = %q", errPayload.Message)
	}
}

// TestParserErrorMessageFallback pins down the "prefer message, fall back
// to type" rule, mirroring the claude driver's analogous "result or
// subtype" convention.
func TestParserErrorMessageFallback(t *testing.T) {
	p := New().NewParser()
	line := []byte(`{"error":{"type":"UnknownError","message":""}}`)
	if _, err := p.Feed(line); err != nil {
		t.Fatalf("Feed() error = %v", err)
	}
	events, err := p.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertEvents(t, events, []wantEvent{{core.EventError}})
	errPayload, err := core.UnmarshalPayload[core.ErrorPayload](events[0].Payload)
	if err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if errPayload.Message != "UnknownError" {
		t.Errorf("ErrorPayload.Message = %q, want fallback to type %q", errPayload.Message, "UnknownError")
	}
}

// TestParserEmptyObjectProducesNothing proves a completely empty JSON
// object (no response, no stats, no error — should never happen for real
// output, but the schema technically allows it) is treated as "nothing
// found", not as an empty successful turn.
func TestParserEmptyObjectProducesNothing(t *testing.T) {
	p := New().NewParser()
	if _, err := p.Feed([]byte(`{}`)); err != nil {
		t.Fatalf("Feed() error = %v", err)
	}
	events, err := p.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %+v, want none for an empty object", events)
	}
}

func TestParserFeedTrimsCRLF(t *testing.T) {
	p := New().NewParser()
	line := []byte(`{"response":"ok","stats":{"models":{}}}` + "\r")
	if _, err := p.Feed(line); err != nil {
		t.Fatalf("Feed() error = %v", err)
	}
	events, err := p.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertEvents(t, events, []wantEvent{{core.EventText}, {core.EventTurnDone}, {core.EventUsage}})
}

// TestParserFeedCapsBufferSize proves the 10 MiB cap bounds total buffered
// output, not just one line: once exceeded, further input is dropped so
// Close finds an incomplete (and therefore ignored) trailing object rather
// than growing without bound.
func TestParserFeedCapsBufferSize(t *testing.T) {
	p := New().NewParser()
	huge := make([]byte, maxBufferBytes)
	for i := range huge {
		huge[i] = 'x'
	}
	if _, err := p.Feed(huge); err != nil {
		t.Fatalf("Feed(huge) error = %v", err)
	}
	// This complete, valid object arrives after the cap was already hit and
	// must be dropped, not parsed.
	if _, err := p.Feed([]byte(`{"response":"too late"}`)); err != nil {
		t.Fatalf("Feed(after cap) error = %v", err)
	}
	events, err := p.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %+v, want none once the buffer cap is exceeded", events)
	}
}

func TestParserCloseWithNoInput(t *testing.T) {
	p := New().NewParser()
	events, err := p.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %+v, want none", events)
	}
}

func TestFindLastJSONObject(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no braces", "just some log text\nmore text", ""},
		{"simple object", `noise before {"a":1} noise after`, `{"a":1}`},
		{
			"braces inside string values are ignored",
			`{"response":"here is a { snippet } with braces","stats":null}`,
			`{"response":"here is a { snippet } with braces","stats":null}`,
		},
		{
			"escaped quote before a brace does not end the string early",
			`{"response":"a \"quoted { word\" and more"}`,
			`{"response":"a \"quoted { word\" and more"}`,
		},
		{
			"multiple top-level objects: last one wins",
			"{\"first\":true}\nsome log noise\n{\"second\":true}",
			`{"second":true}`,
		},
		{
			"nested object: only the outer span is returned",
			`{"a":{"b":{"c":1}}}`,
			`{"a":{"b":{"c":1}}}`,
		},
		{"unbalanced open brace: no match", `{"a":1`, ""},
		{"stray closing brace before a valid object: ignored, not fatal", `} noise {"a":1}`, `{"a":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findLastJSONObject([]byte(tt.in))
			if tt.want == "" {
				if got != nil {
					t.Errorf("findLastJSONObject(%q) = %q, want nil", tt.in, got)
				}
				return
			}
			if string(got) != tt.want {
				t.Errorf("findLastJSONObject(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
