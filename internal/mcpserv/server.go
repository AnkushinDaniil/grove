// Package mcpserv is grove's daemon-side MCP server: the tree-of-agents control
// plane every spawned CLI talks to. It listens on the daemon's Unix socket,
// authenticates each connection to a node by its per-node token (identity is
// implicit — tools never take a "self" parameter), and exposes the grove tool
// catalog. Worker nodes report progress, raise attention and complete;
// orchestrator nodes additionally spawn, list and inspect their subtree. Side
// effects on the tree happen here; node creation, session launch and event-wake
// are delegated to a Spawner (package orch).
//
// The transport is newline-delimited JSON-RPC 2.0 (the same framing MCP's stdio
// servers use), carried over the 0600 Unix socket. Each spawned CLI mounts a
// thin `grove mcp` stdio shim that prefixes a one-line auth frame and then pipes
// bytes to the socket, so the token never rides the MCP protocol itself.
package mcpserv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// Deps are the collaborators a Server needs. Tree, Tokens and Spawner are
// required; the rest have sane defaults.
type Deps struct {
	Tree    *tree.Tree
	Tokens  *Registry
	Spawner Spawner
	Limits  Limits
	Logger  *slog.Logger
	Version string
	Now     func() time.Time
}

// Server serves the grove MCP protocol over a Unix socket. It is safe for
// concurrent connections; per-connection state is confined to its goroutine.
type Server struct {
	tree    *tree.Tree
	tokens  *Registry
	spawner Spawner
	limits  Limits
	log     *slog.Logger
	version string
	now     func() time.Time

	pollMu sync.Mutex
	polls  map[core.NodeID][]time.Time
}

// New builds a Server from its dependencies.
func New(d Deps) *Server {
	if d.Logger == nil {
		d.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Limits == (Limits{}) {
		d.Limits = DefaultLimits()
	}
	return &Server{
		tree:    d.Tree,
		tokens:  d.Tokens,
		spawner: d.Spawner,
		limits:  d.Limits,
		log:     d.Logger,
		version: d.Version,
		now:     d.Now,
		polls:   make(map[core.NodeID][]time.Time),
	}
}

// Listen creates the daemon's MCP Unix socket at path with 0600 permissions,
// replacing any stale socket left by a previous run.
func Listen(ctx context.Context, path string) (net.Listener, error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale socket %s: %w", path, err)
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("chmod socket %s: %w", path, err)
	}
	return ln, nil
}

// Serve accepts connections on ln until ctx is canceled, serving each in its own
// goroutine. It closes ln on ctx cancellation and waits for in-flight
// connections to drain before returning.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	var wg sync.WaitGroup
	// Unblock Accept and tear down live connections when ctx is canceled.
	context.AfterFunc(ctx, func() { _ = ln.Close() })

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				// Accept failed because ctx canceled and we closed the listener:
				// a clean shutdown, not an error to propagate.
				wg.Wait()
				return nil //nolint:nilerr // ctx-canceled shutdown, see comment
			}
			// Transient accept errors (e.g. EMFILE) shouldn't kill the server;
			// back off briefly and retry.
			s.log.Warn("mcp accept", "err", err)
			select {
			case <-ctx.Done():
				wg.Wait()
				return nil
			case <-time.After(50 * time.Millisecond):
				continue
			}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.handleConn(ctx, conn)
		}()
	}
}

// handleConn authenticates one connection from its opening auth frame, then
// serves JSON-RPC requests until EOF or ctx cancellation.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()
	// Close the connection when the server shuts down so a blocked read returns.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	lc := newLineConn(conn)
	sess, err := s.authenticate(lc)
	if err != nil {
		s.log.Warn("mcp auth rejected", "err", err)
		return
	}
	s.log.Debug("mcp connection authenticated", "node", sess.node, "role", sess.role)

	for {
		line, err := lc.readLine()
		if err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				s.log.Debug("mcp read", "node", sess.node, "err", err)
			}
			return
		}
		if err := s.dispatch(ctx, lc, sess, line); err != nil {
			s.log.Debug("mcp dispatch", "node", sess.node, "err", err)
			return
		}
	}
}

// authFrame is the one-line preamble the `grove mcp` shim writes before any MCP
// traffic, binding the connection to a node without putting the token on the
// MCP protocol.
type authFrame struct {
	Node  core.NodeID `json:"grove_node"`
	Token string      `json:"grove_token"`
}

// connSession is the authenticated identity of one connection.
type connSession struct {
	node  core.NodeID
	role  Role
	token string
}

// authenticate reads and validates the opening auth frame.
func (s *Server) authenticate(lc *lineConn) (connSession, error) {
	line, err := lc.readLine()
	if err != nil {
		return connSession{}, fmt.Errorf("read auth frame: %w", err)
	}
	var af authFrame
	if err := json.Unmarshal(line, &af); err != nil {
		return connSession{}, fmt.Errorf("parse auth frame: %w", err)
	}
	if af.Node == "" || af.Token == "" {
		return connSession{}, errors.New("auth frame missing node or token")
	}
	role, ok := s.tokens.Resolve(af.Node, af.Token)
	if !ok {
		return connSession{}, fmt.Errorf("token rejected for node %s", af.Node)
	}
	return connSession{node: af.Node, role: role, token: af.Token}, nil
}

// dispatch handles one framed request. It returns a non-nil error only on a
// fatal connection condition (write failure); protocol-level failures are
// reported to the client as JSON-RPC error responses.
func (s *Server) dispatch(ctx context.Context, lc *lineConn, sess connSession, line []byte) error {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return lc.writeMessage(rpcResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage("null"),
			Error:   newRPCError(codeParseError, "parse error"),
		})
	}

	result, rerr := s.route(ctx, sess, req)
	if req.isNotification() {
		return nil // notifications never get a response, even on error
	}
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	if rerr != nil {
		resp.Error = rerr
	} else {
		resp.Result = result
	}
	return lc.writeMessage(resp)
}

// route maps a method to its handler. Protocol failures come back as *rpcError.
func (s *Server) route(ctx context.Context, sess connSession, req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.initializeResult(), nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolCatalog(sess.role)}, nil
	case "tools/call":
		return s.callTool(ctx, sess, req.Params)
	default:
		if req.isNotification() {
			return nil, nil // ignore notifications/initialized and friends
		}
		return nil, newRPCError(codeMethodNotFound, "method not found: "+req.Method)
	}
}

// initializeResult advertises the protocol version, tool capability and the
// durable protocol instructions (briefing layer 1, survives compaction).
func (s *Server) initializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "grove", "version": s.version},
		"instructions":    protocolInstructions,
	}
}
