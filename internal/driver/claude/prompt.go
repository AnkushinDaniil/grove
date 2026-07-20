package claude

import (
	"encoding/json"
	"fmt"
)

// promptLine is the stream-json stdin shape Claude Code expects for a
// follow-up user turn on a HeadlessStream session.
type promptLine struct {
	Type    string        `json:"type"`
	Message promptMessage `json:"message"`
}

type promptMessage struct {
	Role    string        `json:"role"`
	Content []promptBlock `json:"content"`
}

type promptBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// FormatPrompt encodes text as a stream-json user-turn stdin line, newline
// terminated as Claude Code's stdin reader expects.
func (claudeDriver) FormatPrompt(text string) ([]byte, error) {
	line := promptLine{
		Type: "user",
		Message: promptMessage{
			Role:    "user",
			Content: []promptBlock{{Type: "text", Text: text}},
		},
	}
	b, err := json.Marshal(line)
	if err != nil {
		return nil, fmt.Errorf("marshal prompt line: %w", err)
	}
	return append(b, '\n'), nil
}
