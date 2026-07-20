package session

import (
	"os"
	"strings"
)

// scrubbedKeys are dropped from the inherited environment so a spawned agent
// never sees the daemon's own credentials or config-dir; profile isolation
// supplies its own.
var scrubbedKeys = []string{
	"ANTHROPIC_API_KEY=",
	"CLAUDE_CONFIG_DIR=",
	"TERM=", // replaced with a deterministic value below
}

// DefaultBaseEnv is the base environment for spawned sessions: the daemon's
// environment minus the scrubbed keys, plus a deterministic TERM.
func DefaultBaseEnv() []string {
	return scrubEnv(os.Environ())
}

func scrubEnv(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if hasAnyPrefix(e, scrubbedKeys) {
			continue
		}
		out = append(out, e)
	}
	return append(out, "TERM=xterm-256color")
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
