package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"github.com/coder/websocket"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/term"
)

const (
	// termChunk caps one server→client binary frame; replay and live output are
	// split into frames no larger than this.
	termChunk = 32 * 1024
	// termReadLimit bounds a single client→server frame (keystrokes, pastes,
	// resize JSON).
	termReadLimit = 1 << 20
	// initResizeTimeout bounds how long we wait for the client's first (resize)
	// frame before giving up on a live attach.
	initResizeTimeout = 10 * time.Second
	// dropGrace is how long the writer waits for the process-exit signal after
	// the live channel closes, to tell an exit apart from a slow-consumer drop.
	dropGrace = 2 * time.Second
)

// liveMsg is the server→client {"t":"live"} control frame.
type liveMsg struct {
	T string `json:"t"`
}

// exitMsg is the server→client {"t":"exit","code":N} control frame.
type exitMsg struct {
	T    string `json:"t"`
	Code int    `json:"code"`
}

// resizeMsg is the client→server {"t":"resize","cols":C,"rows":R} control frame.
type resizeMsg struct {
	T    string `json:"t"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// serveTerm bridges a terminal session. A live session is attached
// bidirectionally; a finished session with persisted scrollback is replayed and
// closed.
func (h *Handlers) serveTerm(w http.ResponseWriter, r *http.Request) {
	sid := core.SessionID(r.PathValue("id"))
	conn, err := websocket.Accept(w, r, h.acceptOpts)
	if err != nil {
		h.logger.Debug("ws term accept", "err", err)
		return
	}
	conn.SetReadLimit(termReadLimit)
	defer func() { _ = conn.CloseNow() }()

	if handle, live := h.sessions.Terminal(sid); live {
		h.serveTermLive(r.Context(), conn, handle, sid)
		return
	}
	h.serveTermReplay(r.Context(), conn, sid)
}

// ackOnInput clears the session's node attention the moment the user starts
// typing into the terminal — typing IS answering the permission prompt or
// question the attention flagged.
func (h *Handlers) ackOnInput(ctx context.Context, sid core.SessionID) {
	sess, ok := h.tree.SessionByID(sid)
	if !ok {
		return
	}
	node, ok := h.tree.Get(sess.NodeID)
	if !ok || node.Attention == core.AttentionNone {
		return
	}
	if _, err := h.store.AckNodeEvents(ctx, node.ID, time.Now()); err != nil {
		h.logger.Warn("terminal auto-ack events", "node", node.ID, "err", err)
	}
	if _, err := h.tree.Ack(ctx, node.ID); err != nil {
		h.logger.Warn("terminal auto-ack node", "node", node.ID, "err", err)
	}
}

// serveTermLive attaches to a running PTY: it applies the initial resize, sends
// the scrollback replay, signals live, then pumps output to the client and
// input back to the PTY until either side closes or the process exits.
func (h *Handlers) serveTermLive(parent context.Context, conn *websocket.Conn, handle *term.Handle, sid core.SessionID) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	if err := readInitialResize(ctx, conn, handle); err != nil {
		return
	}

	replay, live, cancelSub := handle.Attach()
	defer cancelSub()

	if err := writeBinaryChunks(ctx, conn, replay); err != nil {
		return
	}
	if err := writeControl(ctx, conn, liveMsg{T: "live"}); err != nil {
		return
	}

	acked := false
	onInput := func() {
		if !acked {
			acked = true
			h.ackOnInput(ctx, sid)
		}
	}
	go pumpClient(ctx, cancel, conn, handle, onInput)
	pumpTerminal(ctx, conn, handle, live)
}

// pumpClient forwards client frames to the PTY: binary frames are keystrokes,
// text frames are resize controls. Canceling on return tears down the writer.
// onInput fires on keystroke frames (attention auto-ack).
func pumpClient(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, handle *term.Handle, onInput func()) {
	defer cancel()
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		switch typ {
		case websocket.MessageBinary:
			if len(data) > 0 && onInput != nil {
				onInput()
			}
			if err := handle.Write(data); err != nil {
				return
			}
		case websocket.MessageText:
			applyResize(data, handle)
		}
	}
}

// pumpTerminal forwards live PTY output to the client and sends the exit frame
// when the process ends.
func pumpTerminal(ctx context.Context, conn *websocket.Conn, handle *term.Handle, live <-chan []byte) {
	done := handle.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			finishTerminal(ctx, conn, handle)
			return
		case chunk, ok := <-live:
			if !ok {
				// The subscription ended: either the process exited (finish
				// closes subscribers just before Done) or we lagged and were
				// dropped. Wait briefly for Done to disambiguate.
				if waitClosed(done, dropGrace) {
					finishTerminal(ctx, conn, handle)
				} else {
					_ = conn.Close(websocket.StatusTryAgainLater, "terminal viewer lagged")
				}
				return
			}
			if err := writeBinaryChunks(ctx, conn, chunk); err != nil {
				return
			}
		}
	}
}

// finishTerminal sends the exit control frame with the process exit code and
// closes the socket.
func finishTerminal(ctx context.Context, conn *websocket.Conn, handle *term.Handle) {
	code, _ := handle.ExitState()
	_ = writeControl(ctx, conn, exitMsg{T: "exit", Code: code})
	_ = conn.Close(websocket.StatusNormalClosure, "session ended")
}

// serveTermReplay replays a finished session's scrollback, sends its stored exit
// code, then closes. With no scrollback there is nothing to show, so it closes
// immediately.
func (h *Handlers) serveTermReplay(ctx context.Context, conn *websocket.Conn, sid core.SessionID) {
	data, err := term.LoadScrollback(filepath.Join(h.scrollbackDir, string(sid)+".bin"))
	if err != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "no live session")
		return
	}
	if err := writeBinaryChunks(ctx, conn, data); err != nil {
		return
	}
	_ = writeControl(ctx, conn, exitMsg{T: "exit", Code: h.storedExitCode(sid)})
	_ = conn.Close(websocket.StatusNormalClosure, "replay complete")
}

// storedExitCode returns the exit code recorded for a session, or -1 when it is
// unknown (interrupted, or no longer in the tree snapshot).
func (h *Handlers) storedExitCode(sid core.SessionID) int {
	for _, s := range h.tree.Snapshot().Sessions {
		if s.ID == sid {
			if s.ExitCode != nil {
				return *s.ExitCode
			}
			break
		}
	}
	return -1
}

// readInitialResize reads the client's first frame, applying it when it is a
// resize control. A read error (or timeout) aborts the attach.
func readInitialResize(ctx context.Context, conn *websocket.Conn, handle *term.Handle) error {
	rctx, cancel := context.WithTimeout(ctx, initResizeTimeout)
	defer cancel()
	typ, data, err := conn.Read(rctx)
	if err != nil {
		return err
	}
	if typ == websocket.MessageText {
		applyResize(data, handle)
	}
	return nil
}

// applyResize applies a resize control frame to the PTY, ignoring anything else.
func applyResize(data []byte, handle *term.Handle) {
	var msg resizeMsg
	if err := json.Unmarshal(data, &msg); err != nil || msg.T != "resize" {
		return
	}
	cols, rows := clampDim(msg.Cols), clampDim(msg.Rows)
	if cols == 0 || rows == 0 {
		return
	}
	_ = handle.Resize(cols, rows)
}

// clampDim coerces a terminal dimension into the uint16 PTY range, returning 0
// for non-positive values so the caller can skip a bogus resize.
func clampDim(v int) uint16 {
	if v <= 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint16(v)
}

// writeControl writes a JSON control frame as a text message under the write
// timeout.
func writeControl(ctx context.Context, conn *websocket.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(wctx, websocket.MessageText, data)
}

// writeBinaryChunks writes data as one or more binary frames no larger than
// termChunk, each under the write timeout.
func writeBinaryChunks(ctx context.Context, conn *websocket.Conn, data []byte) error {
	for len(data) > 0 {
		n := min(len(data), termChunk)
		if err := writeBinaryFrame(ctx, conn, data[:n]); err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

// writeBinaryFrame writes one binary frame under the write timeout.
func writeBinaryFrame(ctx context.Context, conn *websocket.Conn, data []byte) error {
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return conn.Write(wctx, websocket.MessageBinary, data)
}

// waitClosed reports whether ch closes within d.
func waitClosed(ch <-chan struct{}, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ch:
		return true
	case <-timer.C:
		return false
	}
}
