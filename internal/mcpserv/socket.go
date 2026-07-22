package mcpserv

import "path/filepath"

// SocketName is the daemon's MCP Unix socket filename under GROVE_HOME.
const SocketName = "daemon.sock"

// SocketPath returns the MCP socket path for a resolved grove home directory.
func SocketPath(home string) string { return filepath.Join(home, SocketName) }
