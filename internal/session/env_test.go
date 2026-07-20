package session

import (
	"slices"
	"strings"
	"testing"
)

func TestScrubEnv(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_API_KEY=secret",
		"CLAUDE_CONFIG_DIR=/home/x/.claude",
		"TERM=dumb",
		"FOO=bar",
	}
	out := scrubEnv(in)

	joined := strings.Join(out, "\n")
	for _, banned := range []string{"ANTHROPIC_API_KEY=", "CLAUDE_CONFIG_DIR="} {
		if strings.Contains(joined, banned) {
			t.Errorf("scrubEnv leaked %q", banned)
		}
	}
	if !contains(out, "PATH=/usr/bin") || !contains(out, "FOO=bar") {
		t.Errorf("scrubEnv dropped unrelated vars: %v", out)
	}
	terms := 0
	for _, e := range out {
		if strings.HasPrefix(e, "TERM=") {
			terms++
			if e != "TERM=xterm-256color" {
				t.Errorf("TERM = %q, want xterm-256color", e)
			}
		}
	}
	if terms != 1 {
		t.Errorf("TERM appears %d times, want exactly 1", terms)
	}
}

func contains(ss []string, want string) bool {
	return slices.Contains(ss, want)
}
