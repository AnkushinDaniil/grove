package claude

import "testing"

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		limit int
		want  string
	}{
		{"under limit", "hello", 10, "hello"},
		{"exact limit", "hello", 5, "hello"},
		{"over limit", "hello world", 5, "hello"},
		{"empty", "", 5, ""},
		{"unicode truncation by rune", "héllo wörld", 6, "héllo "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.s, tt.limit); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.limit, got, tt.want)
			}
		})
	}
}

func TestCompactTruncate(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		limit int
		want  string
	}{
		{"empty", "", 120, ""},
		{"already compact", `{"a":1,"b":2}`, 120, `{"a":1,"b":2}`},
		{"strips insignificant whitespace", "{\n  \"a\": 1,\n  \"b\": 2\n}", 120, `{"a":1,"b":2}`},
		{"truncates after compacting", `{"a":1,"b":2}`, 5, `{"a":`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactTruncate([]byte(tt.raw), tt.limit); got != tt.want {
				t.Errorf("compactTruncate(%q, %d) = %q, want %q", tt.raw, tt.limit, got, tt.want)
			}
		})
	}
}

func TestFlattenToolResultContent(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"plain string", `"hello world"`, "hello world"},
		{"single text block", `[{"type":"text","text":"hi"}]`, "hi"},
		{"multiple text blocks joined", `[{"type":"text","text":"a"},{"type":"text","text":"b"}]`, "a\nb"},
		{"skips blocks with empty text", `[{"type":"text","text":""},{"type":"text","text":"b"}]`, "b"},
		{"unknown shape falls back to raw", `42`, "42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := flattenToolResultContent([]byte(tt.raw)); got != tt.want {
				t.Errorf("flattenToolResultContent(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
