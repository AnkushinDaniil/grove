// Package api implements grove's REST contract (docs/API.md) as thin handlers
// over the domain packages: tree state, the session manager, the SQLite store
// and the worktree engine. Handlers translate between the JSON wire DTOs and
// core types and never hold business logic of their own.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/gitcli"
	"github.com/AnkushinDaniil/grove/internal/github"
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

	// ProfilesDir is the root under which new profiles' config dirs default
	// (<ProfilesDir>/<driver>/<name>). It is layout.Profiles on the daemon.
	ProfilesDir string

	// GitHub wraps the gh CLI for the Review Radar endpoints. Nil defaults to a
	// client over the real gh binary; tests inject one backed by a fake runner.
	GitHub GitHubClient

	// Orchestrator launches a node's headless session with grove's MCP tools
	// mounted (the tree-of-agents control plane). Nil leaves headless sessions
	// plain — they run without the grove tools.
	Orchestrator Orchestrator

	// Memory backs GET /nodes/{id}/memory with MemPalace-recalled entries. Nil
	// makes the endpoint report an unavailable backend (healthy:false).
	Memory Memory
}

// Orchestrator launches a node as a root orchestrator: a headless session with
// grove's MCP server mounted, so the agent can spawn/track children and report
// through the tree. The scheduler propagates the mount to every child it spawns.
type Orchestrator interface {
	LaunchOrchestrator(ctx context.Context, nodeID core.NodeID, prompt, resumeID string) (core.Session, error)
}

// Handlers serves the REST contract over the domain packages.
type Handlers struct {
	tree         *tree.Tree
	sessions     *session.Manager
	store        *store.Store
	worktrees    *worktree.Engine
	auth         *Auth
	hookTokens   *HookTokens
	orchestrator Orchestrator
	memory       Memory
	logger       *slog.Logger

	version     string
	commit      string
	home        func() (string, error)
	profilesDir string
	github      GitHubClient
	git         *gitcli.Runner

	// stats caches computed GET /stats payloads for statsCacheTTL, keyed by
	// scope+range, so repeated dashboard polls do not re-aggregate the DB.
	stats *statsCache

	// aiDrafter runs a headless claude to draft PR review text (POST
	// /reviews/pr/ai-draft). A nil value falls back to the real claude exec
	// (defaultAIDrafter); tests override it to avoid shelling out.
	aiDrafter aiDraftFunc
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
	gh := cfg.GitHub
	if gh == nil {
		gh = github.New()
	}
	return &Handlers{
		tree:         cfg.Tree,
		sessions:     cfg.Sessions,
		store:        cfg.Store,
		worktrees:    cfg.Worktrees,
		auth:         cfg.Auth,
		hookTokens:   cfg.HookTokens,
		orchestrator: cfg.Orchestrator,
		memory:       cfg.Memory,
		logger:       logger,
		version:      cfg.Version,
		commit:       cfg.Commit,
		home:         home,
		profilesDir:  cfg.ProfilesDir,
		github:       gh,
		git:          gitcli.NewRunner(),
		stats:        newStatsCache(statsCacheTTL),
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
	mux.HandleFunc("GET /api/v1/nodes/{id}/memory", h.handleNodeMemory)
	mux.HandleFunc("GET /api/v1/nodes/{id}/resume-target", h.handleResumeTarget)
	mux.HandleFunc("GET /api/v1/inbox", h.handleInbox)
	mux.HandleFunc("GET /api/v1/fs/dirs", h.handleFsDirs)
	mux.HandleFunc("GET /api/v1/version", h.handleVersion)
	mux.HandleFunc("GET /api/v1/usage", h.handleUsage)
	mux.HandleFunc("GET /api/v1/stats", h.handleStats)

	mux.HandleFunc("POST /api/v1/feedback", h.handleCreateFeedback)
	mux.HandleFunc("GET /api/v1/feedback", h.handleListFeedback)
	mux.HandleFunc("POST /api/v1/feedback/{id}/resolve", h.handleResolveFeedback)

	mux.HandleFunc("GET /api/v1/reviews", h.handleReviews)
	mux.HandleFunc("GET /api/v1/reviews/sources", h.handleReviewSources)
	mux.HandleFunc("POST /api/v1/reviews/sources", h.handleSetReviewSources)
	mux.HandleFunc("POST /api/v1/reviews/start", h.handleReviewStart)

	mux.HandleFunc("GET /api/v1/reviews/pr", h.handlePRReview)
	mux.HandleFunc("GET /api/v1/reviews/pr/drafts", h.handleListDrafts)
	mux.HandleFunc("POST /api/v1/reviews/pr/drafts", h.handleCreateDraft)
	mux.HandleFunc("DELETE /api/v1/reviews/pr/drafts/{id}", h.handleDeleteDraft)
	mux.HandleFunc("POST /api/v1/reviews/pr/ai-draft", h.handleAIDraft)
	mux.HandleFunc("POST /api/v1/reviews/pr/submit", h.handleSubmitReview)
	mux.HandleFunc("POST /api/v1/reviews/pr/reply", h.handleReplyThread)

	mux.HandleFunc("GET /api/v1/projects/{id}/repos", h.handleListRepos)
	mux.HandleFunc("POST /api/v1/projects/{id}/repos", h.handleCreateRepo)
	mux.HandleFunc("DELETE /api/v1/repos/{id}", h.handleDeleteRepo)

	mux.HandleFunc("GET /api/v1/profiles", h.handleListProfiles)
	mux.HandleFunc("POST /api/v1/profiles", h.handleCreateProfile)
	mux.HandleFunc("DELETE /api/v1/profiles/{id}", h.handleDeleteProfile)
	mux.HandleFunc("GET /api/v1/profiles/{id}/doctor", h.handleProfileDoctor)

	mux.HandleFunc("GET /api/v1/reviews/worktree", h.handleWorktreeReview)
	mux.HandleFunc("GET /api/v1/reviews/worktree/comments", h.handleListWorktreeComments)
	mux.HandleFunc("POST /api/v1/reviews/worktree/comments", h.handleCreateWorktreeComment)
	mux.HandleFunc("DELETE /api/v1/reviews/worktree/comments/{id}", h.handleDeleteWorktreeComment)
	mux.HandleFunc("POST /api/v1/reviews/worktree/merge", h.handleWorktreeMerge)
	mux.HandleFunc("POST /api/v1/reviews/worktree/address", h.handleWorktreeAddress)

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
