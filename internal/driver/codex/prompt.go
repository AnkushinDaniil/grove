package codex

import "github.com/AnkushinDaniil/grove/internal/driver"

// FormatPrompt is unsupported: codex has no persistent stdin JSON stream
// for a headless run (HeadlessStream is false) — each turn is its own
// `codex exec` or `codex exec resume` invocation instead.
func (codexDriver) FormatPrompt(string) ([]byte, error) {
	return nil, driver.ErrUnsupported
}
