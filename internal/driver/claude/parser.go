package claude

import (
	"bytes"
	"encoding/json"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

// maxLineBytes caps the accepted stream-json line size; longer lines are
// silently skipped rather than erroring the stream.
const maxLineBytes = 10 * 1024 * 1024

// parser is the stateful per-run driver.Parser for Claude Code's stream-json
// output. Each line is self-contained, so it holds no cross-line state
// today, but NewParser hands out a distinct instance per run.
type parser struct{}

// NewParser returns a fresh Parser for one Claude Code run.
func (claudeDriver) NewParser() driver.Parser { return &parser{} }

// streamLine is the superset of fields grove reads across every stream-json
// line type; fields unused by a given "type" are simply left zero.
type streamLine struct {
	Type         string         `json:"type"`
	Subtype      string         `json:"subtype"`
	SessionID    string         `json:"session_id"`
	Model        string         `json:"model"`
	Message      *streamMessage `json:"message"`
	DurationMS   int64          `json:"duration_ms"`
	Result       string         `json:"result"`
	Usage        *streamUsage   `json:"usage"`
	TotalCostUSD float64        `json:"total_cost_usd"`
}

type streamMessage struct {
	Content []streamBlock `json:"content"`
}

type streamBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	Content json.RawMessage `json:"content"`
	IsError bool            `json:"is_error"`
}

type streamUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// Feed parses one native stdout line into normalized events. Blank,
// oversized and non-JSON lines are skipped rather than erroring the stream;
// a trailing '\r' left by naive CRLF splitting is tolerated.
func (p *parser) Feed(line []byte) ([]core.EventInput, error) {
	line = bytes.TrimRight(line, "\r\n")
	if len(line) == 0 || len(line) > maxLineBytes {
		return nil, nil
	}
	var raw streamLine
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, nil //nolint:nilerr // non-JSON lines are garbage, not stream failures; never error the stream.
	}
	switch raw.Type {
	case "system":
		return parseSystem(raw)
	case "assistant":
		return parseAssistant(raw)
	case "user":
		return parseUser(raw)
	case "result":
		return parseResult(raw)
	default:
		return nil, nil
	}
}

// Close flushes trailing state at EOF. Claude Code's stream carries none.
func (p *parser) Close() ([]core.EventInput, error) { return nil, nil }
