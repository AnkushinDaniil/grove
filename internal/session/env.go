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

// shimPathMarkers identify PATH entries belonging to third-party CLI shims
// that intercept `claude` and friends (cmux wraps the binary and alters
// session behavior non-deterministically — resume worked or failed depending
// on the wrapper's state). grove must always talk to the real CLI.
var shimPathMarkers = []string{"cmux-cli-shims", "/cmux.app/"}

func scrubEnv(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if hasAnyPrefix(e, scrubbedKeys) {
			continue
		}
		if strings.HasPrefix(e, "PATH=") {
			e = "PATH=" + sanitizePATH(strings.TrimPrefix(e, "PATH="))
		}
		out = append(out, e)
	}
	return append(out, "TERM=xterm-256color")
}

// sanitizePATH drops shim directories from a PATH value.
func sanitizePATH(path string) string {
	entries := strings.Split(path, string(os.PathListSeparator))
	kept := entries[:0]
	for _, entry := range entries {
		if containsAny(entry, shimPathMarkers) {
			continue
		}
		kept = append(kept, entry)
	}
	return strings.Join(kept, string(os.PathListSeparator))
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
