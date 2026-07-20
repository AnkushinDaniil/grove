package store

import (
	"testing"

	"github.com/AnkushinDaniil/grove/internal/core"
)

func testEvent(id core.EventID, nodeID core.NodeID, requiresAttention bool) core.Event {
	return core.Event{
		ID:                id,
		NodeID:            nodeID,
		SessionID:         "",
		Type:              core.EventText,
		Payload:           `{"text":"hi","final":false}`,
		RequiresAttention: requiresAttention,
		CreatedAt:         msTime(1_700_000_000_000),
	}
}

func TestAppendEventsAndListEventsPagination(t *testing.T) {
	s := newTestStore(t)
	n := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, n)

	e1 := testEvent(core.NewEventID(), n.ID, false)
	e2 := testEvent(core.NewEventID(), n.ID, false)
	e3 := testEvent(core.NewEventID(), n.ID, false)
	if err := s.AppendEvents(t.Context(), []core.Event{e1, e2, e3}); err != nil {
		t.Fatalf("AppendEvents: %v", err)
	}

	all, err := s.ListEvents(t.Context(), n.ID, "", 0)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(all) != 3 || all[0].ID != e1.ID || all[1].ID != e2.ID || all[2].ID != e3.ID {
		t.Fatalf("ListEvents(afterID=\"\") ids = %v, want [%s %s %s] in order", ids(all), e1.ID, e2.ID, e3.ID)
	}

	page, err := s.ListEvents(t.Context(), n.ID, e1.ID, 0)
	if err != nil {
		t.Fatalf("ListEvents after e1: %v", err)
	}
	if len(page) != 2 || page[0].ID != e2.ID || page[1].ID != e3.ID {
		t.Errorf("ListEvents(afterID=e1) ids = %v, want [%s %s]", ids(page), e2.ID, e3.ID)
	}

	limited, err := s.ListEvents(t.Context(), n.ID, "", 1)
	if err != nil {
		t.Fatalf("ListEvents limit=1: %v", err)
	}
	if len(limited) != 1 || limited[0].ID != e1.ID {
		t.Errorf("ListEvents(limit=1) ids = %v, want [%s]", ids(limited), e1.ID)
	}

	none, err := s.ListEvents(t.Context(), n.ID, e3.ID, 0)
	if err != nil {
		t.Fatalf("ListEvents after e3: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("ListEvents(afterID=e3) ids = %v, want none left", ids(none))
	}
}

func ids(events []core.Event) []core.EventID {
	out := make([]core.EventID, len(events))
	for i, e := range events {
		out[i] = e.ID
	}
	return out
}

func TestListEventsLimitClamp(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{"zero defaults", 0, defaultEventsLimit},
		{"negative defaults", -5, defaultEventsLimit},
		{"over max clamps", maxEventsLimit + 1000, maxEventsLimit},
		{"within range passes through", 10, 10},
	}
	for _, tt := range tests {
		if got := clampEventsLimit(tt.limit); got != tt.want {
			t.Errorf("%s: clampEventsLimit(%d) = %d, want %d", tt.name, tt.limit, got, tt.want)
		}
	}
}

func TestInboxAndAckFlow(t *testing.T) {
	s := newTestStore(t)
	n1 := testNode(core.NewNodeID(), "")
	n2 := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, n1)
	mustSaveNode(t, s, n2)

	attn1 := testEvent(core.NewEventID(), n1.ID, true)
	plain := testEvent(core.NewEventID(), n1.ID, false)
	attn2 := testEvent(core.NewEventID(), n2.ID, true)
	if err := s.AppendEvents(t.Context(), []core.Event{attn1, plain, attn2}); err != nil {
		t.Fatalf("AppendEvents: %v", err)
	}

	inbox, err := s.ListInbox(t.Context())
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	// Newest first: attn2 was appended after attn1, and IDs are time-sortable.
	if len(inbox) != 2 || inbox[0].ID != attn2.ID || inbox[1].ID != attn1.ID {
		t.Fatalf("ListInbox ids = %v, want [%s %s]", ids(inbox), attn2.ID, attn1.ID)
	}

	at := msTime(1_700_000_020_000)
	affected, err := s.AckNodeEvents(t.Context(), n1.ID, at)
	if err != nil {
		t.Fatalf("AckNodeEvents: %v", err)
	}
	if affected != 1 {
		t.Errorf("AckNodeEvents affected = %d, want 1", affected)
	}

	inboxAfter, err := s.ListInbox(t.Context())
	if err != nil {
		t.Fatalf("ListInbox after ack: %v", err)
	}
	if len(inboxAfter) != 1 || inboxAfter[0].ID != attn2.ID {
		t.Errorf("ListInbox after ack ids = %v, want [%s]", ids(inboxAfter), attn2.ID)
	}

	// Acking again finds nothing left to ack: not an error, zero affected.
	affected2, err := s.AckNodeEvents(t.Context(), n1.ID, at)
	if err != nil {
		t.Fatalf("AckNodeEvents (second): %v", err)
	}
	if affected2 != 0 {
		t.Errorf("second AckNodeEvents affected = %d, want 0", affected2)
	}
}

func TestAppendEventsEmptyIsNoop(t *testing.T) {
	s := newTestStore(t)
	if err := s.AppendEvents(t.Context(), nil); err != nil {
		t.Errorf("AppendEvents(nil): %v", err)
	}
}
