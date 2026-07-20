package term

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func startCmd(t *testing.T, cmd *exec.Cmd, opts ...Option) *Handle {
	t.Helper()
	h, err := Start(cmd, 80, 24, opts...)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = h.Stop(ctx)
	})
	return h
}

func waitDone(t *testing.T, h *Handle, within time.Duration) {
	t.Helper()
	select {
	case <-h.Done():
	case <-time.After(within):
		t.Fatalf("process did not exit within %s", within)
	}
}

// TestHandleAttachNoGapNoDup verifies the attach boundary: a viewer that
// attaches mid-stream sees the replay snapshot immediately followed by the live
// feed with no missing or duplicated bytes. The sniffer captures the whole
// output stream as ground truth.
func TestHandleAttachNoGapNoDup(t *testing.T) {
	var mu sync.Mutex
	var ground []byte
	h := startCmd(t, exec.Command("/bin/cat"), WithSniffer(func(b []byte) {
		mu.Lock()
		ground = append(ground, b...)
		mu.Unlock()
	}))

	go func() {
		for i := range 20 {
			_ = h.Write(fmt.Appendf(nil, "line-%02d\n", i))
			time.Sleep(3 * time.Millisecond)
		}
		_ = h.Write([]byte{0x04}) // Ctrl-D: EOF to cat
	}()

	time.Sleep(25 * time.Millisecond) // let some output land in the ring
	replay, ch, cancel := h.Attach()
	defer cancel()

	var live []byte
	for chunk := range ch {
		live = append(live, chunk...)
	}
	waitDone(t, h, 3*time.Second)

	mu.Lock()
	g := bytes.Clone(ground)
	mu.Unlock()

	if len(replay) == 0 || len(live) == 0 {
		t.Fatalf("want both replay (%d) and live (%d) non-empty", len(replay), len(live))
	}
	if !bytes.HasPrefix(g, replay) {
		t.Fatalf("replay is not a prefix of the ground-truth stream")
	}
	if !bytes.HasPrefix(g[len(replay):], live) {
		t.Fatalf("live feed does not continue exactly after replay (gap or duplicate)")
	}
}

func TestHandleEchoVisible(t *testing.T) {
	h := startCmd(t, exec.Command("/bin/cat"))
	if err := h.Write([]byte("ping\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	replay, ch, cancel := h.Attach()
	defer cancel()
	if bytes.Contains(replay, []byte("ping")) {
		return
	}
	deadline := time.After(3 * time.Second)
	var seen []byte
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed before seeing echo; saw %q", seen)
			}
			seen = append(seen, chunk...)
			if bytes.Contains(seen, []byte("ping")) {
				return
			}
		case <-deadline:
			t.Fatalf("did not observe echo of 'ping'; saw %q", seen)
		}
	}
}

func TestHandleResize(t *testing.T) {
	h := startCmd(t, exec.Command("/bin/cat"))
	if err := h.Resize(100, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if err := h.Resize(132, 50); err != nil {
		t.Fatalf("second Resize: %v", err)
	}
}

func TestHandleStopTerminates(t *testing.T) {
	h := startCmd(t, exec.Command("/bin/sh", "-c", "sleep 30"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitDone(t, h, time.Second)
	code, err := h.ExitState()
	if err != nil {
		t.Fatalf("ExitState err = %v", err)
	}
	if code < 128 {
		t.Fatalf("exit code = %d, want a signalled exit (>=128)", code)
	}
}

func TestHandleCleanExit(t *testing.T) {
	h := startCmd(t, exec.Command("/bin/sh", "-c", "exit 0"))
	waitDone(t, h, 3*time.Second)
	code, err := h.ExitState()
	if err != nil || code != 0 {
		t.Fatalf("ExitState = (%d, %v), want (0, nil)", code, err)
	}
}

func TestHandleNonZeroExit(t *testing.T) {
	h := startCmd(t, exec.Command("/bin/sh", "-c", "exit 7"))
	waitDone(t, h, 3*time.Second)
	code, err := h.ExitState()
	if err != nil || code != 7 {
		t.Fatalf("ExitState = (%d, %v), want (7, nil)", code, err)
	}
}

func TestHandleScrollbackRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scroll", "session.bin")
	h := startCmd(t, exec.Command("/bin/sh", "-c", `printf 'alpha\nbeta\ngamma\n'`),
		WithScrollbackPath(path))
	waitDone(t, h, 3*time.Second)

	loaded, err := LoadScrollback(path)
	if err != nil {
		t.Fatalf("LoadScrollback: %v", err)
	}
	if len(loaded) == 0 {
		t.Fatal("scrollback file is empty")
	}
	if !bytes.Equal(loaded, h.ring.Bytes()) {
		t.Fatalf("scrollback %q != ring %q", loaded, h.ring.Bytes())
	}
	if !bytes.Contains(loaded, []byte("alpha")) {
		t.Fatalf("scrollback missing output: %q", loaded)
	}
}

// TestHandleSlowSubscriberDropped floods a subscriber that stops reading and
// asserts its channel is eventually closed (dropped) rather than blocking the
// drain loop.
func TestHandleSlowSubscriberDropped(t *testing.T) {
	// An unbounded flood makes the overflow deterministic regardless of host
	// speed or PTY read coalescing: the drain loop keeps producing chunks
	// while we deliberately do not read, so the bounded channel must fill and
	// the next fan-out write must drop us. (The previous timed shell script
	// was flaky on slow CI runners.)
	h := startCmd(t, exec.Command("yes", "grove-flood"))

	_, ch, cancel := h.Attach()
	defer cancel()

	deadline := time.After(15 * time.Second)
	for {
		if len(ch) == cap(ch) {
			break // buffer saturated while the producer still floods
		}
		select {
		case <-deadline:
			t.Fatalf("subscriber buffer never filled: len=%d cap=%d", len(ch), cap(ch))
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Drain what is buffered and expect closure from the overflow drop.
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // dropped as designed
			}
		case <-deadline:
			t.Fatal("slow subscriber was not dropped")
		}
	}
}

func TestHandleAttachAfterExit(t *testing.T) {
	h := startCmd(t, exec.Command("/bin/sh", "-c", `printf 'done\n'`))
	waitDone(t, h, 3*time.Second)

	replay, ch, cancel := h.Attach()
	defer cancel()
	if !bytes.Contains(replay, []byte("done")) {
		t.Fatalf("replay after exit = %q, want to contain 'done'", replay)
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected an already-closed channel after exit")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed for a post-exit attach")
	}
}
