// Package session runs and supervises CLI agent processes bound to tree nodes.
// It spawns each run in PTY or headless mode, feeds normalized events and
// status transitions back through the tree, and exposes attach/prompt/stop
// control. The tree is the single writer of state; this package is its single
// producer of session snapshots.
package session

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/term"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

const (
	defaultMaxRunning = 12
	ptyCols           = 120
	ptyRows           = 32
	stopGrace         = 5 * time.Second
	stderrTailMax     = 8 * 1024
	// headlessMaxLine bounds a single JSONL event line from a headless agent.
	headlessMaxLine = 10 * 1024 * 1024
)

// Config configures a Manager.
type Config struct {
	// ScrollbackDir is where PTY scrollback files are written. Empty disables
	// scrollback persistence.
	ScrollbackDir string
	// MaxRunning caps concurrent live sessions. Zero means defaultMaxRunning.
	MaxRunning int
	// BaseEnv builds the base environment for each spawned process. Nil means
	// DefaultBaseEnv.
	BaseEnv func() []string

	// HookCommand is the "<grove> hook" invocation embedded in generated agent
	// hook settings so native-hook drivers can phone events home. Empty disables
	// hook wiring, preserving the unwired launch path.
	HookCommand string
	// DaemonURL is the daemon's own base URL (http://127.0.0.1:<port>) embedded in
	// the generated hook wiring. Empty disables hook wiring.
	DaemonURL string
	// MintHookToken returns the per-node hook token embedded in the generated hook
	// wiring, minted idempotently per node. Nil disables hook wiring.
	MintHookToken func(nodeID core.NodeID) string
}

// LaunchOption mutates the driver LaunchSpec before the command is built. It is
// the seam for wiring hooks, MCP and profile isolation, which are injected by
// higher layers later.
type LaunchOption func(*driver.LaunchSpec)

// Manager owns the set of live sessions and their supervising goroutines.
type Manager struct {
	reg  *driver.Registry
	tree *tree.Tree
	cfg  Config

	// baseCtx bounds every spawned process and its supervising goroutines to
	// the manager's own lifetime, not to any single request. Shutdown cancels
	// it as a hard-stop backstop.
	baseCtx    context.Context
	baseCancel context.CancelFunc

	mu   sync.Mutex
	live map[core.SessionID]*liveSession
	wg   sync.WaitGroup
}

// liveSession is the runtime state for one running session. sess is the latest
// snapshot and is guarded by its own mutex so event pumps and control calls do
// not contend on the manager lock.
type liveSession struct {
	id     core.SessionID
	nodeID core.NodeID
	mode   core.SessionMode
	driver driver.Driver
	caps   driver.Caps

	handle *term.Handle // PTY mode
	stdin  io.WriteCloser
	proc   *os.Process
	cancel context.CancelFunc

	mu   sync.Mutex
	sess core.Session
	done chan struct{}
}

// NewManager builds a Manager over a driver registry and tree.
func NewManager(reg *driver.Registry, tr *tree.Tree, cfg Config) *Manager {
	if cfg.MaxRunning <= 0 {
		cfg.MaxRunning = defaultMaxRunning
	}
	if cfg.BaseEnv == nil {
		cfg.BaseEnv = DefaultBaseEnv
	}
	//nolint:gosec // G118: baseCancel is retained on the Manager and invoked by Shutdown.
	baseCtx, baseCancel := context.WithCancel(context.Background())
	return &Manager{
		reg:        reg,
		tree:       tr,
		cfg:        cfg,
		baseCtx:    baseCtx,
		baseCancel: baseCancel,
		live:       make(map[core.SessionID]*liveSession),
	}
}

// Start launches a new session for nodeID in the given mode. It resolves the
// node's driver, enforces the live-session budget, materializes the command's
// files and spawns the process, returning the running session snapshot.
func (m *Manager) Start(
	ctx context.Context,
	nodeID core.NodeID,
	mode core.SessionMode,
	prompt, resumeID string,
	opts ...LaunchOption,
) (core.Session, error) {
	if !mode.Valid() {
		return core.Session{}, fmt.Errorf("%w: unknown session mode %q", core.ErrInvalid, mode)
	}
	node, ok := m.tree.Get(nodeID)
	if !ok {
		return core.Session{}, fmt.Errorf("%w: node %s not found", core.ErrInvalid, nodeID)
	}
	if node.Archived() {
		return core.Session{}, fmt.Errorf("%w: node %s is archived", core.ErrInvalid, nodeID)
	}
	resolved, ok := m.tree.Resolve(nodeID)
	if !ok || resolved.Driver == "" {
		return core.Session{}, fmt.Errorf("%w: %s", ErrNoDriver, nodeID)
	}
	drv, ok := m.reg.Get(resolved.Driver)
	if !ok {
		return core.Session{}, fmt.Errorf("%w: driver %q not registered", ErrNoDriver, resolved.Driver)
	}

	m.mu.Lock()
	if len(m.live) >= m.cfg.MaxRunning {
		n := len(m.live)
		m.mu.Unlock()
		return core.Session{}, fmt.Errorf("%w: %d/%d live", ErrBudgetExhausted, n, m.cfg.MaxRunning)
	}
	m.mu.Unlock()

	cwd, err := m.workingDir(node)
	if err != nil {
		return core.Session{}, err
	}

	spec := driver.LaunchSpec{Mode: mode, Prompt: prompt, ResumeID: resumeID, CWD: cwd}
	for _, opt := range opts {
		opt(&spec)
	}
	if spec.Hooks == nil {
		spec.Hooks = m.hookWiring(drv.Capabilities(), nodeID)
	}
	execSpec, err := drv.NewCommand(spec)
	if err != nil {
		return core.Session{}, fmt.Errorf("build command: %w", err)
	}
	if len(execSpec.Argv) == 0 {
		return core.Session{}, fmt.Errorf("%w: driver %q produced empty argv", core.ErrInvalid, drv.ID())
	}
	if err := materializeFiles(execSpec); err != nil {
		return core.Session{}, err
	}

	sess := core.Session{
		ID:        core.NewSessionID(),
		NodeID:    nodeID,
		Driver:    resolved.Driver,
		ProfileID: resolved.ProfileID,
		Mode:      mode,
		Status:    core.SessionStarting,
		CWD:       cwd,
		StartedAt: time.Now(),
	}
	if _, err := m.tree.ApplySession(ctx, sess); err != nil {
		return core.Session{}, fmt.Errorf("apply starting session: %w", err)
	}

	// Past this point the run is bound to the manager's lifetime, not this
	// request: it must keep running (and record its state) even if ctx is
	// canceled, so the spawn helpers use m.baseCtx internally.
	switch mode {
	case core.ModePTY:
		return m.startPTY(sess, drv, execSpec)
	case core.ModeHeadless:
		//nolint:contextcheck // the spawned run is bound to the manager lifetime (m.baseCtx), deliberately not the request ctx.
		return m.startHeadless(sess, drv, execSpec)
	default:
		return core.Session{}, fmt.Errorf("%w: unknown session mode %q", core.ErrInvalid, mode)
	}
}

// hookWiring builds native-hook wiring for nodeID when the driver supports hooks
// and the manager was configured with the command, daemon URL and token minter
// needed to phone events home. It returns nil to leave the launch unwired,
// preserving the no-hooks path for any driver or configuration missing a piece.
func (m *Manager) hookWiring(caps driver.Caps, nodeID core.NodeID) *driver.HookWiring {
	if !caps.NativeHooks || m.cfg.HookCommand == "" || m.cfg.DaemonURL == "" || m.cfg.MintHookToken == nil {
		return nil
	}
	token := m.cfg.MintHookToken(nodeID)
	if token == "" {
		return nil
	}
	return &driver.HookWiring{
		HookCommand: m.cfg.HookCommand,
		DaemonURL:   m.cfg.DaemonURL,
		NodeID:      nodeID,
		Token:       token,
	}
}

// Stop terminates a live session and waits for it to be finalized.
func (m *Manager) Stop(ctx context.Context, sid core.SessionID) error {
	ls, ok := m.get(sid)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sid)
	}
	switch ls.mode {
	case core.ModePTY:
		if err := ls.handle.Stop(ctx); err != nil {
			return fmt.Errorf("stop pty session: %w", err)
		}
	case core.ModeHeadless:
		m.stopHeadless(ctx, ls)
	default:
		return fmt.Errorf("%w: unknown session mode %q", core.ErrInvalid, ls.mode)
	}
	select {
	case <-ls.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Prompt delivers a follow-up prompt to a live session. PTY sessions receive it
// as a bracketed-paste keystroke sequence; headless streaming sessions receive
// the driver-formatted bytes on stdin.
func (m *Manager) Prompt(_ context.Context, sid core.SessionID, text string) error {
	ls, ok := m.get(sid)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sid)
	}
	switch ls.mode {
	case core.ModePTY:
		seq := "\x1b[200~" + text + "\x1b[201~\r"
		if err := ls.handle.Write([]byte(seq)); err != nil {
			return fmt.Errorf("write prompt: %w", err)
		}
		return nil
	case core.ModeHeadless:
		return m.promptHeadless(ls, text)
	default:
		return fmt.Errorf("%w: unknown session mode %q", core.ErrInvalid, ls.mode)
	}
}

// Terminal returns the PTY handle for a live session so the ws layer can attach,
// resize and write to it. It reports false for headless or unknown sessions.
func (m *Manager) Terminal(sid core.SessionID) (*term.Handle, bool) {
	ls, ok := m.get(sid)
	if !ok || ls.handle == nil {
		return nil, false
	}
	return ls.handle, true
}

// Shutdown stops every live session and waits for all supervising goroutines to
// finish, or until ctx is done.
func (m *Manager) Shutdown(ctx context.Context) error {
	// Releases baseCtx on return, hard-killing any process that outlived the
	// graceful stop below (exec.CommandContext watches baseCtx).
	defer m.baseCancel()

	m.mu.Lock()
	ids := make([]core.SessionID, 0, len(m.live))
	for id := range m.live {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		_ = m.Stop(ctx, id)
	}

	waited := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(waited)
	}()
	select {
	case <-waited:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) get(sid core.SessionID) (*liveSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ls, ok := m.live[sid]
	return ls, ok
}

func (m *Manager) addLive(ls *liveSession) {
	m.mu.Lock()
	m.live[ls.id] = ls
	m.mu.Unlock()
}

func (m *Manager) removeLive(id core.SessionID) {
	m.mu.Lock()
	delete(m.live, id)
	m.mu.Unlock()
}

// finalize records a session's terminal status once its process has exited and
// removes it from the live set. An infrastructure error maps to SessionFailed;
// otherwise the session exits with code (node status derives done/failed).
// finalize runs exactly once per session, from its supervising goroutine.
func (m *Manager) finalize(ls *liveSession, code int, infraErr error) {
	defer close(ls.done)
	defer m.removeLive(ls.id)

	ls.mu.Lock()
	final := ls.sess
	ls.mu.Unlock()

	final.EndedAt = time.Now()
	switch {
	case infraErr != nil && core.CanTransition(final.Status, core.SessionFailed):
		final.Status = core.SessionFailed
	case infraErr == nil && core.CanTransition(final.Status, core.SessionExited):
		final.Status = core.SessionExited
		c := code
		final.ExitCode = &c
	}

	// The CLI's exit farewell carries the authoritative conversation id;
	// prefer it over anything hooks captured (see ExtractResumeID).
	if final.Mode == core.ModePTY {
		if id := resumeIDFromScrollback(m.cfg.ScrollbackDir, final.ID); id != "" {
			final.DriverSessionID = id
		}
	}

	// Best effort: the process is already gone, so a persistence failure here
	// has no recovery path beyond the store's own error surfacing.
	_, _ = m.tree.ApplySession(m.baseCtx, final)

	ls.mu.Lock()
	ls.sess = final
	ls.mu.Unlock()
}

// failStart marks a session failed when spawning could not complete.
func (m *Manager) failStart(sess core.Session, cause error) (core.Session, error) {
	sess.Status = core.SessionFailed
	sess.EndedAt = time.Now()
	if _, err := m.tree.ApplySession(m.baseCtx, sess); err != nil {
		return core.Session{}, fmt.Errorf("%w (persisting failed status also failed: %w)", cause, err)
	}
	return core.Session{}, cause
}

// updateSession applies mutate to the live session's snapshot under lock and
// persists the result through the tree.
func (m *Manager) updateSession(ctx context.Context, ls *liveSession, mutate func(*core.Session)) error {
	ls.mu.Lock()
	next := ls.sess
	mutate(&next)
	ls.mu.Unlock()
	if _, err := m.tree.ApplySession(ctx, next); err != nil {
		return fmt.Errorf("apply session: %w", err)
	}
	ls.mu.Lock()
	ls.sess = next
	ls.mu.Unlock()
	return nil
}

// workingDir picks the process working directory, in priority order: the node's
// machine-managed worktree workspace, the effective user-set working directory
// inherited down the tree, then the user's home as a fallback.
func (m *Manager) workingDir(node core.Node) (string, error) {
	if node.WorkspaceDir != "" {
		return node.WorkspaceDir, nil
	}
	if dir, ok := m.tree.ResolveWorkDir(node.ID); ok && dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return home, nil
}

// materializeFiles writes the driver-generated files (settings, notify configs)
// before the process is spawned. Relative paths are joined to the command dir.
func materializeFiles(spec driver.ExecSpec) error {
	for p, content := range spec.Files {
		path := p
		if !filepath.IsAbs(path) {
			path = filepath.Join(spec.Dir, p)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("create dir for %s: %w", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}
