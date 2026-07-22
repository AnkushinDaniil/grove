package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// testModel is the model attributed to seeded usage events.
const testModel = "claude-sonnet-5"

// recentTime is a timestamp safely inside every stats window (a minute ago), so
// seeded "in range" data never races the half-open upper bound (to = now).
func recentTime() time.Time { return time.Now().Add(-time.Minute) }

// seedUsage appends a usage event on nodeID attributed to sessID.
func (h *harness) seedUsage(nodeID core.NodeID, sessID core.SessionID, in, out int64, cost float64, at time.Time) {
	h.t.Helper()
	payload, err := core.MarshalPayload(core.UsagePayload{InputTokens: in, OutputTokens: out, CostUSD: cost, Model: testModel})
	if err != nil {
		h.t.Fatalf("marshal usage: %v", err)
	}
	h.appendEvent(core.Event{
		ID: core.NewEventID(), NodeID: nodeID, SessionID: sessID,
		Type: core.EventUsage, Payload: payload, CreatedAt: at,
	})
}

func (h *harness) appendEvent(ev core.Event) {
	h.t.Helper()
	if err := h.store.AppendEvents(h.t.Context(), []core.Event{ev}); err != nil {
		h.t.Fatalf("AppendEvents: %v", err)
	}
}

func (h *harness) seedSession(nodeID core.NodeID, status core.SessionStatus) core.SessionID {
	h.t.Helper()
	sess := core.Session{
		ID: core.NewSessionID(), NodeID: nodeID, Driver: "claude",
		Mode: core.ModeHeadless, Status: status, CWD: "/tmp", StartedAt: recentTime(),
	}
	if err := h.store.SaveSessions(h.t.Context(), []core.Session{sess}); err != nil {
		h.t.Fatalf("SaveSessions: %v", err)
	}
	return sess.ID
}

func TestStatsAggregationShape(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")
	projectID := core.NodeID(project.ID)
	// A task node under the project (counts toward tasks_created). Created before
	// the rest of the seeding so its stamped-now created_at sits safely before the
	// handler's window end.
	h.createNode(projectID, core.KindTask, "T", "fake")

	sess := h.seedSession(projectID, core.SessionRunning)
	at := recentTime()
	h.seedUsage(projectID, sess, 100, 10, 0.10, at)
	h.seedUsage(projectID, sess, 50, 5, 0.05, at)

	// tool_call/tool_result and a Skill call.
	h.appendEvent(h.toolEvent(projectID, core.EventToolCall, `{"name":"Bash"}`, at))
	h.appendEvent(h.toolEvent(projectID, core.EventToolResult, `{"name":"Bash","ok":false}`, at))
	h.appendEvent(h.toolEvent(projectID, core.EventToolCall, `{"name":"Skill","input_summary":"code-review"}`, at))

	// Feedback on the project.
	h.postFeedback(project.ID, "skill", "code-review", "misfired")

	var stats statsResponse
	h.decode(h.do(http.MethodGet, "/api/v1/stats?range=24h", nil), http.StatusOK, &stats)

	if stats.Range != "24h" || stats.Scope != string(h.root.ID) {
		t.Errorf("range/scope = %q/%q, want 24h/%s", stats.Range, stats.Scope, h.root.ID)
	}
	if stats.Tokens.Total.Input != 150 || stats.Tokens.Total.Output != 15 {
		t.Errorf("tokens total = %d/%d, want 150/15", stats.Tokens.Total.Input, stats.Tokens.Total.Output)
	}
	if len(stats.Tokens.ByDriver) != 1 || stats.Tokens.ByDriver[0].Driver != "claude" {
		t.Errorf("by_driver = %+v, want one claude row", stats.Tokens.ByDriver)
	}
	if len(stats.Models) != 1 || stats.Models[0].Model != "claude-sonnet-5" || stats.Models[0].Input != 150 {
		t.Errorf("models = %+v, want claude-sonnet-5 with 150", stats.Models)
	}
	bash := findTool(stats.Tools, "Bash")
	if bash == nil || bash.Calls != 1 || bash.Errors != 1 {
		t.Errorf("Bash tool = %+v, want calls=1 errors=1", bash)
	}
	if len(stats.Skills) != 1 || stats.Skills[0].Skill != "code-review" || stats.Skills[0].Invocations != 1 {
		t.Errorf("skills = %+v, want code-review=1", stats.Skills)
	}
	if stats.Agents.SessionsActive != 1 {
		t.Errorf("sessions_active = %d, want 1", stats.Agents.SessionsActive)
	}
	if stats.Flow.TasksCreated != 1 {
		t.Errorf("tasks_created = %d, want 1", stats.Flow.TasksCreated)
	}
	if len(stats.Feedback) != 1 || stats.Feedback[0].Kind != "skill" ||
		stats.Feedback[0].Subject != "code-review" || stats.Feedback[0].Open != 1 || stats.Feedback[0].Total != 1 {
		t.Errorf("feedback = %+v, want one open skill/code-review", stats.Feedback)
	}
}

func TestStatsRangeValidation(t *testing.T) {
	h := newHarness(t, nil)
	h.decode(h.do(http.MethodGet, "/api/v1/stats?range=bogus", nil), http.StatusBadRequest, nil)
	h.decode(h.do(http.MethodGet, "/api/v1/stats?range=1h", nil), http.StatusBadRequest, nil)
	// Each valid range is accepted.
	for _, rng := range []string{"24h", "7d", "30d"} {
		h.decode(h.do(http.MethodGet, "/api/v1/stats?range="+rng, nil), http.StatusOK, nil)
	}
}

func TestStatsUnknownScope(t *testing.T) {
	h := newHarness(t, nil)
	h.decode(h.do(http.MethodGet, "/api/v1/stats?scope=does-not-exist", nil), http.StatusBadRequest, nil)
}

func TestStatsScopeSubtreeFiltering(t *testing.T) {
	h := newHarness(t, nil)
	projA := h.createNode(h.root.ID, core.KindProject, "A", "fake")
	projB := h.createNode(h.root.ID, core.KindProject, "B", "fake")
	sessA := h.seedSession(core.NodeID(projA.ID), core.SessionExited)
	sessB := h.seedSession(core.NodeID(projB.ID), core.SessionExited)

	at := recentTime()
	h.seedUsage(core.NodeID(projA.ID), sessA, 100, 10, 0.1, at)
	h.seedUsage(core.NodeID(projB.ID), sessB, 500, 50, 0.5, at)

	// Scoped to A: only A's usage.
	var scopedA statsResponse
	h.decode(h.do(http.MethodGet, "/api/v1/stats?scope="+projA.ID+"&range=24h", nil), http.StatusOK, &scopedA)
	if scopedA.Scope != projA.ID || scopedA.Tokens.Total.Input != 100 {
		t.Errorf("scoped A input = %d (scope %q), want 100 excluding B", scopedA.Tokens.Total.Input, scopedA.Scope)
	}

	// Whole workspace (default root): both.
	var whole statsResponse
	h.decode(h.do(http.MethodGet, "/api/v1/stats?range=24h", nil), http.StatusOK, &whole)
	if whole.Tokens.Total.Input != 600 {
		t.Errorf("workspace input = %d, want 600 (A+B)", whole.Tokens.Total.Input)
	}
}

func TestStatsRangeExcludesOldEvents(t *testing.T) {
	h := newHarness(t, nil)
	project := h.createNode(h.root.ID, core.KindProject, "Proj", "fake")
	projectID := core.NodeID(project.ID)
	sess := h.seedSession(projectID, core.SessionExited)

	h.seedUsage(projectID, sess, 100, 10, 0.1, recentTime())
	h.seedUsage(projectID, sess, 999, 99, 9.9, time.Now().Add(-48*time.Hour))

	var stats statsResponse
	h.decode(h.do(http.MethodGet, "/api/v1/stats?range=24h", nil), http.StatusOK, &stats)
	if stats.Tokens.Total.Input != 100 {
		t.Errorf("input = %d, want 100 (48h-old event excluded from 24h range)", stats.Tokens.Total.Input)
	}
}

func (h *harness) toolEvent(nodeID core.NodeID, typ core.EventType, payload string, at time.Time) core.Event {
	return core.Event{ID: core.NewEventID(), NodeID: nodeID, Type: typ, Payload: payload, CreatedAt: at}
}

func findTool(tools []toolStatDTO, name string) *toolStatDTO {
	for i := range tools {
		if tools[i].Name == name {
			return &tools[i]
		}
	}
	return nil
}
