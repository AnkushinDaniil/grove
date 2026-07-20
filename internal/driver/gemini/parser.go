package gemini

import (
	"bytes"
	"encoding/json"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

// maxBufferBytes caps the accepted headless output buffer; once exceeded,
// further input is silently dropped rather than growing without bound.
// gemini's `--output-format json` result is pretty-printed
// (JSON.stringify(_, null, 2), confirmed from
// packages/core/src/output/json-formatter.ts) and may be preceded by
// non-JSON log lines, so the parser cannot assume any single line is a
// complete JSON value — it buffers the whole run and parses once in Close.
const maxBufferBytes = 10 * 1024 * 1024

// parser is the stateful per-run driver.Parser for Gemini CLI's headless
// `--output-format json` output. Unlike claude/codex, gemini emits its
// result as ONE JSON value that is not guaranteed to be one line, so Feed
// only buffers; all parsing happens in Close once the stream is complete.
type parser struct {
	buf     []byte
	dropped bool
}

// NewParser returns a fresh Parser for one Gemini CLI run.
func (geminiDriver) NewParser() driver.Parser { return &parser{} }

// jsonOutput mirrors gemini-cli's JsonOutput
// (packages/core/src/output/types.ts): Response/Stats/Error are all
// optional so a completely empty object round-trips as "nothing found"
// rather than an error.
//
// SessionID is deliberately not decoded here: recent gemini-cli builds
// populate it (json-formatter.ts passes config.getSessionId() through when
// set), but the publicly documented schema (docs/cli/headless.md) does not
// mention it and Capabilities().EmitsSessionID is false for this driver —
// RecoverSessionID is the supported way to learn the session id, so this
// field is intentionally left unused rather than half-relied-on.
type jsonOutput struct {
	Response string           `json:"response"`
	Stats    *jsonStats       `json:"stats"`
	Error    *jsonOutputError `json:"error"`
}

// jsonStats mirrors SessionMetrics
// (packages/core/src/telemetry/uiTelemetry.ts): Models is keyed by model
// name, each entry carrying that model's cumulative token counts for the
// run. There is no run-level cost or duration field in this schema (unlike
// Claude's total_cost_usd/duration_ms), so UsagePayload.CostUSD and
// TurnDonePayload.DurationMS are left unset for this driver.
type jsonStats struct {
	Models map[string]jsonModelMetrics `json:"models"`
}

// jsonModelMetrics mirrors ModelMetrics's "tokens" field. Prompt is the raw,
// pre-cache-discount input token count — the same "total input tokens for
// the request" meaning as Claude's/codex's input_tokens — while gemini's own
// "input" field there nets out cached tokens and has no equivalent slot in
// core.UsagePayload, so it is not decoded here.
type jsonModelMetrics struct {
	Tokens struct {
		Prompt     int64 `json:"prompt"`
		Candidates int64 `json:"candidates"`
	} `json:"tokens"`
}

// jsonOutputError mirrors JsonError (packages/core/src/output/types.ts).
type jsonOutputError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Feed buffers the line; gemini's headless JSON result is not guaranteed to
// be one line (see the parser doc comment), so all parsing is deferred to
// Close. The buffer is capped at maxBufferBytes; once exceeded, further
// input is dropped rather than growing without bound — Close will then fail
// to find a complete JSON object and report nothing, the same
// garbage-tolerant outcome as any other malformed stream.
func (p *parser) Feed(line []byte) ([]core.EventInput, error) {
	line = bytes.TrimRight(line, "\r\n")
	if p.dropped || len(line) == 0 {
		return nil, nil
	}
	if len(p.buf)+len(line)+1 > maxBufferBytes {
		p.dropped = true
		return nil, nil
	}
	p.buf = append(p.buf, line...)
	p.buf = append(p.buf, '\n')
	return nil, nil
}

// Close parses the buffered output at EOF: it locates the last complete
// top-level JSON object in the buffer (skipping any preceding non-JSON log
// lines, and correctly ignoring brace characters inside JSON string values
// such as code the model echoed back in its response text) and maps it to
// events. A buffer with no complete JSON object, one that fails to
// unmarshal, or one that unmarshals to a completely empty object produces
// no events — never an error.
func (p *parser) Close() ([]core.EventInput, error) {
	span := findLastJSONObject(p.buf)
	if span == nil {
		return nil, nil
	}
	var out jsonOutput
	if err := json.Unmarshal(span, &out); err != nil {
		return nil, nil //nolint:nilerr // garbage-tolerant: an unparseable trailing object is not a stream failure.
	}

	hasResult := out.Response != "" || out.Stats != nil
	if !hasResult && out.Error == nil {
		return nil, nil
	}

	var events []core.EventInput
	if hasResult {
		textPayload, err := core.MarshalPayload(core.TextPayload{Text: out.Response, Final: true})
		if err != nil {
			return nil, err
		}
		turnPayload, err := core.MarshalPayload(core.TurnDonePayload{ResultText: out.Response})
		if err != nil {
			return nil, err
		}
		events = append(events,
			core.EventInput{Type: core.EventText, Payload: textPayload},
			core.EventInput{Type: core.EventTurnDone, Payload: turnPayload},
		)
		if out.Stats != nil {
			usagePayload, err := marshalUsage(out.Stats)
			if err != nil {
				return nil, err
			}
			events = append(events, core.EventInput{Type: core.EventUsage, Payload: usagePayload})
		}
	}
	if out.Error != nil {
		msg := out.Error.Message
		if msg == "" {
			msg = out.Error.Type
		}
		errPayload, err := core.MarshalPayload(core.ErrorPayload{Message: msg})
		if err != nil {
			return nil, err
		}
		events = append(events, core.EventInput{Type: core.EventError, Payload: errPayload})
	}
	return events, nil
}

// marshalUsage sums token counts across every model in stats.Models (a
// headless run almost always used exactly one, but the schema is a map) and
// sets Model only when exactly one model is present, to avoid attributing
// combined usage to the wrong model when several appear.
func marshalUsage(stats *jsonStats) (string, error) {
	usage := core.UsagePayload{}
	for name, m := range stats.Models {
		usage.InputTokens += m.Tokens.Prompt
		usage.OutputTokens += m.Tokens.Candidates
		if len(stats.Models) == 1 {
			usage.Model = name
		}
	}
	return core.MarshalPayload(usage)
}

// findLastJSONObject returns the last complete top-level {...} object in s,
// or nil if none is found. Brace characters inside JSON string literals are
// correctly ignored via a minimal string-aware scan (tracking quote state
// and backslash escapes), so response text containing "{" or "}" cannot
// fool the boundary detection. Nested objects only close their innermost
// span at depth>0, so only the outermost object of a value is recorded.
func findLastJSONObject(s []byte) []byte {
	start, end := -1, -1
	depth := 0
	inString := false
	escaped := false
	for i, b := range s {
		if inString {
			switch {
			case escaped:
				escaped = false
			case b == '\\':
				escaped = true
			case b == '"':
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					end = i + 1
				}
			}
		}
	}
	if start < 0 || end < 0 {
		return nil
	}
	return s[start:end]
}
