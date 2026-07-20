package worktree

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"simple lowercase", "optimize rpc", "optimize-rpc"},
		{"mixed case", "Optimize RPC Handler", "optimize-rpc-handler"},
		{"punctuation collapses to dashes", "Fix bug: null pointer!!", "fix-bug-null-pointer"},
		{"repeated separators collapse", "a   b---c__d", "a-b-c-d"},
		{"leading and trailing separators trimmed", "  -Spaced Out-  ", "spaced-out"},
		{"digits kept", "Upgrade to v2 API", "upgrade-to-v2-api"},
		{"pure cyrillic falls back", "Оптимизация запроса", fallbackSlug},
		{"pure emoji falls back", "🚀🔥✨", fallbackSlug},
		{"empty title falls back", "", fallbackSlug},
		{"only punctuation falls back", "!!!???", fallbackSlug},
		{"mixed ascii and cyrillic keeps ascii", "Fix баг in parser", "fix-in-parser"},
		{"mixed ascii and emoji keeps ascii", "🚀 Deploy service", "deploy-service"},
		{
			"long title truncated to max length",
			"this is a very long task title that definitely exceeds the limit",
			"this-is-a-very-long-task",
		},
		{
			// Collapsed form is "aaa-bbb-ccc-ddd-eee-fff-ggg"; the 24th
			// character lands exactly on the dash after "fff", so
			// truncation must also trim the resulting trailing dash.
			"truncation landing on a dash trims it",
			"aaa bbb ccc ddd eee fff ggg",
			"aaa-bbb-ccc-ddd-eee-fff",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slugify(tt.title)
			if got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.title, got, tt.want)
			}
			if len(got) > maxSlugLen {
				t.Errorf("slugify(%q) = %q, length %d exceeds max %d", tt.title, got, len(got), maxSlugLen)
			}
			if got != "" && (got[0] == '-' || got[len(got)-1] == '-') {
				t.Errorf("slugify(%q) = %q, has leading/trailing dash", tt.title, got)
			}
		})
	}
}
