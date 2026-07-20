package codex

import (
	"encoding/json"
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/driver"
)

// mcpFlags builds `--config mcp_servers.<name>.*` overrides for each MCP
// ref, plus the env entries needed to supply bearer tokens. Codex reads a
// streamable-HTTP MCP server's bearer token from an environment variable
// named by `bearer_token_env_var` rather than accepting the token value
// inline (confirmed: codex-rs/config/src/mcp_types.rs and mcp_edit.rs
// reject an inline `bearer_token` for streamable HTTP servers and direct
// callers to `bearer_token_env_var`) — unlike Claude's --mcp-config, which
// embeds the token directly in argv.
func mcpFlags(refs []driver.MCPRef) ([]string, []string, error) {
	var flags, env []string
	for _, ref := range refs {
		// A JSON string is valid TOML string syntax, so `key`/`url` double
		// as both a quoted TOML dotted-key segment and a TOML string value
		// — this also safely handles ref.Name values with dots, spaces, or
		// other characters a bare TOML key would reject.
		key, err := json.Marshal(ref.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal mcp server name: %w", err)
		}
		url, err := json.Marshal(ref.URL)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal mcp server url: %w", err)
		}
		flags = append(flags, "--config", fmt.Sprintf("mcp_servers.%s.url=%s", key, url))

		if ref.Token != "" {
			envVar := mcpTokenEnvVar(ref.Name)
			envVarJSON, err := json.Marshal(envVar)
			if err != nil {
				return nil, nil, fmt.Errorf("marshal mcp bearer token env var: %w", err)
			}
			flags = append(flags, "--config", fmt.Sprintf("mcp_servers.%s.bearer_token_env_var=%s", key, envVarJSON))
			env = append(env, envVar+"="+ref.Token)
		}
	}
	return flags, env, nil
}

// mcpTokenEnvVar derives a safe, deterministic environment variable name
// for an MCP ref's bearer token. ref.Name may contain characters that are
// not valid in an env var name, so it is sanitized: non-alphanumeric bytes
// become '_' and letters are upper-cased.
func mcpTokenEnvVar(name string) string {
	b := make([]byte, 0, len(name)+16)
	b = append(b, "GROVE_MCP_"...)
	for i := range len(name) {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
			b = append(b, c-'a'+'A')
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	b = append(b, "_TOKEN"...)
	return string(b)
}
