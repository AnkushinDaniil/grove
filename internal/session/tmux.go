package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/term"
	"github.com/AnkushinDaniil/grove/internal/tmux"
)

// maxReattach bounds how many times watchPTYTmux re-attaches after a transient
// detach before giving up, so a tmux session that keeps dropping its client
// cannot spin the watch loop.
const maxReattach = 3

// startPTYTmux hosts an interactive session inside tmux so its child survives a
// daemon restart. The child runs in a detached tmux session; grove attaches a
// PTY client and streams through that, exactly as it would a direct child.
func (m *Manager) startPTYTmux(
	sess core.Session,
	drv driver.Driver,
	spec driver.ExecSpec,
) (core.Session, error) {
	name := tmux.SessionName(string(sess.ID))
	exitFile := m.exitFilePath(sess.ID)
	env := append(m.cfg.BaseEnv(), spec.Env...)
	if err := m.tmux.NewSession(m.baseCtx, tmux.NewSpec{
		Name:     name,
		Argv:     spec.Argv,
		Env:      env,
		Dir:      spec.Dir,
		Cols:     ptyCols,
		Rows:     ptyRows,
		ExitFile: exitFile,
	}); err != nil {
		return m.failStart(sess, fmt.Errorf("start tmux session: %w", err))
	}

	handle, err := m.attachTmux(sess.ID, name)
	if err != nil {
		_ = m.tmux.KillSession(m.baseCtx, name)
		return m.failStart(sess, fmt.Errorf("attach tmux session: %w", err))
	}

	ls := &liveSession{
		id:       sess.ID,
		nodeID:   sess.NodeID,
		mode:     core.ModePTY,
		driver:   drv,
		caps:     drv.Capabilities(),
		handle:   handle,
		tmuxName: name,
		exitFile: exitFile,
		sess:     sess,
		done:     make(chan struct{}),
	}
	m.addLive(ls)

	running := sess
	running.Status = core.SessionRunning
	if _, err := m.tree.ApplySession(m.baseCtx, running); err != nil {
		return core.Session{}, fmt.Errorf("apply running session: %w", err)
	}
	ls.mu.Lock()
	ls.sess = running
	ls.mu.Unlock()

	m.wg.Add(1)
	go m.watchPTYTmux(ls)
	return running, nil
}

// attachTmux spawns a PTY client attached to a tmux session and returns the
// handle grove streams, writes and resizes through. The attach client's own
// death never kills the tmux child, which is what lets the child outlive the
// daemon.
func (m *Manager) attachTmux(id core.SessionID, name string) (*term.Handle, error) {
	argv := tmux.AttachCommand(name)
	//nolint:gosec // G204: argv is tmux attach-session with a grove-controlled session name.
	cmd := exec.CommandContext(m.baseCtx, argv[0], argv[1:]...)
	cmd.Env = m.cfg.BaseEnv()

	var opts []term.Option
	if path := ScrollbackFile(m.cfg.ScrollbackDir, id); path != "" {
		opts = append(opts, term.WithScrollbackPath(path))
	}
	return term.Start(cmd, ptyCols, ptyRows, opts...)
}

// watchPTYTmux supervises a tmux-hosted PTY session. When the attach client
// exits it distinguishes three cases by the tmux session's existence:
//
//   - the daemon is detaching (Shutdown): leave the child running for Reattach;
//   - the tmux session is gone: the child exited, so read its real exit code
//     from the wrapper's exit file and finalize as usual;
//   - the tmux session is still alive: a transient detach, so re-attach a fresh
//     client and keep streaming (bounded by maxReattach).
func (m *Manager) watchPTYTmux(ls *liveSession) {
	defer m.wg.Done()
	reattaches := 0
	for {
		h := ls.currentHandle()
		<-h.Done()
		code, exitErr := h.ExitState()

		if m.detaching.Load() {
			// Graceful daemon shutdown: the child stays alive in tmux.
			return
		}

		alive, err := m.tmux.HasSession(m.baseCtx, ls.tmuxName)
		if err == nil && alive && reattaches < maxReattach {
			reattaches++
			if next, aerr := m.attachTmux(ls.id, ls.tmuxName); aerr == nil {
				ls.setHandle(next)
				continue
			}
			// Re-attach failed: fall through and finalize the session.
		}

		finalCode := code
		if c, ok := tmux.ReadExitCode(ls.exitFile); ok {
			finalCode = c
		}
		_ = m.tmux.KillSession(m.baseCtx, ls.tmuxName)
		_ = os.Remove(ls.exitFile)
		m.finalize(ls, finalCode, exitErr)
		return
	}
}

// stopPTYTmux stops a tmux-hosted PTY session on explicit request: killing the
// tmux session SIGHUPs the child, and the watch loop then reads the exit path
// and finalizes. The attach client is stopped too so its handle unblocks
// promptly.
func (m *Manager) stopPTYTmux(ctx context.Context, ls *liveSession) error {
	if err := m.tmux.KillSession(ctx, ls.tmuxName); err != nil {
		return fmt.Errorf("stop tmux session: %w", err)
	}
	_ = ls.currentHandle().Stop(ctx)
	return nil
}

// Reattach revives interactive sessions whose tmux-hosted child survived a
// daemon restart. For each surviving grove tmux session that maps to a live PTY
// node on disk it re-attaches a client and flips the session (and its node)
// back to running; grove tmux sessions with no matching live PTY node are
// killed as orphans. It is a no-op when tmux hosting is disabled. Reattach must
// run after the tree is loaded and after MarkInterrupted (which flips surviving
// sessions to interrupted first). Returns how many sessions were revived.
func (m *Manager) Reattach(ctx context.Context) (int, error) {
	if !m.usePTYTmux() {
		return 0, nil
	}
	names, err := m.tmux.ListGroveSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("list grove tmux sessions: %w", err)
	}
	reattached := 0
	for _, name := range names {
		if m.reviveOrKill(ctx, name) {
			reattached++
		}
	}
	return reattached, nil
}

// reviveOrKill re-attaches one surviving tmux session when it maps to a live
// PTY node on disk, or kills it as an orphan otherwise. It reports whether a
// session was revived.
func (m *Manager) reviveOrKill(ctx context.Context, name string) bool {
	idStr, ok := tmux.SessionID(name)
	if !ok {
		return false
	}
	sess, ok := m.tree.SessionByID(core.SessionID(idStr))
	if !ok || sess.Mode != core.ModePTY {
		_ = m.tmux.KillSession(ctx, name)
		return false
	}
	node, ok := m.tree.Get(sess.NodeID)
	if !ok || node.Archived() {
		_ = m.tmux.KillSession(ctx, name)
		return false
	}
	return m.reattachOne(ctx, sess, name) == nil
}

// reattachOne attaches a client to a surviving tmux session, registers it as
// live and flips the session and its node back to running.
func (m *Manager) reattachOne(ctx context.Context, sess core.Session, name string) error {
	drv, ok := m.reg.Get(sess.Driver)
	if !ok {
		return fmt.Errorf("%w: driver %q not registered", ErrNoDriver, sess.Driver)
	}
	handle, err := m.attachTmux(sess.ID, name)
	if err != nil {
		return fmt.Errorf("attach tmux session: %w", err)
	}
	ls := &liveSession{
		id:       sess.ID,
		nodeID:   sess.NodeID,
		mode:     core.ModePTY,
		driver:   drv,
		caps:     drv.Capabilities(),
		handle:   handle,
		tmuxName: name,
		exitFile: m.exitFilePath(sess.ID),
		sess:     sess,
		done:     make(chan struct{}),
	}
	m.addLive(ls)

	revived := sess
	revived.Status = core.SessionRunning
	revived.EndedAt = time.Time{}
	if _, err := m.tree.ApplySession(ctx, revived); err != nil {
		m.removeLive(sess.ID)
		_ = handle.Stop(ctx)
		return fmt.Errorf("apply revived session: %w", err)
	}
	ls.mu.Lock()
	ls.sess = revived
	ls.mu.Unlock()

	m.wg.Add(1)
	go m.watchPTYTmux(ls)
	return nil
}
