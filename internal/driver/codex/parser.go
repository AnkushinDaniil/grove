package codex

import (
	"bytes"
	"encoding/json"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

// maxLineBytes caps the accepted event stream line size; longer lines are
// silently skipped rather than erroring the stream.
const maxLineBytes = 10 * 1024 * 1024

// parser is the stateful per-run driver.Parser for codex's `exec --json`
// event stream. It tracks the last agent_message text seen so turn_done can
// carry a result_text: unlike Claude, codex has no single "result" line
// that repeats the final text — the text only appears once, on the
// agent_message item.completed event that precedes turn.completed.
type parser struct {
	lastAgentText string
}

// NewParser returns a fresh Parser for one Codex run.
func (codexDriver) NewParser() driver.Parser { return &parser{} }

// streamEvent is the superset of fields grove reads across every codex
// `exec --json` ThreadEvent variant. Field shapes are pinned from
// codex-rs/exec/src/exec_events.rs (codex-cli 0.143.0): the top-level enum
// is #[serde(tag = "type")], so every variant's fields sit flat alongside
// "type" on the same JSON object; fields unused by a given "type" are
// simply left zero.
type streamEvent struct {
	Type     string       `json:"type"`
	ThreadID string       `json:"thread_id"` // thread.started
	Item     *streamItem  `json:"item"`      // item.started / item.updated / item.completed
	Usage    *streamUsage `json:"usage"`     // turn.completed
	Error    *streamError `json:"error"`     // turn.failed
	Message  string       `json:"message"`   // top-level "error"
}

// streamItem is the superset of ThreadItemDetails fields grove reads. A
// ThreadItem flattens {id, ...details}, with "type" as the details enum's
// tag (#[serde(tag = "type", rename_all = "snake_case")]); reasoning,
// file_change, web_search, todo_list and collab_tool_call items are
// intentionally not decoded beyond "type" — see parseItemCompleted.
type streamItem struct {
	Type             string          `json:"type"`
	Text             string          `json:"text"`              // agent_message
	Command          string          `json:"command"`           // command_execution
	AggregatedOutput string          `json:"aggregated_output"` // command_execution
	Server           string          `json:"server"`            // mcp_tool_call
	Tool             string          `json:"tool"`              // mcp_tool_call
	Arguments        json.RawMessage `json:"arguments"`         // mcp_tool_call
	Result           json.RawMessage `json:"result"`            // mcp_tool_call
	Status           string          `json:"status"`            // command_execution, mcp_tool_call: in_progress|completed|failed|declined
	Message          string          `json:"message"`           // item type "error"
}

// streamUsage is codex's per-turn token usage. cached_input_tokens,
// cache_write_input_tokens and reasoning_output_tokens are observed on the
// wire (probed live) but dropped here: core.UsagePayload has no field for
// them, matching how the claude driver only carries input/output tokens
// too.
type streamUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// streamError is ThreadErrorEvent: {message}, used both standalone (the
// top-level "error" event, via streamEvent.Message) and nested in
// TurnFailedEvent (streamEvent.Error).
type streamError struct {
	Message string `json:"message"`
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
	case "thread.started":
		return parseThreadStarted(raw)
	case "turn.completed":
		return p.parseTurnCompleted(raw)
	case "turn.failed":
		return p.parseTurnFailed(raw)
	case "item.started":
		return parseItemStarted(raw)
	case "item.completed":
		return p.parseItemCompleted(raw)
	case "error":
		return parseTopLevelError(raw)
	default: // turn.started, item.updated, and unknown types carry nothing grove surfaces today.
		return nil, nil
	}
}

// Close flushes trailing state at EOF. Codex's stream carries none.
func (p *parser) Close() ([]core.EventInput, error) { return nil, nil }
