//go:build darwin

package notify

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// execTimeout bounds a single notifier invocation so a hung notifier can never
// pin a goroutine indefinitely.
const execTimeout = 3 * time.Second

// macSink delivers banners through terminal-notifier when it is on PATH, else
// through the always-present osascript. Each Notify runs the notifier in its own
// goroutine under execTimeout and never blocks or fails the caller.
type macSink struct {
	logger *slog.Logger
}

// newPlatformSink returns the macOS sink.
func newPlatformSink(logger *slog.Logger) Sink {
	return &macSink{logger: logger}
}

// Notify dispatches asynchronously; delivery problems surface only in debug logs.
func (m *macSink) Notify(n Notification) {
	go m.dispatch(n)
}

func (m *macSink) dispatch(n Notification) {
	ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
	defer cancel()

	var (
		cmd *exec.Cmd
		via string
	)
	if path, err := exec.LookPath("terminal-notifier"); err == nil {
		via = "terminal-notifier"
		//nolint:gosec // G204: args are grove-internal notification fields, not shell-interpreted.
		cmd = exec.CommandContext(ctx, path, terminalNotifierArgs(n)...)
	} else {
		via = "osascript"
		//nolint:gosec // G204: the script is built from escaped internal fields; osascript args are not shell-interpreted.
		cmd = exec.CommandContext(ctx, "osascript", "-e", osascriptBody(n))
	}
	m.logger.Debug("notify dispatch", "via", via, "node", n.NodeID, "title", n.Title, "sound", n.Sound)
	if out, err := cmd.CombinedOutput(); err != nil {
		m.logger.Debug("notify dispatch failed", "err", err, "output", strings.TrimSpace(string(out)))
	}
}

// terminalNotifierArgs builds the terminal-notifier flag list. -group keys the
// banner so a node's newer banner replaces its older one; -open sets the click
// deep link; -sound plays the default alert.
func terminalNotifierArgs(n Notification) []string {
	args := []string{"-title", n.Title, "-message", n.Body, "-group", "grove-" + string(n.NodeID)}
	if n.Subtitle != "" {
		args = append(args, "-subtitle", n.Subtitle)
	}
	if n.Sound {
		args = append(args, "-sound", "default")
	}
	if n.URL != "" {
		args = append(args, "-open", n.URL)
	}
	return args
}

// osascriptBody builds the AppleScript for the osascript fallback. osascript has
// no click-through, so the URL is dropped; sound uses the named "Glass" alert.
func osascriptBody(n Notification) string {
	var b strings.Builder
	b.WriteString("display notification ")
	b.WriteString(appleQuote(n.Body))
	b.WriteString(" with title ")
	b.WriteString(appleQuote(n.Title))
	if n.Subtitle != "" {
		b.WriteString(" subtitle ")
		b.WriteString(appleQuote(n.Subtitle))
	}
	if n.Sound {
		b.WriteString(` sound name "Glass"`)
	}
	return b.String()
}

// appleQuote renders s as an AppleScript double-quoted string literal, escaping
// backslashes and quotes and flattening newlines so the -e argument stays a
// single well-formed statement.
func appleQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ", "\r", " ")
	return `"` + r.Replace(s) + `"`
}
