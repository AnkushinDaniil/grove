// Package notify turns node attention transitions into desktop notifications.
// It exposes a small Sink seam (the platform notifier), a Coalescer that damps
// notification storms, and a Runner that watches the tree and drives the Sink.
// The whole package is best-effort: a notifier that is missing, slow or failing
// must never block or fail the daemon, so delivery errors are logged at debug
// and every exec is bounded and asynchronous.
package notify

import (
	"log/slog"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// Notification is one desktop banner to display. Sound selects an audible cue;
// URL is an optional deep link opened when the banner is clicked (notifier
// permitting). NodeID groups a node's banners so a newer one replaces the older.
type Notification struct {
	NodeID   core.NodeID
	Title    string
	Subtitle string
	Body     string
	Sound    bool
	URL      string
}

// Sink delivers a Notification. Implementations must not block the caller:
// platform sinks dispatch asynchronously and swallow errors (debug-logged).
type Sink interface {
	Notify(n Notification)
}

// NopSink drops every notification. It is the sink on non-darwin platforms and
// a safe default when notifications are disabled.
type NopSink struct{}

// Notify implements Sink by doing nothing.
func (NopSink) Notify(Notification) {}

// New returns the platform's default Sink: a macOS notifier on darwin (preferring
// terminal-notifier, falling back to osascript), NopSink elsewhere.
func New(logger *slog.Logger) Sink {
	if logger == nil {
		logger = slog.Default()
	}
	return newPlatformSink(logger)
}

// notificationFor builds the notification for a node whose attention just became
// non-none. It reports false for attention kinds the v1 policy does not notify
// on. Sound fires for the interactive kinds (permission/question/error); a plain
// done is silent.
func notificationFor(node core.Node, daemonURL string) (Notification, bool) {
	var sound bool
	switch node.Attention {
	case core.AttentionPermission, core.AttentionQuestion, core.AttentionError:
		sound = true
	case core.AttentionDone:
		sound = false
	case core.AttentionNone, core.AttentionReview:
		// none never notifies; review is surfaced in the inbox, not as a banner.
		return Notification{}, false
	default:
		return Notification{}, false
	}
	return Notification{
		NodeID:   node.ID,
		Title:    "grove: " + node.Title,
		Subtitle: string(node.Attention),
		Body:     node.AttentionReason,
		Sound:    sound,
		URL:      nodeURL(daemonURL, node.ID),
	}, true
}

// nodeURL builds the deep link to a node's page, tolerating a trailing slash on
// the daemon base URL.
func nodeURL(daemonURL string, id core.NodeID) string {
	if daemonURL == "" {
		return ""
	}
	base := daemonURL
	if base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base + "/n/" + string(id)
}
