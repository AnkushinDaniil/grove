package opencode

import (
	"bytes"
	"encoding/json"
)

// summaryLen is the approximate character cap on event summaries surfaced
// to the UI (tool_call input, tool_result content), matching the claude and
// codex drivers' convention.
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
