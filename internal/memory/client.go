package memory

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// This file turns the phase-1 health probe (probe.go) into a reusable MemPalace
// MCP client: the daemon acts as an MCP client, calling tools/call for semantic
// search and drawer writes so grove gets recall injection and auto-capture
// (docs/ORCHESTRATION.md §8, phase 2). Every call is a short-lived stdio
// exchange — spawn `mempalace-mcp`, handshake, one tools/call, reap — mirroring
// the probe's process handling; the query cost dwarfs the spawn cost, and a
// stateless per-call model needs no long-lived subprocess supervision.
//
// Tool mapping (pinned against MemPalace 3.6.0 via a live tools/list + tools/call
// probe, 2026-07-22):
//   - write  -> mempalace_add_drawer{wing, room, content, source_file, added_by}
//   - search -> mempalace_search{query, wing, room, limit, max_distance}
//     result: content[0].text is JSON {results: [{text, wing, room, source_path,
//     created_at, ...}]}.

const (
	// defaultCallTimeout bounds a read/write tools/call. Generous because a busy
	// palace can take tens of seconds; auto-capture runs off the caller's path so
	// the latency is invisible, and a timeout simply spools the write.
	defaultCallTimeout = 45 * time.Second

	// defaultRecallTimeout bounds recall injection. Short and strict: recall runs
	// synchronously while composing a spawn/wake briefing and must never delay an
	// agent launch. A slow palace degrades to no injected memory, not a stall.
	defaultRecallTimeout = 4 * time.Second

	// recallLimit and endpointLimit cap how many drawers a search returns before
	// scope post-filtering.
	recallLimit   = 8
	endpointLimit = 100
)

// backendName is the memory backend grove reports to the API/UI. It matches the
// serverInfo.name MemPalace returns on initialize.
const backendName = "mempalace"

// mcpCommand resolves how to launch the MemPalace MCP server. MemPalace ≥3.6
// ships a dedicated binary (`mempalace-mcp`); older builds exposed it as the
// `mcp` subcommand of the main CLI. Prefer the dedicated binary — spawning the
// CLI without it just EOFs. Shared by the health probe (probe.go) and the
// client's tools/call exchanges.
func (e Env) mcpCommand() (bin string, args []string, err error) {
	if p, perr := e.lookPath(MCPBinaryName); perr == nil {
		return p, nil, nil
	}
	if p, perr := e.lookPath(BinaryName); perr == nil {
		return p, []string{"mcp"}, nil
	}
	return "", nil, fmt.Errorf("%q (or %q) not found on PATH", MCPBinaryName, BinaryName)
}

// Client is the daemon's MemPalace MCP client. It is safe for concurrent use;
// spool replay is serialized internally. A nil *Client is a valid no-op backend
// (every method degrades to "unavailable"), so callers need no nil checks.
type Client struct {
	env      Env
	tree     Tree
	spool    string // spool file for writes made while the backend is down
	log      *slog.Logger
	now      func() time.Time
	callTO   time.Duration
	recallTO time.Duration

	replayMu sync.Mutex // serializes spool drain so two writers don't double-replay
}

// Options configures NewClient. Tree and SpoolPath are the only ones that matter
// in production; the rest have sane defaults and exist for tests.
type Options struct {
	Tree          Tree
	SpoolPath     string // where writes spool when MemPalace is unavailable
	Env           Env    // PATH/Home resolution; zero value targets the real env
	Logger        *slog.Logger
	Now           func() time.Time
	CallTimeout   time.Duration
	RecallTimeout time.Duration
}

// NewClient builds a Client. It does not touch MemPalace or the filesystem;
// resolution and health are evaluated lazily per call.
func NewClient(o Options) *Client {
	log := o.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	callTO := o.CallTimeout
	if callTO <= 0 {
		callTO = defaultCallTimeout
	}
	recallTO := o.RecallTimeout
	if recallTO <= 0 {
		recallTO = defaultRecallTimeout
	}
	return &Client{
		env:      o.Env,
		tree:     o.Tree,
		spool:    o.SpoolPath,
		log:      log,
		now:      now,
		callTO:   callTO,
		recallTO: recallTO,
	}
}

// drawerWrite is the resolved payload for one mempalace_add_drawer call. It is
// also the on-disk spool record, so a write buffered while the backend is down
// replays verbatim later.
type drawerWrite struct {
	Wing       string `json:"wing"`
	Room       string `json:"room"`
	Content    string `json:"content"`
	SourceFile string `json:"source_file"`
	AddedBy    string `json:"added_by"`
}

// Entry is one memory item surfaced to grove: a MemPalace drawer mapped into
// grove's vocabulary. CreatedAt is passed through verbatim as MemPalace's ISO
// string (it may lack a timezone) so grove never reinterprets the palace clock.
type Entry struct {
	ID        string
	Kind      string // fact|decision|gotcha|convention
	Content   string
	Source    string // auto|agent|user
	CreatedAt string
	Wing      string
	Room      string
}

// available reports whether a MemPalace binary resolves. It is the cheap
// precondition every operation checks first so a missing install degrades
// instantly instead of paying a spawn.
func (c *Client) available() bool {
	if c == nil {
		return false
	}
	_, _, err := c.env.mcpCommand()
	return err == nil
}

// Search runs a semantic query within a resolved scope and returns the matching
// entries, newest-relevance first, plus whether the backend answered. It never
// returns an error to the caller: an unavailable or slow backend yields
// (nil, false) so recall and the REST endpoint degrade to "no memory" instead of
// failing (ORCHESTRATION.md §8: reads degrade, never error).
func (c *Client) Search(ctx context.Context, query string, filter scopeFilter, limit int) ([]Entry, bool) {
	if !c.available() || !filter.valid() || strings.TrimSpace(query) == "" {
		return nil, false
	}
	args := map[string]any{
		"query": query,
		"wing":  filter.wing,
		"limit": limit,
		// Distance filtering is disabled (0): scope is enforced by wing + room
		// membership, and on a BM25-fallback palace cosine distance is absent
		// anyway. We want everything in scope, ranked, not distance-pruned.
		"max_distance": 0,
	}
	raw, err := c.callTool(ctx, "mempalace_search", args)
	if err != nil {
		c.log.Debug("memory search failed", "err", err)
		return nil, false
	}
	entries := parseSearchEntries(raw)
	out := entries[:0]
	for _, e := range entries {
		if filter.allows(e.Room) {
			out = append(out, e)
		}
	}
	return out, true
}

// Write files one drawer into MemPalace. On any failure (backend down, spawn
// error, timeout) it spools the write to replay later and returns nil — a
// capture must never surface an error to the agent path. A live write first
// drains any spooled backlog so recovery is automatic.
func (c *Client) Write(ctx context.Context, w drawerWrite) error {
	if c == nil {
		return nil
	}
	if !c.available() {
		return c.spoolWrite(w)
	}
	c.drainSpool(ctx)
	if err := c.writeOnce(ctx, w); err != nil {
		c.log.Debug("memory write failed; spooling", "err", err)
		return c.spoolWrite(w)
	}
	return nil
}

// writeOnce issues a single mempalace_add_drawer call.
func (c *Client) writeOnce(ctx context.Context, w drawerWrite) error {
	args := map[string]any{
		"wing":    w.Wing,
		"room":    w.Room,
		"content": w.Content,
	}
	if w.SourceFile != "" {
		args["source_file"] = w.SourceFile
	}
	if w.AddedBy != "" {
		args["added_by"] = w.AddedBy
	}
	_, err := c.callTool(ctx, "mempalace_add_drawer", args)
	return err
}

// callTool spawns the MCP server, performs the initialize/initialized handshake,
// issues one tools/call, reaps the child, and returns the tool's text payload
// (the content[0].text body, which MemPalace fills with a JSON document). The
// whole exchange is bounded by c.callTO.
func (c *Client) callTool(ctx context.Context, name string, args map[string]any) (json.RawMessage, error) {
	bin, cmdArgs, err := c.env.mcpCommand()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.callTO)
	defer cancel()

	//nolint:gosec // G204: PATH-resolved mempalace binary, fixed args.
	cmd := exec.CommandContext(ctx, bin, cmdArgs...)
	cmd.Env = c.env.childEnv()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mempalace mcp: %w", err)
	}

	payload, cerr := exchange(ctx, stdin, bufio.NewReader(stdout), name, args)

	// Reap in the same order as probe(): finish reading stdout before killing.
	_ = stdin.Close()
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	if cerr != nil {
		return nil, fmt.Errorf("%w%s", cerr, stderrHint(&stderr))
	}
	return payload, nil
}

// exchange performs the handshake then one tools/call over the child's pipes,
// and returns the extracted text payload of the tool result.
func exchange(ctx context.Context, w io.Writer, r *bufio.Reader, name string, args map[string]any) (json.RawMessage, error) {
	conn := &stdioConn{w: w, r: r}
	if _, err := conn.call(ctx, "initialize", initParams()); err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	if err := conn.notify("notifications/initialized", map[string]any{}); err != nil {
		return nil, fmt.Errorf("initialized notification: %w", err)
	}
	res, err := conn.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", name, err)
	}
	return toolText(res)
}

// toolCallResult is the MCP tools/call envelope: a content array of typed parts
// plus an error flag. MemPalace returns its payload as a single text part whose
// body is a JSON document.
type toolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// toolText pulls the first text part out of a tools/call result. An isError
// result is surfaced as an error carrying the text (which holds MemPalace's
// message).
func toolText(raw json.RawMessage) (json.RawMessage, error) {
	var res toolCallResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("decode tool result: %w", err)
	}
	var text string
	for _, part := range res.Content {
		if part.Type == "text" {
			text = part.Text
			break
		}
	}
	if res.IsError {
		return nil, fmt.Errorf("tool error: %s", strings.TrimSpace(text))
	}
	// MemPalace signals tool-level failures inside the payload
	// ({"success":false,"error":"…"} or {"error":"…"}), not via the MCP isError
	// flag — so a rejected write would otherwise look like success. Surface those
	// as errors so writes spool and searches degrade instead of silently passing.
	if err := payloadError(text); err != nil {
		return nil, err
	}
	return json.RawMessage(text), nil
}

// payloadError reports a MemPalace tool-level failure carried in the JSON
// payload. Non-JSON or success payloads return nil.
func payloadError(text string) error {
	var status struct {
		Error   string `json:"error"`
		Success *bool  `json:"success"`
	}
	if json.Unmarshal([]byte(text), &status) != nil {
		return nil //nolint:nilerr // a non-JSON payload carries no tool-level error to surface.
	}
	if strings.TrimSpace(status.Error) != "" {
		return fmt.Errorf("mempalace: %s", strings.TrimSpace(status.Error))
	}
	if status.Success != nil && !*status.Success {
		return fmt.Errorf("mempalace reported failure")
	}
	return nil
}

// searchPayload is the JSON body MemPalace returns from mempalace_search.
type searchPayload struct {
	Results []struct {
		Text       string `json:"text"`
		Wing       string `json:"wing"`
		Room       string `json:"room"`
		SourcePath string `json:"source_path"`
		CreatedAt  string `json:"created_at"`
	} `json:"results"`
}

// parseSearchEntries decodes a search payload into entries, splitting grove's
// kind/source header off each drawer's content. A malformed payload yields no
// entries rather than an error — search is best-effort.
func parseSearchEntries(raw json.RawMessage) []Entry {
	var p searchPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}
	out := make([]Entry, 0, len(p.Results))
	for _, r := range p.Results {
		kind, source, content := splitHeader(r.Text)
		out = append(out, Entry{
			ID:        entryID(r.SourcePath, r.Room, r.CreatedAt, content),
			Kind:      kind,
			Content:   content,
			Source:    source,
			CreatedAt: strings.TrimSpace(r.CreatedAt),
			Wing:      r.Wing,
			Room:      r.Room,
		})
	}
	return out
}
