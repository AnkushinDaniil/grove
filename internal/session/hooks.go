package session

import (
	"context"
	"fmt"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// permissionNotificationTypes are the Claude notification kinds that map to a
// permission prompt (versus a plain question).
var permissionNotificationTypes = map[string]bool{
	"permission":        true,
	"permission_prompt": true,
}

// ApplyHook maps a Claude hook post onto tree state for nodeID. The mapping:
//
//	SessionStart → record driver session id / transcript on the current session
//	Notification → EventAwaitingInput (permission vs question) + AwaitingInput
//	Stop         → EventTurnDone ("turn finished") + AwaitingInput (user's turn)
//	SessionEnd   → EventSessionEnded when a reason is present; status is owned
//	               by the process exit path, so this never changes it
//
// Events are ingested against the node (raising attention); session status
// changes go through the live session and respect the lifecycle state machine.
func (m *Manager) ApplyHook(ctx context.Context, nodeID core.NodeID, hookEvent string, payload map[string]any) error {
	if _, ok := m.tree.Get(nodeID); !ok {
		return fmt.Errorf("%w: node %s not found", core.ErrInvalid, nodeID)
	}
	sess, hasSession := m.tree.SessionFor(nodeID)
	var sid core.SessionID
	if hasSession {
		sid = sess.ID
	}

	switch hookEvent {
	case "SessionStart":
		return m.hookSessionStart(ctx, sess, hasSession, payload)
	case "Notification":
		return m.hookNotification(ctx, nodeID, sid, sess, hasSession, payload)
	case "Stop":
		return m.hookStop(ctx, nodeID, sid, sess, hasSession)
	case "SessionEnd":
		return m.hookSessionEnd(ctx, nodeID, sid, payload)
	default:
		return fmt.Errorf("%w: unknown hook event %q", core.ErrInvalid, hookEvent)
	}
}

func (m *Manager) hookSessionStart(ctx context.Context, sess core.Session, hasSession bool, payload map[string]any) error {
	if !hasSession {
		return nil
	}
	driverSessionID, _ := payload["session_id"].(string)
	if driverSessionID == "" {
		return nil
	}
	transcript, _ := payload["transcript_path"].(string)
	updated := sess
	updated.DriverSessionID = driverSessionID
	if transcript != "" {
		updated.TranscriptPath = transcript
	}
	if _, err := m.tree.ApplySession(ctx, updated); err != nil {
		return fmt.Errorf("apply session-start hook: %w", err)
	}
	return nil
}

func (m *Manager) hookNotification(
	ctx context.Context,
	nodeID core.NodeID,
	sid core.SessionID,
	sess core.Session,
	hasSession bool,
	payload map[string]any,
) error {
	reason := core.AwaitQuestion
	if nt, _ := payload["notification_type"].(string); permissionNotificationTypes[nt] {
		reason = core.AwaitPermission
	}
	detail, _ := payload["message"].(string)
	encoded, err := core.MarshalPayload(core.AwaitingPayload{Reason: reason, Detail: detail})
	if err != nil {
		return fmt.Errorf("encode awaiting payload: %w", err)
	}
	if _, err := m.tree.IngestEvents(ctx, nodeID, sid, []core.EventInput{{
		Type:    core.EventAwaitingInput,
		Payload: encoded,
		Reason:  reason,
		Detail:  detail,
	}}); err != nil {
		return fmt.Errorf("ingest notification hook: %w", err)
	}
	if hasSession {
		return m.setSessionStatus(ctx, sess, core.SessionAwaitingInput)
	}
	return nil
}

func (m *Manager) hookStop(ctx context.Context, nodeID core.NodeID, sid core.SessionID, sess core.Session, hasSession bool) error {
	encoded, err := core.MarshalPayload(core.TurnDonePayload{})
	if err != nil {
		return fmt.Errorf("encode turn-done payload: %w", err)
	}
	if _, err := m.tree.IngestEvents(ctx, nodeID, sid, []core.EventInput{{
		Type:    core.EventTurnDone,
		Payload: encoded,
		Detail:  "turn finished",
	}}); err != nil {
		return fmt.Errorf("ingest stop hook: %w", err)
	}
	if hasSession {
		return m.setSessionStatus(ctx, sess, core.SessionAwaitingInput)
	}
	return nil
}

func (m *Manager) hookSessionEnd(ctx context.Context, nodeID core.NodeID, sid core.SessionID, payload map[string]any) error {
	reason, _ := payload["reason"].(string)
	if reason == "" {
		return nil // the process exit path owns end-of-session status
	}
	encoded, err := core.MarshalPayload(core.SessionEndedPayload{})
	if err != nil {
		return fmt.Errorf("encode session-ended payload: %w", err)
	}
	if _, err := m.tree.IngestEvents(ctx, nodeID, sid, []core.EventInput{{
		Type:    core.EventSessionEnded,
		Payload: encoded,
		Detail:  reason,
	}}); err != nil {
		return fmt.Errorf("ingest session-end hook: %w", err)
	}
	return nil
}

// setSessionStatus transitions a session's status if the lifecycle allows it,
// preferring the live snapshot when the session is still running. An illegal or
// redundant transition is silently ignored.
func (m *Manager) setSessionStatus(ctx context.Context, sess core.Session, status core.SessionStatus) error {
	if ls, live := m.get(sess.ID); live {
		ls.mu.Lock()
		cur := ls.sess
		ls.mu.Unlock()
		if !core.CanTransition(cur.Status, status) {
			return nil
		}
		return m.updateSession(ctx, ls, func(s *core.Session) { s.Status = status })
	}
	if !core.CanTransition(sess.Status, status) {
		return nil
	}
	updated := sess
	updated.Status = status
	if _, err := m.tree.ApplySession(ctx, updated); err != nil {
		return fmt.Errorf("apply session status: %w", err)
	}
	return nil
}
