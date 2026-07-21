package memory

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"time"
)

// mcpProtocolVersion is the MCP revision grove advertises in the initialize
// handshake. Servers negotiate their own; we only require initialize to succeed
// and tools/list to return. https://modelcontextprotocol.io/specification
const mcpProtocolVersion = "2025-06-18"

// ProbeReport is the outcome of an MCP health probe.
type ProbeReport struct {
	OK         bool     // handshake + tools/list succeeded with at least one tool
	ServerName string   // serverInfo.name from initialize, if provided
	Protocol   string   // negotiated protocolVersion, if provided
	ToolCount  int      // number of tools advertised by tools/list
	ToolNames  []string // a sample of tool names, for logging
	RoundTrip  bool     // whether a scratch write/read roundtrip was performed
	Note       string   // human-readable summary of what the probe did
}

// Probe health-checks the MCP server: it spawns `mempalace mcp`, performs the
// JSON-RPC initialize handshake, lists tools, and asserts a sane tool count.
// The child is always reaped and the whole exchange is bounded by a 10s timeout.
//
// A store/read tool pair exists (mempalace_add_drawer + mempalace_search), but a
// real write would land in the user's live palace — MemPalace has no dedicated
// scratch room in its API — so the roundtrip is intentionally skipped to avoid
// polluting real memory. Handshake + tools/list is the PASS criterion.
func (e Env) Probe(ctx context.Context) (ProbeReport, error) {
	return e.probe(ctx, probeTimeout)
}

func (e Env) probe(ctx context.Context, timeout time.Duration) (ProbeReport, error) {
	// MemPalace ≥3.6 ships a dedicated MCP server binary (`mempalace-mcp`);
	// older builds exposed it as the `mcp` subcommand of the main CLI. Prefer
	// the dedicated binary — spawning the CLI without it just EOFs.
	var bin string
	var args []string
	if p, err := e.lookPath(MCPBinaryName); err == nil {
		bin = p
	} else if p, err := e.lookPath(BinaryName); err == nil {
		bin, args = p, []string{"mcp"}
	} else {
		return ProbeReport{}, fmt.Errorf("cannot probe: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	//nolint:gosec // G204: PATH-resolved mempalace binary, fixed args.
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = e.childEnv()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return ProbeReport{}, fmt.Errorf("mcp stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ProbeReport{}, fmt.Errorf("mcp stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return ProbeReport{}, fmt.Errorf("start mempalace mcp: %w", err)
	}

	rep, herr := handshake(ctx, stdin, bufio.NewReader(stdout))

	// Reap the child (close stdin, kill, wait) only after the handshake has
	// finished reading stdout, and read stderr only after Wait returns — that
	// ordering avoids racing exec's stdout read and stderr-copier goroutines.
	_ = stdin.Close()
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	if herr != nil {
		return rep, fmt.Errorf("%w%s", herr, stderrHint(&stderr))
	}
	return rep, nil
}

// handshake performs initialize -> initialized -> tools/list over the child's
// stdio pipes and classifies the result. Errors are stage-labeled; the caller
// adds any stderr context after reaping the process.
func handshake(ctx context.Context, w io.Writer, r *bufio.Reader) (ProbeReport, error) {
	conn := &stdioConn{w: w, r: r}
	rep := ProbeReport{}

	initRes, err := conn.call(ctx, "initialize", initParams())
	if err != nil {
		return rep, fmt.Errorf("initialize: %w", err)
	}
	rep.ServerName, rep.Protocol = parseServerInfo(initRes)

	if err := conn.notify("notifications/initialized", map[string]any{}); err != nil {
		return rep, fmt.Errorf("initialized notification: %w", err)
	}

	listRes, err := conn.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return rep, fmt.Errorf("tools/list: %w", err)
	}
	names, err := parseToolNames(listRes)
	if err != nil {
		return rep, fmt.Errorf("parse tools/list: %w", err)
	}
	rep.ToolCount = len(names)
	rep.ToolNames = sample(names, 6)

	if rep.ToolCount == 0 {
		rep.Note = "MCP server returned zero tools"
		return rep, nil
	}
	rep.OK = true
	if hasTool(names, "mempalace_add_drawer") && hasTool(names, "mempalace_search") {
		rep.Note = "write/read tools present; scratch roundtrip skipped to avoid writing into the live palace"
	} else {
		rep.Note = "handshake + tools/list succeeded"
	}
	return rep, nil
}

// stdioConn is a minimal newline-delimited JSON-RPC 2.0 client over the child's
// stdio pipes — the MCP stdio transport frames one JSON object per line.
type stdioConn struct {
	w  io.Writer
	r  *bufio.Reader
	id int
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// call sends a request and returns the result for the matching id, skipping any
// interleaved notifications or unrelated messages.
func (c *stdioConn) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.id++
	id := c.id
	if err := c.write(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, err := c.r.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 {
			var resp rpcResponse
			if json.Unmarshal(trimmed, &resp) == nil && resp.ID == id {
				if resp.Error != nil {
					return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
				}
				return resp.Result, nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("read response for %s: %w", method, err)
		}
	}
}

// notify sends a notification (a request without an id, expecting no response).
func (c *stdioConn) notify(method string, params any) error {
	return c.write(rpcRequest{JSONRPC: "2.0", Method: method, Params: params})
}

func (c *stdioConn) write(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal rpc: %w", err)
	}
	if _, err := c.w.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write rpc: %w", err)
	}
	return nil
}

func initParams() map[string]any {
	return map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "grove", "version": "dev"},
	}
}

func parseServerInfo(raw json.RawMessage) (name, protocol string) {
	var r struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	_ = json.Unmarshal(raw, &r)
	return r.ServerInfo.Name, r.ProtocolVersion
}

func parseToolNames(raw json.RawMessage) ([]string, error) {
	var r struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(r.Tools))
	for _, t := range r.Tools {
		names = append(names, t.Name)
	}
	return names, nil
}

func hasTool(names []string, want string) bool {
	return slices.Contains(names, want)
}

func sample(names []string, n int) []string {
	if len(names) <= n {
		return names
	}
	return names[:n]
}

// stderrHint appends a bounded slice of the child's stderr to an error message.
func stderrHint(buf *bytes.Buffer) string {
	s := bytes.TrimSpace(buf.Bytes())
	if len(s) == 0 {
		return ""
	}
	const max = 300
	if len(s) > max {
		s = s[:max]
	}
	return fmt.Sprintf(" (stderr: %s)", s)
}
