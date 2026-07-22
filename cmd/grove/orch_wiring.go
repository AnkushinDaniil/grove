package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/AnkushinDaniil/grove/internal/config"
	"github.com/AnkushinDaniil/grove/internal/mcpserv"
	"github.com/AnkushinDaniil/grove/internal/orch"
	"github.com/AnkushinDaniil/grove/internal/session"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// startOrchestration wires grove's orchestration runtime into the daemon: the
// MCP control plane every spawned agent talks to over the 0600 Unix socket, and
// the event-wake scheduler that keeps orchestrators asleep until their children
// report. Both background loops are bound to ctx and stop on daemon shutdown,
// which also unlinks the socket.
//
// It returns the per-node token registry so the API can mount
// session.WithOrchestration when launching the first (root) orchestrator; every
// child that orchestrator spawns then inherits the mount from the scheduler, so
// the whole subtree becomes self-driving.
func startOrchestration(
	ctx context.Context,
	logger *slog.Logger,
	layout config.Layout,
	tr *tree.Tree,
	mgr *session.Manager,
) (*orch.Scheduler, error) {
	groveBin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable for mcp mount: %w", err)
	}
	socketPath := mcpserv.SocketPath(layout.Home)
	tokens := mcpserv.NewRegistry()

	scheduler := orch.New(orch.Deps{
		Tree:       tr,
		Starter:    mgr,
		Tokens:     tokens,
		SocketPath: socketPath,
		GroveBin:   groveBin,
		Logger:     logger,
	})
	srv := mcpserv.New(mcpserv.Deps{
		Tree:    tr,
		Tokens:  tokens,
		Spawner: scheduler,
		Logger:  logger,
		Version: version,
	})

	ln, err := mcpserv.Listen(ctx, socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on mcp socket: %w", err)
	}
	go func() {
		if serr := srv.Serve(ctx, ln); serr != nil {
			logger.Error("mcp server stopped", "err", serr)
		}
	}()
	go func() {
		if serr := scheduler.Run(ctx); serr != nil {
			logger.Error("orchestration scheduler stopped", "err", serr)
		}
	}()
	logger.Info("orchestration runtime started", "socket", socketPath)
	return scheduler, nil
}
