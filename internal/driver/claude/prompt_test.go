package claude

import (
	"encoding/json"
	"testing"
)

func TestFormatPrompt(t *testing.T) {
	d := New()
	b, err := d.FormatPrompt("hello there")
	if err != nil {
		t.Fatalf("FormatPrompt() error = %v", err)
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		t.Fatalf("FormatPrompt() = %q, want trailing newline", b)
	}

	var line promptLine
	if err := json.Unmarshal(b[:len(b)-1], &line); err != nil {
		t.Fatalf("unmarshal prompt line: %v", err)
	}
	if line.Type != "user" {
		t.Errorf("Type = %q, want %q", line.Type, "user")
	}
	if line.Message.Role != "user" {
		t.Errorf("Message.Role = %q, want %q", line.Message.Role, "user")
	}
	if len(line.Message.Content) != 1 || line.Message.Content[0].Type != "text" ||
		line.Message.Content[0].Text != "hello there" {
		t.Errorf("Message.Content = %+v, want single text block %q", line.Message.Content, "hello there")
	}
}
