// Package api implements grove's REST contract (docs/API.md) as thin handlers
// over the domain packages: tree state, the session manager, the SQLite store
// and the worktree engine. Handlers translate between the JSON wire DTOs and
// core types and never hold business logic of their own.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/AnkushinDaniil/grove/internal/session"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/tree"
	"github.com/AnkushinDaniil/grove/internal/worktree"
)

// maxBodyBytes caps request bodies; all grove payloads are small JSON documents.
const maxBodyBytes = 1 << 20

// Auth-exempt request paths referenced by the server's auth middleware: the two
// login endpoints (no cookie yet) and the hook callback (authenticated by its
// own per-node token header instead of the session cookie).
const (
	PathAuthSession = "/api/v1/auth/session"
	PathAuthMe      = "/api/v1/auth/me"
	PathHook        = "/api/v1/internal/hook"
)

// Config carries the Handlers dependencies.
type Config struct {
	Tree       *tree.Tree
	Sessions   *session.Manager
	Store      *store.Store
	Worktrees  *worktree.Engine
	Auth       *Auth
	HookTokens *HookTokens
	Logger     *slog.Logger

	Version string
	Commit  string

	// Home resolves the daemon user's home directory for filesystem completion
	// (GET /fs/dirs). Injected as a seam so tests are independent of $HOME; nil
	// defaults to os.UserHomeDir.
	Home func() (string, error)
}

// Handlers serves the REST contract over the domain packages.
type Handlers struct {
	tree       *tree.Tree
	sessions   *session.Manager
	store      *store.Store
	worktrees  *worktree.Engine
	auth       *Auth
	hookTokens *HookTokens
	logger     *slog.Logger

	version string
	commit  string
	home    func() (string, error)
}

// New builds Handlers from cfg.
func New(cfg Config) *Handlers {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	home := cfg.Home
	if home == nil {
		home = os.UserHomeDir
	}
	return &Handlers{
		tree:       cfg.Tree,
		sessions:   cfg.Sessions,
		store:      cfg.Store,
		worktrees:  cfg.Worktrees,
		auth:       cfg.Auth,
		hookTokens: cfg.HookTokens,
		logger:     logger,
		version:    cfg.Version,
		commit:     cfg.Commit,
		home:       home,
	}
}

// Routes registers every REST route on a fresh mux and returns it. The server
// mounts this under its auth and host-allowlist middleware.
func (h *Handlers) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/tree", h.handleTree)
	mux.HandleFunc("POST /api/v1/nodes", h.handleCreateNode)
	mux.HandleFunc("PATCH /api/v1/nodes/{id}", h.handlePatchNode)
	mux.HandleFunc("POST /api/v1/nodes/{id}/archive", h.handleArchiveNode)
	mux.HandleFunc("POST /api/v1/nodes/{id}/ack", h.handleAckNode)
	mux.HandleFunc("POST /api/v1/nodes/{id}/sessions", h.handleCreateSession)
	mux.HandleFunc("POST /api/v1/nodes/{id}/prompt", h.handlePrompt)
	mux.HandleFunc("POST /api/v1/sessions/{id}/stop", h.handleStopSession)
	mux.HandleFunc("GET /api/v1/nodes/{id}/events", h.handleListEvents)
	mux.HandleFunc("GET /api/v1/inbox", h.handleInbox)
	mux.HandleFunc("GET /api/v1/fs/dirs", h.handleFsDirs)
	mux.HandleFunc("GET /api/v1/version", h.handleVersion)
	mux.HandleFunc("GET /api/v1/usage", h.handleUsage)
	mux.HandleFunc("GET /api/v1/stats", h.handleStats)

	mux.HandleFunc("POST "+PathAuthSession, h.handleAuthSession)
	mux.HandleFunc("GET "+PathAuthMe, h.handleAuthMe)
	mux.HandleFunc("POST "+PathHook, h.handleHook)

	return mux
}

// decodeJSON reads and decodes a JSON request body with a size cap. An empty
// body leaves v at its zero value, so endpoints whose fields are all optional
// accept a missing body.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(v)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
