package term

import (
	"fmt"
	"os"
	"time"
)

// scrollbackFlushInterval is how often a dirty ring is persisted to disk.
const scrollbackFlushInterval = 5 * time.Second

// flushLoop periodically persists the ring while the process is alive. The
// final flush happens in finish once the process exits.
func (h *Handle) flushLoop() {
	ticker := time.NewTicker(scrollbackFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			h.flush()
		case <-h.done:
			return
		}
	}
}

// flush writes the ring to the scrollback file via a temp-file rename. It is a
// no-op when nothing changed since the last flush. Best effort: I/O errors are
// dropped because scrollback is a convenience cache, not the source of truth.
func (h *Handle) flush() {
	total := h.ring.Total()
	h.flushMu.Lock()
	defer h.flushMu.Unlock()
	if total == h.lastFlushed {
		return
	}
	if writeFileAtomic(h.scrollbackPath, h.ring.Bytes()) == nil {
		h.lastFlushed = total
	}
}

// writeFileAtomic writes data to path via a sibling temp file and a rename, so
// a reader never observes a partially written scrollback.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write scrollback temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename scrollback: %w", err)
	}
	return nil
}

// LoadScrollback reads a scrollback file persisted by a prior run, for replay
// after a daemon restart.
func LoadScrollback(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load scrollback %s: %w", path, err)
	}
	return data, nil
}
