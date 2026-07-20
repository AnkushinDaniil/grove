package ws

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/AnkushinDaniil/grove/internal/api"
	"github.com/AnkushinDaniil/grove/internal/tree"
)

// writeTimeout bounds a single server→client frame write; a viewer that cannot
// keep up within it is dropped rather than blocking the broadcast.
const writeTimeout = 5 * time.Second

// stateHello is the first /ws/state frame: the full snapshot plus the inbox.
type stateHello struct {
	T        string           `json:"t"`
	Rev      uint64           `json:"rev"`
	Nodes    []api.NodeDTO    `json:"nodes"`
	Sessions []api.SessionDTO `json:"sessions"`
	Inbox    []api.EventDTO   `json:"inbox"`
}

// stateDelta is a subsequent /ws/state frame; empty arrays are omitted.
type stateDelta struct {
	T        string           `json:"t"`
	Rev      uint64           `json:"rev"`
	Nodes    []api.NodeDTO    `json:"nodes,omitempty"`
	Sessions []api.SessionDTO `json:"sessions,omitempty"`
	Events   []api.EventDTO   `json:"events,omitempty"`
}

// serveState streams the tree: a hello snapshot on connect, then one delta per
// tree revision. A dropped subscription (slow consumer) closes the socket so the
// client reconnects and re-syncs from a fresh hello.
func (h *Handlers) serveState(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, h.acceptOpts)
	if err != nil {
		h.logger.Debug("ws state accept", "err", err)
		return
	}
	defer func() { _ = conn.CloseNow() }()

	snap, deltas, cancel := h.tree.Subscribe()
	defer cancel()

	// CloseRead drains and discards client frames (the state socket is
	// server-push only) and returns a context canceled when the client goes away.
	ctx := conn.CloseRead(r.Context())

	inbox, err := h.store.ListInbox(ctx)
	if err != nil {
		h.logger.Error("ws state inbox", "err", err)
	}
	hello := stateHello{
		T:        "hello",
		Rev:      snap.Rev,
		Nodes:    api.NodesToDTO(snap.Nodes),
		Sessions: api.SessionsToDTO(snap.Sessions),
		Inbox:    api.EventsToDTO(inbox),
	}
	if err := writeState(ctx, conn, hello); err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-deltas:
			if !ok {
				_ = conn.Close(websocket.StatusTryAgainLater, "subscription lagged")
				return
			}
			if err := writeState(ctx, conn, deltaMessage(d)); err != nil {
				return
			}
		}
	}
}

// deltaMessage maps a tree.Delta to its wire frame, mapping only non-empty
// sections so the omitempty tags drop the rest.
func deltaMessage(d tree.Delta) stateDelta {
	msg := stateDelta{T: "delta", Rev: d.Rev}
	if len(d.Nodes) > 0 {
		msg.Nodes = api.NodesToDTO(d.Nodes)
	}
	if len(d.Sessions) > 0 {
		msg.Sessions = api.SessionsToDTO(d.Sessions)
	}
	if len(d.Events) > 0 {
		msg.Events = api.EventsToDTO(d.Events)
	}
	return msg
}

// writeState writes one JSON text frame under the per-frame write timeout.
func writeState(ctx context.Context, conn *websocket.Conn, v any) error {
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return wsjson.Write(wctx, conn, v)
}
