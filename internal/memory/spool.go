package memory

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// The spool is grove's durability net for auto-capture: when MemPalace is down
// mid-flight, writes append to ~/.grove/spool/memory.jsonl (one drawerWrite per
// line) instead of being lost, and the next successful live write drains them
// back in order (ORCHESTRATION.md §8: "writes spool ... and replay on recovery").

// spoolWrite appends one write to the spool file (creating its directory). It is
// the last-resort path, so a spool failure is the only error Write surfaces.
func (c *Client) spoolWrite(w drawerWrite) error {
	if c.spool == "" {
		return fmt.Errorf("memory backend unavailable and no spool configured")
	}
	c.replayMu.Lock()
	defer c.replayMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(c.spool), 0o700); err != nil {
		return fmt.Errorf("create spool dir: %w", err)
	}
	line, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("marshal spooled write: %w", err)
	}
	f, err := os.OpenFile(c.spool, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open spool: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append spool: %w", err)
	}
	return nil
}

// drainSpool replays buffered writes into a live backend, oldest first, and
// removes the spool once every entry lands. A write that fails mid-drain stops
// the pass and rewrites the file with the remaining backlog (including the one
// that failed), so nothing is lost and the next healthy write retries. Called
// only when the backend has just answered, so it does not pay its own probe.
func (c *Client) drainSpool(ctx context.Context) {
	if c.spool == "" {
		return
	}
	c.replayMu.Lock()
	defer c.replayMu.Unlock()

	pending, err := readSpool(c.spool)
	if err != nil || len(pending) == 0 {
		return
	}
	for i, w := range pending {
		if err := c.writeOnce(ctx, w); err != nil {
			// Backend went away again: persist the unflushed remainder and stop.
			c.log.Debug("spool replay interrupted; re-spooling remainder", "flushed", i, "remaining", len(pending)-i, "err", err)
			if werr := writeSpool(c.spool, pending[i:]); werr != nil {
				c.log.Warn("re-spool remainder failed", "err", werr)
			}
			return
		}
	}
	if err := os.Remove(c.spool); err != nil && !os.IsNotExist(err) {
		c.log.Warn("remove drained spool failed", "err", err)
	}
}

// readSpool loads every buffered write in file order. Malformed lines are skipped
// (a partial line from a crash mid-append must not wedge the whole backlog).
func readSpool(path string) ([]drawerWrite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read spool: %w", err)
	}
	var out []drawerWrite
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var w drawerWrite
		if err := json.Unmarshal(line, &w); err != nil {
			continue
		}
		out = append(out, w)
	}
	return out, nil
}

// writeSpool atomically replaces the spool with the given backlog (temp file +
// rename), so an interrupted rewrite never truncates the record.
func writeSpool(path string, writes []drawerWrite) error {
	var buf bytes.Buffer
	for _, w := range writes {
		line, err := json.Marshal(w)
		if err != nil {
			return fmt.Errorf("marshal spooled write: %w", err)
		}
		buf.Write(append(line, '\n'))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create spool dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write spool temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace spool: %w", err)
	}
	return nil
}
