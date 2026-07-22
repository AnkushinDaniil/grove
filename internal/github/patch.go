package github

import (
	"regexp"
	"strconv"
	"strings"
)

// Hunk is one contiguous change region of a file diff: its raw @@ header and the
// sequence of lines within it.
type Hunk struct {
	Header string
	Lines  []DiffLine
}

// DiffLine is one line of a hunk. Op is " " (context), "+" (added) or "-"
// (removed). OldLine/NewLine are the 1-based line numbers on each side, 0 where
// the line does not exist on that side (0 for an added line's OldLine, 0 for a
// removed line's NewLine).
type DiffLine struct {
	Op      string
	OldLine int
	NewLine int
	Text    string
}

// hunkHeaderRE matches a unified-diff hunk header, capturing the old and new
// start lines. Counts are optional (`@@ -a +b @@` means a single line) and a
// trailing section heading after the closing @@ is ignored.
var hunkHeaderRE = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// parsePatch parses a unified-diff fragment (the GitHub files API `patch` field)
// into hunks with per-line old/new line numbers. It is tolerant: an empty patch
// (binary or too-large file) yields no hunks, and malformed lines are skipped
// rather than erroring. A "\ No newline at end of file" marker is dropped, as it
// is diff metadata rather than a content line.
func parsePatch(patch string) []Hunk {
	if patch == "" {
		return nil
	}
	lines := strings.Split(patch, "\n")
	// Drop a single trailing empty element left by a trailing newline.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	var hunks []Hunk
	cur := -1
	var oldLine, newLine int
	for _, ln := range lines {
		if strings.HasPrefix(ln, "@@") {
			m := hunkHeaderRE.FindStringSubmatch(ln)
			if m == nil {
				cur = -1
				continue
			}
			oldLine, _ = strconv.Atoi(m[1])
			newLine, _ = strconv.Atoi(m[2])
			hunks = append(hunks, Hunk{Header: ln})
			cur = len(hunks) - 1
			continue
		}
		if cur < 0 {
			continue
		}
		if ln == "" {
			// A blank context line whose single leading space was stripped.
			hunks[cur].Lines = append(hunks[cur].Lines, DiffLine{Op: " ", OldLine: oldLine, NewLine: newLine})
			oldLine++
			newLine++
			continue
		}
		op, text := ln[0], ln[1:]
		switch op {
		case ' ':
			hunks[cur].Lines = append(hunks[cur].Lines, DiffLine{Op: " ", OldLine: oldLine, NewLine: newLine, Text: text})
			oldLine++
			newLine++
		case '+':
			hunks[cur].Lines = append(hunks[cur].Lines, DiffLine{Op: "+", NewLine: newLine, Text: text})
			newLine++
		case '-':
			hunks[cur].Lines = append(hunks[cur].Lines, DiffLine{Op: "-", OldLine: oldLine, Text: text})
			oldLine++
		case '\\':
			// "\ No newline at end of file": metadata, not a content line.
			continue
		default:
			// Unknown prefix; skip tolerantly.
			continue
		}
	}
	return hunks
}
