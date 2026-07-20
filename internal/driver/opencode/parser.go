package opencode

import (
	"bytes"
	"encoding/json"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

// maxLineBytes caps the accepted stream line size; longer lines are
// silently skipped rather than erroring the stream.
const maxLineBytes = 10 * 1024 * 1024

// parser is the stateful per-run driver.Parser for OpenCode's `run --format
// json` event stream. It tracks the last completed text part seen so Close
// can synthesize a turn_done, and accumulates step_finish token/cost totals
// so Close can emit one combined usage event: unlike Claude/Codex,
// opencode's JSON-lines stream never emits an explicit
// turn/result-completion event of its own — the CLI process simply exits
// once it observes a "session.status: idle" event internally
// (packages/opencode/src/cli/cmd/run.ts, loop(): the break has no matching
// emit() call) — and a turn can contain several steps, each with its own
// step_finish, so the totals are summed rather than emitted per step.
type parser struct {
	lastText  string
	usage     core.UsagePayload
	haveUsage bool
}

// NewParser returns a fresh Parser for one OpenCode run.
func (opencodeDriver) NewParser() driver.Parser { return &parser{} }

// streamEvent is the superset of fields grove reads across every opencode
// `run --format json` line. Every line has the shape {type, timestamp,
// sessionID, ...data} (packages/opencode/src/cli/cmd/run.ts, emit()); grove
// does not need timestamp or sessionID (see opencode.go,
// Capabilities().EmitsSessionID), so neither is decoded here.
type streamEvent struct {
	Type  string          `json:"type"`
	Part  *streamPart     `json:"part"`  // tool_use, step_start, step_finish, text, reasoning
	Error *streamAPIError `json:"error"` // error
}

// streamPart is the superset of Part fields grove reads
// (packages/sdk/js/src/v2/gen/types.gen.ts). "type" tags which fields are
// populated; subtask/file/snapshot/patch/agent/retry/compaction parts are
// not surfaced by this driver (see Feed's default case).
type streamPart struct {
	Type   string           `json:"type"`
	Text   string           `json:"text"`   // text, reasoning
	Tool   string           `json:"tool"`   // tool
	State  *streamToolState `json:"state"`  // tool
	Tokens *streamTokens    `json:"tokens"` // step-finish
	Cost   float64          `json:"cost"`   // step-finish
}

// streamToolState mirrors ToolState's "completed"/"error" variants — the
// only two Feed sees, since run.ts only calls emit("tool_use", ...) once a
// tool part's status is "completed" or "error".
type streamToolState struct {
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
	Output string          `json:"output"` // completed
	Error  string          `json:"error"`  // error
}

// streamTokens mirrors StepFinishPart's "tokens" field (input/output only;
// reasoning and cache breakdown have no slot in core.UsagePayload).
type streamTokens struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
}

// streamAPIError mirrors the {name, data:{message}} shape every variant of
// opencode's error union shares (packages/sdk/js/src/v2/gen/types.gen.ts,
// ApiError and its siblings; packages/opencode/src/cli/cmd/run.ts prefers
// data.message over name when both are present).
type streamAPIError struct {
	Name string `json:"name"`
	Data *struct {
		Message string `json:"message"`
	} `json:"data"`
}

// message extracts the best available text: data.message when present,
// falling back to the error's name. A nil receiver (a malformed "error"
// line with no error object) safely yields an empty string.
func (e *streamAPIError) message() string {
	if e == nil {
		return ""
	}
	if e.Data != nil && e.Data.Message != "" {
		return e.Data.Message
	}
	return e.Name
}

// Feed parses one native stdout line into normalized events. Blank,
// oversized and non-JSON lines are skipped rather than erroring the stream;
// a trailing '\r' left by naive CRLF splitting is tolerated.
func (p *parser) Feed(line []byte) ([]core.EventInput, error) {
	line = bytes.TrimRight(line, "\r\n")
	if len(line) == 0 || len(line) > maxLineBytes {
		return nil, nil
	}
	var raw streamEvent
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, nil //nolint:nilerr // non-JSON lines are garbage, not stream failures; never error the stream.
	}
	switch raw.Type {
	case "text":
		return p.parseText(raw)
	case "tool_use":
		return parseToolUse(raw)
	case "step_finish":
		p.accumulateUsage(raw)
		return nil, nil
	case "error":
		return parseError(raw)
	default: // step_start, reasoning, and unknown types carry nothing grove surfaces today.
		return nil, nil
	}
}

// Close flushes the turn_done synthesized from state accumulated across
// Feed calls, plus a combined usage event if any step_finish was seen — see
// the parser doc comment for why opencode needs this instead of an
// in-stream completion event.
func (p *parser) Close() ([]core.EventInput, error) {
	turnPayload, err := core.MarshalPayload(core.TurnDonePayload{ResultText: p.lastText})
	if err != nil {
		return nil, err
	}
	events := []core.EventInput{{Type: core.EventTurnDone, Payload: turnPayload}}

	if p.haveUsage {
		usagePayload, err := core.MarshalPayload(p.usage)
		if err != nil {
			return nil, err
		}
		events = append(events, core.EventInput{Type: core.EventUsage, Payload: usagePayload})
	}
	return events, nil
}

// parseText handles a completed text part, tracking it as the turn's
// result-text-so-far (see the parser doc comment) and surfacing it as an
// EventText.
func (p *parser) parseText(raw streamEvent) ([]core.EventInput, error) {
	if raw.Part == nil {
		return nil, nil
	}
	p.lastText = raw.Part.Text
	payload, err := core.MarshalPayload(core.TextPayload{Text: raw.Part.Text})
	if err != nil {
		return nil, err
	}
	return []core.EventInput{{Type: core.EventText, Payload: payload}}, nil
}

// parseToolUse handles a completed-or-errored tool part. Unlike Claude
// (whose tool_use/tool_result arrive on separate lines) opencode's single
// "tool_use" line already carries both the call and its outcome, so both
// events are emitted together from it.
func parseToolUse(raw streamEvent) ([]core.EventInput, error) {
	if raw.Part == nil || raw.Part.State == nil {
		return nil, nil
	}
	callPayload, err := core.MarshalPayload(core.ToolCallPayload{
		Name:         raw.Part.Tool,
		InputSummary: compactTruncate(raw.Part.State.Input, summaryLen),
	})
	if err != nil {
		return nil, err
	}
	ok := raw.Part.State.Status == "completed"
	summary := raw.Part.State.Output
	if !ok {
		summary = raw.Part.State.Error
	}
	resultPayload, err := core.MarshalPayload(core.ToolResultPayload{
		Name:    raw.Part.Tool,
		OK:      ok,
		Summary: truncate(summary, summaryLen),
	})
	if err != nil {
		return nil, err
	}
	return []core.EventInput{
		{Type: core.EventToolCall, Payload: callPayload},
		{Type: core.EventToolResult, Payload: resultPayload},
	}, nil
}

// accumulateUsage folds one step's token/cost totals into the parser's
// running usage state; Close emits it once, combined, matching Claude's and
// Codex's one-usage-event-per-turn convention.
func (p *parser) accumulateUsage(raw streamEvent) {
	if raw.Part == nil || raw.Part.Tokens == nil {
		return
	}
	p.haveUsage = true
	p.usage.InputTokens += raw.Part.Tokens.Input
	p.usage.OutputTokens += raw.Part.Tokens.Output
	p.usage.CostUSD += raw.Part.Cost
}

// parseError handles opencode's "error" line, preferring the specific
// data.message over the generic error name (see streamAPIError.message).
func parseError(raw streamEvent) ([]core.EventInput, error) {
	payload, err := core.MarshalPayload(core.ErrorPayload{Message: raw.Error.message()})
	if err != nil {
		return nil, err
	}
	return []core.EventInput{{Type: core.EventError, Payload: payload}}, nil
}
