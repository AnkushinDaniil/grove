// Package ws serves grove's two WebSocket endpoints over the domain packages:
// /ws/state pushes tree snapshots and deltas, and /ws/term/{session_id} bridges
// a live PTY session (or replays a finished one's scrollback). It reuses the
// internal/api DTO mapping so the wire contract stays single-sourced.
package ws

import (
	"log/slog"
	"net/http"

	"github.com/coder/websocket"

	"github.com/AnkushinDaniil/grove/internal/session"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// Config carries the ws Handlers dependencies.
type Config struct {
	Tree     *tree.Tree
	Sessions *session.Manager
	Store    *store.Store
	Logger   *slog.Logger

	// ScrollbackDir is where finished PTY sessions' scrollback files live, used
	// to replay a terminal for a session that is no longer live.
	ScrollbackDir string
}

// Handlers serves /ws/state and /ws/term.
type Handlers struct {
	tree          *tree.Tree
	sessions      *session.Manager
	store         *store.Store
	logger        *slog.Logger
	scrollbackDir string
	acceptOpts    *websocket.AcceptOptions
}

// New builds ws Handlers from cfg.
func New(cfg Config) *Handlers {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Handlers{
		tree:          cfg.Tree,
		sessions:      cfg.Sessions,
		store:         cfg.Store,
		logger:        logger,
		scrollbackDir: cfg.ScrollbackDir,
		acceptOpts: &websocket.AcceptOptions{
			// The request host is always authorized (coder/websocket short-circuits
			// same-origin upgrades); these patterns additionally allow the loopback
			// origins the daemon and the Vite dev proxy use, plus Tailscale MagicDNS
			// names so the tree/terminal sockets live-update over `tailscale serve`
			// from a phone. The *.ts.net entry mirrors the Host allowlist's
			// tailnetHostSuffix (internal/server/middleware.go) and rests on the same
			// invariant: only devices already in the user's tailnet can originate a
			// *.ts.net Origin, and the daemon token still gates every upgrade. It is
			// needed for the case where `tailscale serve` rewrites the upstream Host
			// to loopback while the browser Origin stays the *.ts.net name, which
			// defeats the same-origin short-circuit above.
			OriginPatterns: []string{
				"127.0.0.1", "127.0.0.1:*",
				"localhost", "localhost:*",
				"*.ts.net", "*.ts.net:*",
			},
		},
	}
}

// Routes registers the WebSocket routes on a fresh mux and returns it. The
// server mounts this under its auth and host-allowlist middleware.
func (h *Handlers) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/state", h.serveState)
	mux.HandleFunc("GET /ws/term/{id}", h.serveTerm)
	return mux
}
