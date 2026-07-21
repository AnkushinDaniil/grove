package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Doctor runs the full diagnostic pass — Detect, palace check, MCP probe, and a
// read-only scan of the user's Claude settings for mempalace hook references —
// printing ✓/✗ lines to w. It reports whether the backend is healthy. Doctor
// never mutates anything; `grove memory install` performs repairs.
func (e Env) Doctor(ctx context.Context, w io.Writer) bool {
	if w == nil {
		w = io.Discard
	}
	st, err := e.Detect(ctx)
	if err != nil {
		fprintf(w, "✗ could not inspect environment: %v\n", err)
		return false
	}
	ok := true

	if st.Installed {
		fprintf(w, "✓ mempalace installed: %s (%s)\n", orUnknown(st.Version), st.Path)
	} else {
		ok = false
		fprintf(w, "✗ mempalace not installed — run `grove memory install`\n")
	}

	if st.PalaceExists {
		fprintf(w, "✓ palace present: %s\n", st.PalacePath)
	} else {
		ok = false
		fprintf(w, "✗ palace not initialized: %s (created by `grove memory install`)\n", st.PalacePath)
	}

	if st.Installed {
		rep, perr := e.Probe(ctx)
		switch {
		case perr != nil:
			ok = false
			fprintf(w, "✗ MCP probe failed: %v\n", perr)
		case rep.OK:
			server := rep.ServerName
			if server == "" {
				server = BinaryName
			}
			fprintf(w, "✓ MCP server healthy: %s, %d tools [%s]\n    %s\n",
				server, rep.ToolCount, strings.Join(rep.ToolNames, ", "), rep.Note)
		default:
			ok = false
			fprintf(w, "✗ MCP server unhealthy: %s\n", rep.Note)
		}
	} else {
		fprintf(w, "  (skipping MCP probe — binary not installed)\n")
	}

	e.reportClaudeHooks(w, st.Installed && ok)

	if ok {
		fprintf(w, "\nMemPalace memory backend is healthy.\n")
	} else {
		fprintf(w, "\nMemPalace memory backend needs attention (see ✗ above).\n")
	}
	return ok
}

// reportClaudeHooks scans the user's Claude settings (READ-ONLY) for hook
// commands that invoke mempalace and ties the finding to the install state. It
// never modifies the settings files.
func (e Env) reportClaudeHooks(w io.Writer, working bool) {
	home, err := e.home()
	if err != nil {
		return
	}
	files := []string{
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(home, ".claude", "settings.local.json"),
	}
	var found []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if jsonMentionsMempalace(data) {
			found = append(found, f)
		}
	}
	switch {
	case len(found) == 0:
		fprintf(w, "  note: no mempalace references in your user-level Claude settings (project .claude/settings*.json files are not scanned)\n")
	case working:
		fprintf(w, "  note: your Claude hooks reference mempalace (%s) — it works now, so the \"command not found\" hook error will disappear\n", strings.Join(found, ", "))
	default:
		fprintf(w, "  note: your Claude hooks reference mempalace (%s) but it is not working yet — run `grove memory install`\n", strings.Join(found, ", "))
	}
}

// jsonMentionsMempalace reports whether any string value in the JSON document
// mentions mempalace, falling back to a raw substring scan for non-JSON files.
func jsonMentionsMempalace(data []byte) bool {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return bytes.Contains(bytes.ToLower(data), []byte(BinaryName))
	}
	return valueMentions(v, BinaryName)
}

func valueMentions(v any, needle string) bool {
	switch t := v.(type) {
	case string:
		return strings.Contains(strings.ToLower(t), needle)
	case []any:
		for _, item := range t {
			if valueMentions(item, needle) {
				return true
			}
		}
	case map[string]any:
		for _, item := range t {
			if valueMentions(item, needle) {
				return true
			}
		}
	}
	return false
}

// fprintf writes a doctor report line, deliberately ignoring write errors:
// the report destination is the user's terminal (or io.Discard) and a failed
// diagnostic print must never abort the diagnosis itself.
func fprintf(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}
