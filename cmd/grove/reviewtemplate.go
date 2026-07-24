package main

import (
	"log/slog"
	"os"
	"path/filepath"
)

// reviewGuidelinesTemplate seeds <GROVE_HOME>/review.md on first run so the
// feature is discoverable and, since it ships with the ASCII-only rule the
// user's setup enforces, has an immediate effect. Kept plain ASCII itself.
const reviewGuidelinesTemplate = `# grove review guidelines
#
# grove injects this file into "Review with AI" so the reviewer follows YOUR
# house style and rules. Edit it freely; delete it to opt out. A repo-local
# .grove/review.md (committed with a repo) is appended after this one and can
# extend or override these rules.

## Formatting
- Write every review comment in plain ASCII only: use '-' instead of em or en
  dashes, '...' instead of an ellipsis character, and no decorative Unicode.

## Style
- (Add your review style here: tone, what to flag vs skip, an example of a good
  comment, naming and structure conventions, and anything else you want the
  reviewer to follow.)
`

// ensureReviewTemplate writes the starter guidelines file when none exists. Any
// error is logged and ignored: a missing template just means no injected
// guidelines, never a failed startup.
func ensureReviewTemplate(home string, logger *slog.Logger) {
	path := filepath.Join(home, "review.md")
	if _, err := os.Stat(path); err == nil {
		return // already present (possibly user-edited); never overwrite
	}
	if err := os.WriteFile(path, []byte(reviewGuidelinesTemplate), 0o600); err != nil {
		logger.Warn("seed review guidelines template", "path", path, "err", err)
		return
	}
	logger.Info("seeded review guidelines template", "path", path)
}
