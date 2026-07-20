package opencode

import "github.com/AnkushinDaniil/grove/internal/driver"

// FormatPrompt is unsupported: opencode has no persistent stdin JSON stream
// for a headless run (HeadlessStream is false) — each turn is its own
// `opencode run` (optionally `--session <id>`) invocation instead.
func (opencodeDriver) FormatPrompt(string) ([]byte, error) {
	return nil, driver.ErrUnsupported
}
