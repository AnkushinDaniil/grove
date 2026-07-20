package session

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
	"github.com/AnkushinDaniil/grove/internal/term"
)

// startPTY spawns the command on a PTY, registers the live session and starts
// its supervising goroutine. PTY output is streamed to attached viewers, not
// parsed; attention comes from the driver's hooks (see ApplyHook).
func (m *Manager) startPTY(
	sess core.Session,
	drv driver.Driver,
	spec driver.ExecSpec,
) (core.Session, error) {
	//nolint:gosec // G204: argv is built by a trusted in-process driver, not user input.
	cmd := exec.CommandContext(m.baseCtx, spec.Argv[0], spec.Argv[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = append(m.cfg.BaseEnv(), spec.Env...)

	var opts []term.Option
	if m.cfg.ScrollbackDir != "" {
		path := filepath.Join(m.cfg.ScrollbackDir, string(sess.ID)+".bin")
		opts = append(opts, term.WithScrollbackPath(path))
	}
	handle, err := term.Start(cmd, ptyCols, ptyRows, opts...)
	if err != nil {
		return m.failStart(sess, fmt.Errorf("start pty: %w", err))
	}

	ls := &liveSession{
		id:     sess.ID,
		nodeID: sess.NodeID,
		mode:   core.ModePTY,
		driver: drv,
		caps:   drv.Capabilities(),
		handle: handle,
		sess:   sess,
		done:   make(chan struct{}),
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
	go m.watchPTY(ls)
	return running, nil
}

// watchPTY waits for the PTY process to exit and finalizes the session.
func (m *Manager) watchPTY(ls *liveSession) {
	defer m.wg.Done()
	<-ls.handle.Done()
	code, exitErr := ls.handle.ExitState()
	m.finalize(ls, code, exitErr)
}
