package claude

import (
	"encoding/json"
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

// hookSettingsPath is the ExecSpec.Files entry (Dir-relative) grove writes
// the generated hook settings to, and the path passed to --settings.
const hookSettingsPath = ".grove/claude-settings.json"

// hookCommand is one entry in a hook matcher's "hooks" array.
type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// hookMatcher is one entry in a hook event's array. grove wires no matcher
// pattern, so the single entry always fires.
type hookMatcher struct {
	Hooks []hookCommand `json:"hooks"`
}

// hookEvents lists the Claude Code hook events grove wires for attention
// detection.
type hookEvents struct {
	SessionStart []hookMatcher `json:"SessionStart"`
	Notification []hookMatcher `json:"Notification"`
	Stop         []hookMatcher `json:"Stop"`
	SessionEnd   []hookMatcher `json:"SessionEnd"`
}

// claudeSettings is the full Claude Code --settings JSON document grove
// generates for a launch.
type claudeSettings struct {
	Hooks hookEvents `json:"hooks"`
}

// marshalHookSettings builds the settings JSON wiring every event grove
// listens for to a single grove hook command invocation.
func marshalHookSettings(h driver.HookWiring) (string, error) {
	cmd := fmt.Sprintf("%s --node %s --token %s --daemon %s", h.HookCommand, h.NodeID, h.Token, h.DaemonURL)
	matcher := []hookMatcher{{Hooks: []hookCommand{{Type: "command", Command: cmd}}}}
	settings := claudeSettings{
		Hooks: hookEvents{
			SessionStart: matcher,
			Notification: matcher,
			Stop:         matcher,
			SessionEnd:   matcher,
		},
	}
	b, err := json.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("marshal hook settings: %w", err)
	}
	return string(b), nil
}
