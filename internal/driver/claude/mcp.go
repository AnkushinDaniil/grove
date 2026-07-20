package claude

import (
	"encoding/json"
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

// mcpServer is one entry in Claude Code's --mcp-config JSON.
type mcpServer struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// mcpConfig is the full --mcp-config document.
type mcpConfig struct {
	MCPServers map[string]mcpServer `json:"mcpServers"`
}

// marshalMCPConfig builds the inline --mcp-config JSON for the given
// servers. Headers are omitted for refs with no token.
func marshalMCPConfig(refs []driver.MCPRef) (string, error) {
	servers := make(map[string]mcpServer, len(refs))
	for _, ref := range refs {
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
