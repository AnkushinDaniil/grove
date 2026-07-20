package gemini

import "github.com/AnkushinDaniil/grove/internal/driver"

// FormatPrompt is unsupported: gemini has no persistent stdin JSON stream
// for a headless run (HeadlessStream is false) — every turn, including
// grove's async wake batches, is its own `gemini -p "<text>" --resume <id>`
// invocation instead (docs/ORCHESTRATION.md §2 event-wake table).
func (geminiDriver) FormatPrompt(string) ([]byte, error) {
	return nil, driver.ErrUnsupported
}
