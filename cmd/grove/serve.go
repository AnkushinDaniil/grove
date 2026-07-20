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
	"github.com/AnkushinDaniil/grove/internal/gitcli"
	"github.com/AnkushinDaniil/grove/internal/server"
	"github.com/AnkushinDaniil/grove/internal/session"
	"github.com/AnkushinDaniil/grove/internal/store"
	"github.com/AnkushinDaniil/grove/internal/tree"
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
	nodes, sessions, err := st.LoadLive(ctx)
	if err != nil {
		return nil, closeOnErr(st, fmt.Errorf("load live state: %w", err))
	}
	tr := tree.New(st)
	if err := tr.Load(nodes, sessions); err != nil {
		return nil, closeOnErr(st, fmt.Errorf("rebuild tree: %w", err))
	}
	if _, err := tr.Bootstrap(ctx, "Workspace"); err != nil {
		return nil, closeOnErr(st, fmt.Errorf("bootstrap workspace: %w", err))
	}

	reg, err := driver.NewRegistry(claude.New(), codex.New())
	if err != nil {
		return nil, closeOnErr(st, fmt.Errorf("build driver registry: %w", err))
	}

	mgr := session.NewManager(reg, tr, session.Config{ScrollbackDir: layout.Scrollback})
	engine := worktree.NewEngine(gitcli.NewRunner(), layout.Worktrees, time.Now)
	auth := api.NewAuth(token)

	apiHandlers := api.New(api.Config{
		Tree:        tr,
		Sessions:    mgr,
		Store:       st,
		Worktrees:   engine,
		Auth:        auth,
		HookTokens:  api.NewHookTokens(),
		Logger:      logger,
		Version:     version,
		Commit:      commit,
		DaemonURL:   fmt.Sprintf("http://127.0.0.1:%d", port),
		HookCommand: hookCommand(logger),
	})
	wsHandlers := ws.New(ws.Config{
		Tree:          tr,
		Sessions:      mgr,
		Store:         st,
		Logger:        logger,
		ScrollbackDir: layout.Scrollback,
	})
	//nolint:contextcheck // New is a pure constructor; the server owns its own lifetime context.
	return server.New(server.Config{
		Addr:     fmt.Sprintf("127.0.0.1:%d", port),
		Auth:     auth,
		API:      apiHandlers,
		WS:       wsHandlers,
		Sessions: mgr,
		Store:    st,
		Logger:   logger,
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
