package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AnkushinDaniil/grove/internal/api"
	"github.com/AnkushinDaniil/grove/internal/config"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/driver/claude"
	"github.com/AnkushinDaniil/grove/internal/driver/codex"
	"github.com/AnkushinDaniil/grove/internal/driver/gemini"
	"github.com/AnkushinDaniil/grove/internal/driver/opencode"
	"github.com/AnkushinDaniil/grove/internal/gitcli"
	"github.com/AnkushinDaniil/grove/internal/memory"
	"github.com/AnkushinDaniil/grove/internal/notify"
	"github.com/AnkushinDaniil/grove/internal/push"
	"github.com/AnkushinDaniil/grove/internal/server"
	"github.com/AnkushinDaniil/grove/internal/session"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/tmux"
	"github.com/AnkushinDaniil/grove/internal/tree"
	usageagg "github.com/AnkushinDaniil/grove/internal/usage"
	"github.com/AnkushinDaniil/grove/internal/worktree"
	"github.com/AnkushinDaniil/grove/internal/ws"
)

// defaultPort is the daemon's loopback TCP port (the only listener).
const defaultPort = 7433

// runServe starts the daemon: it resolves the state layout, opens the store,
// recovers from any unclean shutdown, rebuilds the tree, and serves the HTTP
// surface until interrupted.
func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "TCP port to listen on (bound to 127.0.0.1)")
	home := fs.String("home", "", "grove state directory (default ~/.grove or $GROVE_HOME)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse serve flags: %w", err)
	}

	logger := newLogger()
	slog.SetDefault(logger)

	layout, err := resolveLayout(*home)
	if err != nil {
		return err
	}
	if err := layout.Ensure(); err != nil {
		return fmt.Errorf("ensure state layout: %w", err)
	}
	token, err := config.LoadOrCreateToken(layout.TokenPath)
	if err != nil {
		return fmt.Errorf("load daemon token: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv, err := buildServer(ctx, logger, layout, token, *port)
	if err != nil {
		return err
	}
	return srv.Run(ctx)
}

// buildServer opens the store, recovers state, and wires every component into a
// Server. On any setup failure it closes the store before returning, since
// ownership only transfers to the Server (which closes it on shutdown) on
// success.
func buildServer(ctx context.Context, logger *slog.Logger, layout config.Layout, token string, port int) (*server.Server, error) {
	st, err := store.Open(ctx, layout.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	if _, err := st.MarkInterrupted(ctx, time.Now()); err != nil {
		return nil, closeOnErr(st, fmt.Errorf("recover interrupted sessions: %w", err))
	}
	// Heal resume ids for sessions recorded before hook wiring existed: their
	// scrollback farewells carry the authoritative conversation id.
	if n, err := session.BackfillResumeIDs(ctx, st, layout.Scrollback); err != nil {
		logger.Warn("backfill resume ids", "err", err)
	} else if n > 0 {
		logger.Info("backfilled resume ids from scrollback farewells", "sessions", n)
	}
	nodes, sessions, err := st.LoadLive(ctx)
	if err != nil {
		return nil, closeOnErr(st, fmt.Errorf("load live state: %w", err))
	}
	tr := tree.New(st)
	if err := tr.Load(nodes, sessions); err != nil {
		return nil, closeOnErr(st, fmt.Errorf("rebuild tree: %w", err))
	}
	root, err := tr.Bootstrap(ctx, "Workspace")
	if err != nil {
		return nil, closeOnErr(st, fmt.Errorf("bootstrap workspace: %w", err))
	}
	// Every node inherits its driver down the tree; give the root a default so
	// a fresh install can start sessions immediately (override per node later).
	if root.Driver == "" {
		defaultDriver := claude.New().ID()
		if _, err := tr.UpdateNode(ctx, root.ID, tree.Patch{Driver: &defaultDriver}); err != nil {
			return nil, closeOnErr(st, fmt.Errorf("set default workspace driver: %w", err))
		}
	}

	reg, err := driver.NewRegistry(claude.New(), codex.New(), gemini.New(), opencode.New())
	if err != nil {
		return nil, closeOnErr(st, fmt.Errorf("build driver registry: %w", err))
	}

	// Hook wiring shares one token registry between the session manager (which
	// mints a per-node token into each native-hook launch) and the API (which
	// validates the agent's callbacks against it).
	daemonURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	hookCmd := hookCommand(logger)
	hookTokens := api.NewHookTokens()

	mgr := session.NewManager(reg, tr, session.Config{
		ScrollbackDir: layout.Scrollback,
		UseTmux:       tmux.Available(),
		HookCommand:   hookCmd,
		DaemonURL:     daemonURL,
		MintHookToken: hookTokens.Mint,
		Profiles:      session.NewStoreProfileLookup(st),
	})
	// Revive interactive sessions whose tmux-hosted child outlived the previous
	// daemon: MarkInterrupted flipped them to interrupted above, and Reattach
	// flips the survivors back to running now that the tree is loaded.
	if n, err := mgr.Reattach(ctx); err != nil {
		logger.Warn("reattach surviving sessions", "err", err)
	} else if n > 0 {
		logger.Info("reattached surviving sessions", "sessions", n)
	}
	// Start the MCP control plane + event-wake scheduler (owns the daemon's Unix
	// socket); both loops stop when ctx is canceled. mcpTokens is the per-node
	// token registry; the API layer will consume it to mount WithOrchestration on
	// the root orchestrator launch in a later wave.
	// Orchestration failure must not sink the daemon: if the MCP socket can't
	// bind (e.g. a GROVE_HOME path longer than the OS Unix-socket limit), log it
	// and run without the tree-of-agents control plane — headless sessions then
	// run plain instead of as orchestrators.
	// Keep as an interface so a failed/nil scheduler stays a nil interface
	// (a nil *orch.Scheduler boxed in the interface would be non-nil and panic
	// on call). A nil orchestrator makes headless sessions run plain.
	// grove is a MemPalace MCP client for zero-touch memory (ORCHESTRATION.md §8):
	// recall injection into spawn briefings, auto-capture on completion, and the
	// node-memory REST endpoint. The client degrades gracefully when MemPalace is
	// absent (every op reports unavailable rather than failing), so it is always
	// constructed; writes made while it is down spool under the state dir and
	// replay on the next healthy write.
	mem := memory.NewClient(memory.Options{
		Tree:      tr,
		SpoolPath: filepath.Join(layout.Home, "spool", "memory.jsonl"),
		Logger:    logger,
	})

	var orchestrator api.Orchestrator
	if scheduler, oerr := startOrchestration(ctx, logger, layout, tr, mgr, mem); oerr != nil {
		logger.Error("orchestration disabled", "err", oerr)
	} else {
		orchestrator = scheduler
	}

	engine := worktree.NewEngine(gitcli.NewRunner(), layout.Worktrees, time.Now)
	auth := api.NewAuth(token)

	// The daemon's VAPID keypair is generated once and persisted in settings;
	// a browser's subscription is bound to the public key, so it must survive
	// restarts (see internal/push.GenerateOrLoadKeys).
	pushKeys, err := push.GenerateOrLoadKeys(ctx, st)
	if err != nil {
		return nil, closeOnErr(st, fmt.Errorf("generate vapid keys: %w", err))
	}
	pushDispatcher := push.New(push.Config{Store: st, Keys: pushKeys, Logger: logger})

	apiHandlers := api.New(api.Config{
		Tree:          tr,
		Sessions:      mgr,
		Store:         st,
		Worktrees:     engine,
		Auth:          auth,
		HookTokens:    hookTokens,
		Orchestrator:  orchestrator,
		Memory:        mem,
		Logger:        logger,
		Version:       version,
		Commit:        commit,
		ProfilesDir:   layout.Profiles,
		PushPublicKey: pushKeys.PublicKey(),
	})
	wsHandlers := ws.New(ws.Config{
		Tree:          tr,
		Sessions:      mgr,
		Store:         st,
		Logger:        logger,
		ScrollbackDir: layout.Scrollback,
	})
	// Roll usage events up into usage_rollup so GET /usage serves live data; the
	// aggregator runs off ctx and does a final flush when the daemon shuts down.
	usageagg.New(st, tr, logger).Start(ctx)
	// Coalesce attention notifications through both the platform sink and Web
	// Push, and drive them off tree deltas; the server owns the runner's
	// start/stop lifecycle. MultiSink fans a notification out to both once the
	// coalescer has decided it should fire, so both channels see the same set.
	notifySink := notify.NewMultiSink(notify.New(logger), pushDispatcher)
	notifyRunner := notify.NewRunner(tr, notify.NewCoalescer(notifySink, time.Now), daemonURL, logger)
	//nolint:contextcheck // New is a pure constructor; the server owns its own lifetime context.
	return server.New(server.Config{
		Addr:     fmt.Sprintf("127.0.0.1:%d", port),
		Auth:     auth,
		API:      apiHandlers,
		WS:       wsHandlers,
		Sessions: mgr,
		Store:    st,
		Logger:   logger,
		Notify:   notifyRunner,
	}), nil
}

// closeOnErr closes the store and returns cause, joining any close failure.
func closeOnErr(st *store.Store, cause error) error {
	if cerr := st.Close(); cerr != nil {
		return fmt.Errorf("%w (store close also failed: %w)", cause, cerr)
	}
	return cause
}

// resolveLayout resolves the state layout, letting an explicit --home override
// the GROVE_HOME environment default.
func resolveLayout(home string) (config.Layout, error) {
	if home != "" {
		abs, err := filepath.Abs(home)
		if err != nil {
			return config.Layout{}, fmt.Errorf("resolve --home: %w", err)
		}
		if err := os.Setenv(config.EnvHome, abs); err != nil {
			return config.Layout{}, fmt.Errorf("set %s: %w", config.EnvHome, err)
		}
	}
	layout, err := config.ResolveLayout()
	if err != nil {
		return config.Layout{}, fmt.Errorf("resolve state layout: %w", err)
	}
	return layout, nil
}

// hookCommand builds the "<grove> hook" invocation embedded in generated agent
// hook settings so agents can phone events home. An unresolvable executable path
// disables hook wiring rather than failing startup.
func hookCommand(logger *slog.Logger) string {
	exe, err := os.Executable()
	if err != nil {
		logger.Warn("resolve executable for hook wiring", "err", err)
		return ""
	}
	return exe + " hook"
}

// newLogger builds the daemon's text logger; GROVE_LOG selects the level.
func newLogger() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("GROVE_LOG")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}
