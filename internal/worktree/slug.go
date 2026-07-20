package worktree

import (
	"strings"
	"unicode"
)

const (
	maxSlugLen   = 24
	fallbackSlug = "task"
)

// slugify converts title into a lowercase, hyphen-separated slug usable as a
// single path element: only [a-z0-9-], no leading/trailing or repeated
// dashes, capped at maxSlugLen runes. Titles that yield no ASCII
// alphanumeric characters (e.g. pure Cyrillic or emoji) fall back to
// fallbackSlug.
func slugify(title string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		default:
			// drop punctuation and non-ASCII runes
		}
	}

	slug := strings.TrimRight(b.String(), "-")
	if len(slug) > maxSlugLen {
		slug = strings.TrimRight(slug[:maxSlugLen], "-")
	}
	if slug == "" {
		return fallbackSlug
	}
	return slug
}
