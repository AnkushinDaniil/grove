package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

func TestNotifyFlag(t *testing.T) {
	h := driver.HookWiring{
		HookCommand: "/usr/local/bin/grove-hook",
		DaemonURL:   "http://127.0.0.1:4123",
		NodeID:      core.NodeID("node-1"),
		Token:       "hook-tok",
	}
	flags, err := notifyFlag(h)
	if err != nil {
		t.Fatalf("notifyFlag() error = %v", err)
	}
	if len(flags) != 2 || flags[0] != "--config" {
		t.Fatalf("notifyFlag() = %v, want [--config notify=...]", flags)
	}
	value, ok := strings.CutPrefix(flags[1], "notify=")
	if !ok {
		t.Fatalf("notifyFlag() value = %q, want notify= prefix", flags[1])
	}

	var cmd []string
	if err := json.Unmarshal([]byte(value), &cmd); err != nil {
		t.Fatalf("unmarshal notify array: %v", err)
	}
	want := []string{
		"/usr/local/bin/grove-hook", "--driver", "codex",
		"--node", "node-1", "--token", "hook-tok", "--daemon", "http://127.0.0.1:4123",
	}
	if len(cmd) != len(want) {
		t.Fatalf("notify cmd = %v, want %v", cmd, want)
	}
	for i := range want {
		if cmd[i] != want[i] {
			t.Errorf("notify cmd[%d] = %q, want %q", i, cmd[i], want[i])
		}
	}
}
