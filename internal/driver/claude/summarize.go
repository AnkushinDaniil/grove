package claude

import (
	"bytes"
	"encoding/json"
	"strings"
)

// summaryLen is the approximate character cap on event summaries surfaced to
// the UI (tool_call input, tool_result content).
const summaryLen = 120

// compactTruncate compacts a raw JSON value (strips insignificant
// whitespace) and truncates it to at most limit runes.
func compactTruncate(raw []byte, limit int) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		buf.Reset()
		buf.Write(raw)
	}
	return truncate(buf.String(), limit)
}

// truncate cuts s to at most limit runes.
func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}

// flattenToolResultContent reduces a tool_result "content" field — either a
// plain string or an array of {"type":"text","text":...} blocks in the
// Claude API content-block shape — to plain text for the event summary.
// Unrecognized shapes fall back to the raw JSON.
func flattenToolResultContent(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		texts := make([]string, 0, len(blocks))
		for _, b := range blocks {
			if b.Text != "" {
				texts = append(texts, b.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	return string(raw)
}
