package session

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/driver"
)

// startHeadless spawns the command with piped stdio, registers the live session
// and starts the event pump. The process runs in its own group so it can be
// signalled as a unit.
func (m *Manager) startHeadless(
	sess core.Session,
	drv driver.Driver,
	spec driver.ExecSpec,
) (core.Session, error) {
	runCtx, cancel := context.WithCancel(m.baseCtx)
	//nolint:gosec // G204: argv is built by a trusted in-process driver, not user input.
	cmd := exec.CommandContext(runCtx, spec.Argv[0], spec.Argv[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = append(m.cfg.BaseEnv(), spec.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return signalGroup(cmd.Process, syscall.SIGKILL) }

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return m.failStart(sess, fmt.Errorf("stdout pipe: %w", err))
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return m.failStart(sess, fmt.Errorf("stdin pipe: %w", err))
	}
	stderr := &tailBuffer{max: stderrTailMax}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return m.failStart(sess, fmt.Errorf("start process: %w", err))
	}

	ls := &liveSession{
		id:     sess.ID,
		nodeID: sess.NodeID,
		mode:   core.ModeHeadless,
		driver: drv,
		caps:   drv.Capabilities(),
		stdin:  stdin,
		proc:   cmd.Process,
		cancel: cancel,
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
	go m.pumpHeadless(ls, cmd, stdout, stderr)
	return running, nil
}

// pumpHeadless reads the process's JSONL stdout line by line, feeds each line
// to the driver parser, applies the resulting events, then reaps the process.
func (m *Manager) pumpHeadless(ls *liveSession, cmd *exec.Cmd, stdout io.Reader, stderr *tailBuffer) {
	defer m.wg.Done()
	ctx := m.baseCtx
	parser := ls.driver.NewParser()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), headlessMaxLine)
	for scanner.Scan() {
		inputs, err := parser.Feed(scanner.Bytes())
		if err != nil {
			continue // skip a malformed line; keep pumping
		}
		m.handleInputs(ctx, ls, inputs)
	}
	if tail, err := parser.Close(); err == nil {
		m.handleInputs(ctx, ls, tail)
	}

	code, infraErr := exitInfo(cmd.Wait())
	ls.cancel()
	m.reportStderr(ctx, ls, stderr, code, infraErr)
	m.finalize(ls, code, infraErr)
}

// reportStderr surfaces the tail of a failed process's stderr as an EventError,
// so a headless failure carries diagnostic context. A clean exit reports nothing.
func (m *Manager) reportStderr(ctx context.Context, ls *liveSession, stderr *tailBuffer, code int, infraErr error) {
	if infraErr == nil && code == 0 {
		return
	}
	tail := strings.TrimSpace(stderr.String())
	if tail == "" {
		return
	}
	payload, err := core.MarshalPayload(core.ErrorPayload{Message: tail, Fatal: infraErr != nil})
	if err != nil {
		return
	}
	_, _ = m.tree.IngestEvents(ctx, ls.nodeID, ls.id, []core.EventInput{{
		Type:    core.EventError,
		Payload: payload,
		Detail:  truncate(tail, 200),
	}})
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

// handleInputs threads driver events into the tree: SessionStarted updates the
// session identity, and every event is ingested against the node.
func (m *Manager) handleInputs(ctx context.Context, ls *liveSession, inputs []core.EventInput) {
	if len(inputs) == 0 {
		return
	}
	for _, in := range inputs {
		if in.Type == core.EventSessionStarted {
			m.applySessionStarted(ctx, ls, in.Payload)
		}
	}
	// Best effort: ingest failure only loses history/attention for this batch.
	_, _ = m.tree.IngestEvents(ctx, ls.nodeID, ls.id, inputs)
}

// applySessionStarted records the driver's own session id and transcript path
// reported in the output stream.
func (m *Manager) applySessionStarted(ctx context.Context, ls *liveSession, payload string) {
	p, err := core.UnmarshalPayload[core.SessionStartedPayload](payload)
	if err != nil {
		return
	}
	_ = m.updateSession(ctx, ls, func(s *core.Session) {
		s.DriverSessionID = p.DriverSessionID
		if p.TranscriptPath != "" {
			s.TranscriptPath = p.TranscriptPath
		}
		if core.CanTransition(s.Status, core.SessionRunning) {
			s.Status = core.SessionRunning
		}
	})
}

// promptHeadless writes a driver-formatted follow-up prompt to the process's
// stdin, if the driver streams and the session is still alive.
func (m *Manager) promptHeadless(ls *liveSession, text string) error {
	if !ls.caps.HeadlessStream {
		return fmt.Errorf("%w: driver %s has no headless stream", ErrUnsupportedPrompt, ls.driver.ID())
	}
	data, err := ls.driver.FormatPrompt(text)
	if err != nil {
		return fmt.Errorf("format prompt: %w", err)
	}
	ls.mu.Lock()
	stdin := ls.stdin
	alive := !ls.sess.Status.Terminal()
	ls.mu.Unlock()
	if stdin == nil || !alive {
		return fmt.Errorf("%w: session not accepting input", ErrUnsupportedPrompt)
	}
	if _, err := stdin.Write(data); err != nil {
		return fmt.Errorf("write stdin: %w", err)
	}
	return nil
}

// stopHeadless signals the process group with SIGTERM, escalating to SIGKILL
// after stopGrace or when ctx is done. The pump goroutine records the final
// status via the exit path.
func (m *Manager) stopHeadless(ctx context.Context, ls *liveSession) {
	_ = signalGroup(ls.proc, syscall.SIGTERM)
	timer := time.NewTimer(stopGrace)
	defer timer.Stop()
	select {
	case <-ls.done:
		return
	case <-ctx.Done():
	case <-timer.C:
	}
	_ = signalGroup(ls.proc, syscall.SIGKILL)
}

// signalGroup sends sig to the process group led by proc. A gone process
// (ESRCH) is treated as success.
func signalGroup(proc *os.Process, sig syscall.Signal) error {
	if proc == nil {
		return nil
	}
	if err := syscall.Kill(-proc.Pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("signal group %d: %w", proc.Pid, err)
	}
	return nil
}

// exitInfo normalizes a cmd.Wait result: exit code for a normal exit, 128+signum
// for a signalled one, and a wrapped error only for genuine wait failures.
func exitInfo(err error) (code int, infraErr error) {
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() {
				return 128 + int(ws.Signal()), nil
			}
			return ws.ExitStatus(), nil
		}
		return ee.ExitCode(), nil
	}
	return -1, fmt.Errorf("wait process: %w", err)
}

// tailBuffer captures the last max bytes written to it, for surfacing the tail
// of a failed process's stderr. It is safe for concurrent writes.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
