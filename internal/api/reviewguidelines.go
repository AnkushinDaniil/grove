package api

import (
	"os"
	"path/filepath"
	"strings"
)

// maxGuidelinesBytes caps each guidelines file so a huge one can't crowd out the
// diff in the review prompt.
const maxGuidelinesBytes = 8 * 1024

// reviewGuidelines assembles the reviewer's own house style and rules to inject
// into a review/draft prompt, from two optional files:
//
//   - <GroveHome>/review.md   : global, applies to every repo
//   - <repo>/.grove/review.md : repo-local, appended after the global so it
//     extends or overrides it
//
// Either or both may be absent (returns ""). This is grove's answer to the fact
// that the constrained reviewer deliberately ignores the user's CLAUDE.md (which
// otherwise makes claude go agentic): the reviewer's actual preferences, such as
// "comments must be ASCII-only", are supplied here explicitly instead.
func (h *Handlers) reviewGuidelines(dir string) string {
	var parts []string
	if h.groveHome != "" {
		if s := readGuidelinesFile(filepath.Join(h.groveHome, "review.md")); s != "" {
			parts = append(parts, s)
		}
	}
	if dir != "" {
		if s := readGuidelinesFile(filepath.Join(dir, ".grove", "review.md")); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n")
}

// readGuidelinesFile reads and trims one guidelines file, capped in size. A
// missing or unreadable file yields "" (guidelines are best-effort).
func readGuidelinesFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(b))
	if len(s) > maxGuidelinesBytes {
		s = s[:maxGuidelinesBytes] + "\n... (guidelines truncated)"
	}
	return s
}

// guidelinesBlock wraps assembled guidelines in a labeled prompt section, or
// returns "" when there are none.
func guidelinesBlock(guidelines string) string {
	if strings.TrimSpace(guidelines) == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("The reviewer's own guidelines (their house style and rules). Follow them exactly, ")
	b.WriteString("including any formatting and wording conventions:\n")
	b.WriteString(guidelines)
	b.WriteString("\n")
	return b.String()
}
