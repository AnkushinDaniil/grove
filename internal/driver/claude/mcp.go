package claude

import (
	"encoding/json"
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

// mcpServer is one entry in Claude Code's --mcp-config JSON. It carries either
// a stdio server (Type "stdio": Command/Args/Env) or an HTTP server (Type
// "http": URL/Headers); the unused transport's fields stay empty and omitted.
type mcpServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// mcpConfig is the full --mcp-config document.
type mcpConfig struct {
	MCPServers map[string]mcpServer `json:"mcpServers"`
}

// marshalMCPConfig builds the inline --mcp-config JSON for the given servers.
// A ref with a Command renders as a stdio server (grove's own `grove mcp`
// shim); otherwise it renders as an HTTP server, with headers omitted when the
// ref carries no token.
func marshalMCPConfig(refs []driver.MCPRef) (string, error) {
	servers := make(map[string]mcpServer, len(refs))
	for _, ref := range refs {
		if ref.Command != "" {
			servers[ref.Name] = mcpServer{
				Type:    "stdio",
				Command: ref.Command,
				Args:    ref.Args,
				Env:     ref.Env,
			}
			continue
		}
		srv := mcpServer{Type: "http", URL: ref.URL}
		if ref.Token != "" {
			srv.Headers = map[string]string{"Authorization": "Bearer " + ref.Token}
		}
		servers[ref.Name] = srv
	}
	b, err := json.Marshal(mcpConfig{MCPServers: servers})
	if err != nil {
		return "", fmt.Errorf("marshal mcp config: %w", err)
	}
	return string(b), nil
}
