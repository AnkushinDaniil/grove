package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/AnkushinDaniil/grove/internal/session"
)

// mcpDialTimeout bounds connecting to the daemon socket before the shim gives up.
const mcpDialTimeout = 5 * time.Second

// runMCP is the stdio<->Unix-socket bridge every spawned CLI mounts as its
// "grove" MCP server (via `--mcp-config` / equivalent). It writes a one-line
// auth preamble carrying the node's identity, then pipes MCP JSON-RPC bytes
// between the CLI on stdio and the daemon on its Unix socket. The daemon runs
// the real MCP server; this shim keeps the node token off the MCP protocol and
// out of any TCP listener.
func runMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	socket := fs.String("socket", os.Getenv(session.EnvSocket), "daemon MCP socket (default $"+session.EnvSocket+")")
	node := fs.String("node", os.Getenv(session.EnvNodeID), "grove node id (default $"+session.EnvNodeID+")")
	token := fs.String("token", os.Getenv(session.EnvNodeToken), "per-node token (default $"+session.EnvNodeToken+")")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse mcp flags: %w", err)
	}
	if *socket == "" || *node == "" || *token == "" {
		return fmt.Errorf("grove mcp requires $%s, $%s and $%s", session.EnvSocket, session.EnvNodeID, session.EnvNodeToken)
	}
	return bridgeMCP(*socket, *node, *token, os.Stdin, os.Stdout)
}

// bridgeMCP dials the daemon socket, sends the auth preamble, and shuttles bytes
// until the daemon closes the connection (end of the MCP session).
func bridgeMCP(socket, node, token string, in io.Reader, out io.Writer) error {
	dialer := net.Dialer{Timeout: mcpDialTimeout}
	conn, err := dialer.Dial("unix", socket)
	if err != nil {
		return fmt.Errorf("dial grove daemon at %s: %w", socket, err)
	}
	defer func() { _ = conn.Close() }()

	preamble, err := json.Marshal(map[string]string{"grove_node": node, "grove_token": token})
	if err != nil {
		return fmt.Errorf("encode auth preamble: %w", err)
	}
	if _, err := conn.Write(append(preamble, '\n')); err != nil {
		return fmt.Errorf("write auth preamble: %w", err)
	}

	// Forward CLI stdin to the daemon; half-close on EOF so the daemon sees the
	// end of client input while we keep draining its responses.
	go func() {
		_, _ = io.Copy(conn, in)
		if uc, ok := conn.(*net.UnixConn); ok {
			_ = uc.CloseWrite()
		}
	}()

	// The session ends when the daemon closes its side.
	if _, err := io.Copy(out, conn); err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("mcp bridge: %w", err)
	}
	return nil
}
