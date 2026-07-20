package codex

import (
	"slices"
	"testing"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

func TestMCPFlags(t *testing.T) {
	refs := []driver.MCPRef{
		{Name: "ctx7", URL: "https://mcp.example/ctx7", Token: "mcp-tok"},
		{Name: "grove", URL: "https://mcp.example/grove"},
	}
	flags, env, err := mcpFlags(refs)
	if err != nil {
		t.Fatalf("mcpFlags() error = %v", err)
	}

	want := []string{
		"--config", `mcp_servers."ctx7".url="https://mcp.example/ctx7"`,
		"--config", `mcp_servers."ctx7".bearer_token_env_var="GROVE_MCP_CTX7_TOKEN"`,
		"--config", `mcp_servers."grove".url="https://mcp.example/grove"`,
	}
	if !slices.Equal(flags, want) {
		t.Errorf("mcpFlags() flags = %v, want %v", flags, want)
	}
	if !slices.Equal(env, []string{"GROVE_MCP_CTX7_TOKEN=mcp-tok"}) {
		t.Errorf("mcpFlags() env = %v, want [GROVE_MCP_CTX7_TOKEN=mcp-tok]", env)
	}
}

func TestMCPFlagsNoRefs(t *testing.T) {
	flags, env, err := mcpFlags(nil)
	if err != nil {
		t.Fatalf("mcpFlags() error = %v", err)
	}
	if len(flags) != 0 || len(env) != 0 {
		t.Errorf("mcpFlags(nil) = %v, %v, want empty", flags, env)
	}
}

func TestMCPTokenEnvVar(t *testing.T) {
	tests := []struct{ name, want string }{
		{"ctx7", "GROVE_MCP_CTX7_TOKEN"},
		{"my-server.v2", "GROVE_MCP_MY_SERVER_V2_TOKEN"},
		{"already UPPER_case", "GROVE_MCP_ALREADY_UPPER_CASE_TOKEN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mcpTokenEnvVar(tt.name); got != tt.want {
				t.Errorf("mcpTokenEnvVar(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
