//go:build darwin

package notify

import (
	"slices"
	"strings"
	"testing"
)

func TestTerminalNotifierArgs(t *testing.T) {
	n := Notification{
		NodeID:   "node-1",
		Title:    "grove: T",
		Subtitle: "permission",
		Body:     "needs approval",
		Sound:    true,
		URL:      "http://127.0.0.1:7433/n/node-1",
	}
	args := terminalNotifierArgs(n)

	pairs := map[string]string{
		"-title":    "grove: T",
		"-message":  "needs approval",
		"-group":    "grove-node-1",
		"-subtitle": "permission",
		"-sound":    "default",
		"-open":     "http://127.0.0.1:7433/n/node-1",
	}
	for flag, want := range pairs {
		i := slices.Index(args, flag)
		if i < 0 || i+1 >= len(args) {
			t.Errorf("missing flag %s in %v", flag, args)
			continue
		}
		if got := args[i+1]; got != want {
			t.Errorf("%s = %q, want %q", flag, got, want)
		}
	}
}

func TestTerminalNotifierArgsOmitsOptional(t *testing.T) {
	args := terminalNotifierArgs(Notification{NodeID: "n", Title: "t", Body: "b"})
	for _, unwanted := range []string{"-sound", "-open", "-subtitle"} {
		if slices.Contains(args, unwanted) {
			t.Errorf("unexpected %s for a silent, link-less, subtitle-less banner: %v", unwanted, args)
		}
	}
}

func TestOsascriptBody(t *testing.T) {
	body := osascriptBody(Notification{Title: "grove: T", Subtitle: "permission", Body: "needs approval", Sound: true})
	for _, want := range []string{
		`display notification "needs approval"`,
		`with title "grove: T"`,
		`subtitle "permission"`,
		`sound name "Glass"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("osascript body missing %q; got: %s", want, body)
		}
	}

	silent := osascriptBody(Notification{Title: "t", Body: "b"})
	if strings.Contains(silent, "sound name") {
		t.Errorf("silent banner must not request a sound: %s", silent)
	}
}

func TestAppleQuoteEscapes(t *testing.T) {
	got := appleQuote(`say "hi"\n` + "\nline2")
	want := `"say \"hi\"\\n line2"`
	if got != want {
		t.Errorf("appleQuote = %q, want %q", got, want)
	}
}
