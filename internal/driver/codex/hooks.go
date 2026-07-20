package codex

import (
	"encoding/json"
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

// notifyFlag builds the `--config notify=[...]` override that wires grove's
// hook command as codex's turn-completion notifier (config field `notify:
// Option<Vec<String>>`, confirmed in codex-rs/core/src/config/mod.rs).
//
// KNOWN LIMITATION: codex invokes the notify command with this argv
// verbatim, then appends ONE extra argument containing a JSON payload
// describing the event, e.g. `<argv...> '{"type":"agent-turn-complete",...}'`
// (confirmed from codex's own source comment and example configs). That
// differs from Claude Code's hook delivery, which pipes the event JSON on
// stdin — the contract `grove hook` was built against per
// docs/ORCHESTRATION.md §2 ("≤1 MiB stdin JSON"). So today this wiring
// reliably invokes `<HookCommand> --driver codex --node <id> --token <tok>
// --daemon <url>` once per completed turn — a usable turn-boundary signal
// even though grove-hook does not currently parse the trailing JSON argv
// element — but it only fires on "agent-turn-complete"; there is no
// session-start/permission/session-end coverage. That scope matches
// docs/ORCHESTRATION.md's Codex row ("notify hook (turn-end)"), which this
// implementation follows rather than codex's newer, richer `hooks.*` TOML
// system (SessionStart/PreToolUse/PostToolUse/PermissionRequest/Stop/...,
// delivered via stdin JSON). That system was considered and rejected for
// now: it requires per-launch --dangerously-bypass-hook-trust (hook trust
// is keyed by command hash, and grove's command line changes every launch
// with a fresh --token), and its config shape is a TOML array-of-tables
// that -c's dotted-key overrides were not verified against a live probe.
// Consuming the notify command's trailing argument (or migrating to
// `hooks.Stop`) is future work in grove-hook's own parsing, out of this
// driver's scope.
func notifyFlag(h driver.HookWiring) ([]string, error) {
	cmd := []string{h.HookCommand, "--driver", "codex", "--node", string(h.NodeID), "--token", h.Token, "--daemon", h.DaemonURL}
	b, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal notify command: %w", err)
	}
	// A JSON array-of-strings is valid TOML syntax for the same value, so
	// this can be handed to codex's `--config key=value` (TOML-parsed
	// value) as-is.
	return []string{"--config", "notify=" + string(b)}, nil
}
