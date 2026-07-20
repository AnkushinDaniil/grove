package term

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

const (
	// subChanCap bounds a subscriber's live-chunk queue. A viewer that falls
	// this far behind is dropped (its channel closed) and must re-attach.
	subChanCap = 64
	// readBufSize is the PTY read chunk size.
	readBufSize = 32 * 1024
	// stopGrace is the delay between SIGTERM and SIGKILL in Stop.
	stopGrace = 5 * time.Second
)

// subscription is one attached viewer's live feed.
type subscription struct {
	ch chan []byte
}

// Handle owns one live PTY process: the drain goroutine reads its output into a
// scrollback ring and fans it out to attached subscribers.
type Handle struct {
	cmd            *exec.Cmd
	pty            *os.File
	ring           *Ring
	ringSize       int
	sniffer        func([]byte)
	scrollbackPath string

	mu      sync.Mutex
	subs    map[int]*subscription
	nextSub int
	closed  bool

	flushMu     sync.Mutex
	lastFlushed int64

	done     chan struct{}
	exitCode int
	exitErr  error
}

// Option configures a Handle at Start.
type Option func(*Handle)

// WithRingSize sets the scrollback ring capacity in bytes.
func WithRingSize(n int) Option { return func(h *Handle) { h.ringSize = n } }

// WithScrollbackPath persists the ring to path (periodic + on exit) so output
// survives a daemon restart.
func WithScrollbackPath(path string) Option {
	return func(h *Handle) { h.scrollbackPath = path }
}

// WithSniffer registers a callback invoked with every output chunk in order,
// for attention detection on the raw stream.
func WithSniffer(fn func([]byte)) Option { return func(h *Handle) { h.sniffer = fn } }

// Start launches cmd on a new PTY sized cols×rows and begins draining its
// output. pty.StartWithSize puts the child in its own session with the PTY as
// controlling terminal, so its process group can be signalled as a unit.
func Start(cmd *exec.Cmd, cols, rows uint16, opts ...Option) (*Handle, error) {
	h := &Handle{
		cmd:      cmd,
		ringSize: DefaultRingSize,
		subs:     make(map[int]*subscription),
		done:     make(chan struct{}),
	}
	for _, opt := range opts {
		opt(h)
	}
	h.ring = NewRing(h.ringSize)
	if h.scrollbackPath != "" {
		if err := os.MkdirAll(filepath.Dir(h.scrollbackPath), 0o700); err != nil {
			return nil, fmt.Errorf("create scrollback dir: %w", err)
		}
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}
	h.pty = f
	go h.drain()
	if h.scrollbackPath != "" {
		go h.flushLoop()
	}
	return h, nil
}

// drain reads the PTY until it closes, then reaps the process.
func (h *Handle) drain() {
	defer h.finish()
	buf := make([]byte, readBufSize)
	for {
		n, err := h.pty.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			h.ingest(chunk)
		}
		if err != nil {
			return
		}
	}
}

// ingest stores a chunk and fans it out. The ring write and the fan-out happen
// under one lock, and Attach snapshots the ring under the same lock, so a newly
// attached subscriber sees every byte exactly once with no gap or duplicate at
// the replay/live boundary.
func (h *Handle) ingest(chunk []byte) {
	h.mu.Lock()
	h.ring.Write(chunk)
	for id, sub := range h.subs {
		select {
		case sub.ch <- chunk:
		default:
			delete(h.subs, id)
			close(sub.ch)
		}
	}
	h.mu.Unlock()
	if h.sniffer != nil {
		h.sniffer(chunk)
	}
}

// Attach returns a snapshot of the scrollback so far plus a channel delivering
// every subsequent chunk. cancel releases the subscription. If the process has
// already exited, replay holds the full output and ch is already closed.
func (h *Handle) Attach() (replay []byte, ch <-chan []byte, cancel func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	replay = h.ring.Bytes()
	if h.closed {
		closed := make(chan []byte)
		close(closed)
		return replay, closed, func() {}
	}
	id := h.nextSub
	h.nextSub++
	sub := &subscription{ch: make(chan []byte, subChanCap)}
	h.subs[id] = sub
	cancel = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if s, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(s.ch)
		}
	}
	return replay, sub.ch, cancel
}

// Write sends keystrokes to the PTY (stdin of the child).
func (h *Handle) Write(p []byte) error {
	if _, err := h.pty.Write(p); err != nil {
		return fmt.Errorf("write pty: %w", err)
	}
	return nil
}

// Resize changes the PTY window size. It issues the ioctl through the file's
// SyscallConn so it coordinates with a concurrent Close (from process exit)
// instead of racing on the raw descriptor.
func (h *Handle) Resize(cols, rows uint16) error {
	conn, err := h.pty.SyscallConn()
	if err != nil {
		return fmt.Errorf("pty syscall conn: %w", err)
	}
	ws := &unix.Winsize{Row: rows, Col: cols}
	var ioctlErr error
	if err := conn.Control(func(fd uintptr) {
		//nolint:gosec // G115: a file descriptor always fits in int.
		ioctlErr = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, ws)
	}); err != nil {
		return fmt.Errorf("resize pty control: %w", err)
	}
	if ioctlErr != nil {
		return fmt.Errorf("resize pty: %w", ioctlErr)
	}
	return nil
}

// Done is closed once the process has exited and ExitState is valid.
func (h *Handle) Done() <-chan struct{} { return h.done }

// ExitState returns the process exit code and any infrastructure error. It is
// only valid after Done is closed. A signalled process reports 128+signum.
func (h *Handle) ExitState() (code int, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.exitCode, h.exitErr
}

// Stop terminates the process group with SIGTERM, escalating to SIGKILL after
// stopGrace or when ctx is done. It returns once the process has exited.
func (h *Handle) Stop(ctx context.Context) error {
	if err := h.signalGroup(syscall.SIGTERM); err != nil {
		return err
	}
	timer := time.NewTimer(stopGrace)
	defer timer.Stop()
	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
	case <-timer.C:
	}
	if err := h.signalGroup(syscall.SIGKILL); err != nil {
		return err
	}
	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// signalGroup sends sig to the child's process group. A gone process (ESRCH) is
// treated as success.
func (h *Handle) signalGroup(sig syscall.Signal) error {
	if h.cmd.Process == nil {
		return nil
	}
	pid := h.cmd.Process.Pid
	if err := syscall.Kill(-pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("signal pty group %d: %w", pid, err)
	}
	return nil
}

// finish reaps the process, records its exit state, closes all subscribers and
// does the final scrollback flush. It runs once, after drain returns.
func (h *Handle) finish() {
	code, exitErr := reap(h.cmd)
	_ = h.pty.Close()
	h.mu.Lock()
	h.exitCode = code
	h.exitErr = exitErr
	h.closed = true
	for id, sub := range h.subs {
		delete(h.subs, id)
		close(sub.ch)
	}
	h.mu.Unlock()
	if h.scrollbackPath != "" {
		h.flush()
	}
	close(h.done)
}

// reap waits for the process and normalizes its result: exit code for a normal
// exit, 128+signum for a signalled one, and a wrapped error only for genuine
// wait failures.
func reap(cmd *exec.Cmd) (int, error) {
	err := cmd.Wait()
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
	return -1, fmt.Errorf("wait pty process: %w", err)
}
