package store

import (
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// day1/day2 are two distinct UTC calendar days used to exercise by-day bucketing.
var (
	day1 = time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	day2 = time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
)

func mustAppend(t *testing.T, s *Store, events ...core.Event) {
	t.Helper()
	if err := s.AppendEvents(t.Context(), events); err != nil {
		t.Fatalf("AppendEvents: %v", err)
	}
}

func usagePayload(t *testing.T, in, out int64, cost float64, model string) string {
	t.Helper()
	p, err := core.MarshalPayload(core.UsagePayload{InputTokens: in, OutputTokens: out, CostUSD: cost, Model: model})
	if err != nil {
		t.Fatalf("MarshalPayload usage: %v", err)
	}
	return p
}

func usageEvent(nodeID core.NodeID, sessID core.SessionID, payload string, at time.Time) core.Event {
	return core.Event{
		ID: core.NewEventID(), NodeID: nodeID, SessionID: sessID,
		Type: core.EventUsage, Payload: payload, CreatedAt: at,
	}
}

// TestStatsTokens seeds usage events across two nodes, two drivers/profiles, two
// models and two days, and asserts every token breakdown plus the model list.
func TestStatsTokens(t *testing.T) {
	s := newTestStore(t)
	nodeA := testNode(core.NewNodeID(), "")
	nodeB := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, nodeA)
	mustSaveNode(t, s, nodeB)

	claude := testSession(core.NewSessionID(), nodeA.ID) // claude / profile-1
	mustSaveSession(t, s, claude)
	codex := testSession(core.NewSessionID(), nodeB.ID)
	codex.Driver = "codex"
	codex.ProfileID = core.ProfileID("profile-2")
	mustSaveSession(t, s, codex)

	mustAppend(t, s,
		usageEvent(nodeA.ID, claude.ID, usagePayload(t, 100, 10, 0.10, "sonnet"), day1),
		usageEvent(nodeA.ID, claude.ID, usagePayload(t, 50, 5, 0.05, "sonnet"), day2),
		usageEvent(nodeB.ID, codex.ID, usagePayload(t, 200, 20, 0.20, "gpt"), day2),
	)

	from, to := day1.Add(-time.Hour), day2.Add(time.Hour)
	res, err := s.Stats(t.Context(), []core.NodeID{nodeA.ID, nodeB.ID}, from, to)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	tok := res.Tokens
	if tok.Total.Input != 350 || tok.Total.Output != 35 {
		t.Errorf("total = %d/%d, want 350/35", tok.Total.Input, tok.Total.Output)
	}
	if tok.Total.CostUSD < 0.349 || tok.Total.CostUSD > 0.351 {
		t.Errorf("total cost = %v, want ~0.35", tok.Total.CostUSD)
	}
	if tok.Total.CacheRead != 0 {
		t.Errorf("cache_read = %d, want 0 (not carried yet)", tok.Total.CacheRead)
	}

	// by_day: 2026-07-20 (100/10) then 2026-07-21 (250/25), ascending.
	if len(tok.ByDay) != 2 {
		t.Fatalf("by_day = %+v, want 2 days", tok.ByDay)
	}
	if tok.ByDay[0].Day != "2026-07-20" || tok.ByDay[0].Input != 100 {
		t.Errorf("by_day[0] = %+v, want 2026-07-20 input=100", tok.ByDay[0])
	}
	if tok.ByDay[1].Day != "2026-07-21" || tok.ByDay[1].Input != 250 {
		t.Errorf("by_day[1] = %+v, want 2026-07-21 input=250", tok.ByDay[1])
	}

	// by_driver: codex (200/20) sorts before claude (150/15) by token volume.
	if len(tok.ByDriver) != 2 || tok.ByDriver[0].Driver != "codex" || tok.ByDriver[0].Input != 200 {
		t.Errorf("by_driver = %+v, want codex first with 200", tok.ByDriver)
	}
	if tok.ByDriver[1].Driver != "claude" || tok.ByDriver[1].Input != 150 {
		t.Errorf("by_driver[1] = %+v, want claude 150", tok.ByDriver[1])
	}

	// by_profile: profile-2 (200) before profile-1 (150).
	if len(tok.ByProfile) != 2 || tok.ByProfile[0].ProfileID != "profile-2" || tok.ByProfile[0].Input != 200 {
		t.Errorf("by_profile = %+v, want profile-2 first with 200", tok.ByProfile)
	}

	// top_nodes: nodeB (200) before nodeA (150).
	if len(tok.TopNodes) != 2 || tok.TopNodes[0].NodeID != string(nodeB.ID) || tok.TopNodes[0].Input != 200 {
		t.Errorf("top_nodes = %+v, want nodeB first with 200", tok.TopNodes)
	}

	// models: gpt (200) before sonnet (150).
	if len(res.Models) != 2 || res.Models[0].Model != "gpt" || res.Models[0].Input != 200 {
		t.Fatalf("models = %+v, want gpt first with 200", res.Models)
	}
	if res.Models[1].Model != "sonnet" || res.Models[1].Input != 150 {
		t.Errorf("models[1] = %+v, want sonnet 150", res.Models[1])
	}
}

// TestStatsToolsAndSkills asserts per-tool call/error counts and Skill-tool
// invocation grouping.
func TestStatsToolsAndSkills(t *testing.T) {
	s := newTestStore(t)
	node := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, node)

	toolCall := func(name, summary string) core.Event {
		p, err := core.MarshalPayload(core.ToolCallPayload{Name: name, InputSummary: summary})
		if err != nil {
			t.Fatalf("marshal tool_call: %v", err)
		}
		return core.Event{ID: core.NewEventID(), NodeID: node.ID, Type: core.EventToolCall, Payload: p, CreatedAt: day1}
	}
	toolResult := func(name string, ok bool) core.Event {
		p, err := core.MarshalPayload(core.ToolResultPayload{Name: name, OK: ok})
		if err != nil {
			t.Fatalf("marshal tool_result: %v", err)
		}
		return core.Event{ID: core.NewEventID(), NodeID: node.ID, Type: core.EventToolResult, Payload: p, CreatedAt: day1}
	}

	mustAppend(t, s,
		toolCall("Bash", ""),
		toolCall("Bash", ""),
		toolResult("Bash", true),
		toolResult("Bash", false), // one Bash error
		toolCall("Read", ""),
		toolCall("Skill", "code-review"),
		toolCall("Skill", "code-review"),
		toolCall("Skill", "verify"),
	)

	from, to := day1.Add(-time.Hour), day1.Add(time.Hour)
	res, err := s.Stats(t.Context(), []core.NodeID{node.ID}, from, to)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	// tools: Bash (3 calls: 2 tool_call + 1 Skill? no) — Bash has 2 calls, 1 error;
	// Skill has 3 calls, Read 1 call. Sorted by calls desc.
	tools := map[string]ToolStat{}
	for _, ts := range res.Tools {
		tools[ts.Name] = ts
	}
	if tools["Bash"].Calls != 2 || tools["Bash"].Errors != 1 {
		t.Errorf("Bash = %+v, want calls=2 errors=1", tools["Bash"])
	}
	if tools["Read"].Calls != 1 || tools["Read"].Errors != 0 {
		t.Errorf("Read = %+v, want calls=1 errors=0", tools["Read"])
	}
	if tools["Skill"].Calls != 3 {
		t.Errorf("Skill tool calls = %d, want 3", tools["Skill"].Calls)
	}

	// skills: code-review (2) before verify (1).
	if len(res.Skills) != 2 || res.Skills[0].Skill != "code-review" || res.Skills[0].Invocations != 2 {
		t.Fatalf("skills = %+v, want code-review=2 first", res.Skills)
	}
	if res.Skills[1].Skill != "verify" || res.Skills[1].Invocations != 1 {
		t.Errorf("skills[1] = %+v, want verify=1", res.Skills[1])
	}
}

// TestStatsFlowAndAgents asserts task throughput, attention-wait percentiles,
// active sessions and session-by-day lifecycle counts.
func TestStatsFlowAndAgents(t *testing.T) {
	s := newTestStore(t)

	// Two done tasks (durations 2h and 4h) and one failed task, all updated on day2.
	doneA := taskNode(core.NewNodeID(), core.StatusDone, day1, day1.Add(2*time.Hour))
	doneB := taskNode(core.NewNodeID(), core.StatusDone, day1, day1.Add(4*time.Hour))
	failed := taskNode(core.NewNodeID(), core.StatusFailed, day1, day1.Add(time.Hour))
	mustSaveNode(t, s, doneA)
	mustSaveNode(t, s, doneB)
	mustSaveNode(t, s, failed)
	scope := []core.NodeID{doneA.ID, doneB.ID, failed.ID}

	// Attention events: waits of 2 and 10 minutes, plus one unacked (excluded).
	ackedWait := func(nodeID core.NodeID, waitMin int64) core.Event {
		return core.Event{
			ID: core.NewEventID(), NodeID: nodeID, Type: core.EventAwaitingInput,
			Payload: `{"reason":"question"}`, RequiresAttention: true,
			CreatedAt: day1, AckedAt: day1.Add(time.Duration(waitMin) * time.Minute),
		}
	}
	unacked := core.Event{
		ID: core.NewEventID(), NodeID: doneA.ID, Type: core.EventAwaitingInput,
		Payload: `{"reason":"question"}`, RequiresAttention: true, CreatedAt: day1,
	}
	mustAppend(t, s, ackedWait(doneA.ID, 2), ackedWait(doneB.ID, 10), unacked)

	// Sessions: one active (running, no range constraint), one exited and one
	// failed within range on distinct nodes.
	active := testSession(core.NewSessionID(), doneA.ID)
	active.Status = core.SessionRunning
	active.StartedAt = day1
	mustSaveSession(t, s, active)

	exited := testSession(core.NewSessionID(), doneB.ID)
	exited.Status = core.SessionExited
	exited.StartedAt = day2
	exited.EndedAt = day2.Add(30 * time.Minute)
	mustSaveSession(t, s, exited)

	failedSess := testSession(core.NewSessionID(), failed.ID)
	failedSess.Driver = "codex"
	failedSess.Status = core.SessionFailed
	failedSess.StartedAt = day2
	failedSess.EndedAt = day2.Add(10 * time.Minute)
	mustSaveSession(t, s, failedSess)

	from, to := day1.Add(-time.Hour), day2.Add(2*time.Hour)
	res, err := s.Stats(t.Context(), scope, from, to)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	flow := res.Flow
	if flow.TasksCreated != 3 || flow.TasksDone != 2 || flow.TasksFailed != 1 {
		t.Errorf("flow tasks = created:%d done:%d failed:%d, want 3/2/1",
			flow.TasksCreated, flow.TasksDone, flow.TasksFailed)
	}
	// Median of {2h, 4h} = 3h.
	if flow.MedianTaskHours < 2.99 || flow.MedianTaskHours > 3.01 {
		t.Errorf("median_task_hours = %v, want ~3", flow.MedianTaskHours)
	}
	// Waits {2, 10} min: p50 (linear interp of 2 points at 0.5) = 6; p95 = 9.6.
	if flow.AttentionWaitP50Minutes < 5.99 || flow.AttentionWaitP50Minutes > 6.01 {
		t.Errorf("attention p50 = %v, want ~6", flow.AttentionWaitP50Minutes)
	}
	if flow.AttentionWaitP95Minutes < 9.59 || flow.AttentionWaitP95Minutes > 9.61 {
		t.Errorf("attention p95 = %v, want ~9.6", flow.AttentionWaitP95Minutes)
	}
	if flow.PRsOpened != 0 || flow.PRsMerged != 0 {
		t.Errorf("prs = %d/%d, want 0/0 (no PR meta recorded)", flow.PRsOpened, flow.PRsMerged)
	}

	ag := res.Agents
	if ag.SessionsActive != 1 {
		t.Errorf("sessions_active = %d, want 1", ag.SessionsActive)
	}
	// avg over the two ended sessions in range: (30 + 10)/2 = 20 minutes.
	if ag.AvgSessionMinutes < 19.99 || ag.AvgSessionMinutes > 20.01 {
		t.Errorf("avg_session_minutes = %v, want ~20", ag.AvgSessionMinutes)
	}
	// sessions_by_day for day2: exited→done=1, failed→failed=1.
	var day2Row *SessionDay
	for i := range ag.SessionsByDay {
		if ag.SessionsByDay[i].Day == "2026-07-21" {
			day2Row = &ag.SessionsByDay[i]
		}
	}
	if day2Row == nil || day2Row.Done != 1 || day2Row.Failed != 1 {
		t.Errorf("day2 sessions = %+v, want done=1 failed=1", day2Row)
	}
}

// TestStatsScopeAndRangeFiltering verifies a node outside the scope subtree and
// an event outside the time range are both excluded.
func TestStatsScopeAndRangeFiltering(t *testing.T) {
	s := newTestStore(t)
	inScope := testNode(core.NewNodeID(), "")
	outScope := testNode(core.NewNodeID(), "")
	mustSaveNode(t, s, inScope)
	mustSaveNode(t, s, outScope)
	sess := testSession(core.NewSessionID(), inScope.ID)
	mustSaveSession(t, s, sess)

	inRange := usageEvent(inScope.ID, sess.ID, usagePayload(t, 100, 10, 0.1, "sonnet"), day2)
	tooOld := usageEvent(inScope.ID, sess.ID, usagePayload(t, 999, 99, 9.9, "sonnet"), day1.Add(-48*time.Hour))
	outOfScope := usageEvent(outScope.ID, "", usagePayload(t, 500, 50, 5.0, "sonnet"), day2)
	mustAppend(t, s, inRange, tooOld, outOfScope)

	from, to := day2.Add(-time.Hour), day2.Add(time.Hour)
	res, err := s.Stats(t.Context(), []core.NodeID{inScope.ID}, from, to)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if res.Tokens.Total.Input != 100 {
		t.Errorf("total input = %d, want 100 (out-of-scope and out-of-range excluded)", res.Tokens.Total.Input)
	}
}

func TestStatsEmptyScope(t *testing.T) {
	s := newTestStore(t)
	res, err := s.Stats(t.Context(), nil, day1, day2)
	if err != nil {
		t.Fatalf("Stats(nil scope): %v", err)
	}
	if res.Tokens.Total.Input != 0 || len(res.Tools) != 0 || len(res.Models) != 0 {
		t.Errorf("empty scope result not zeroed: %+v", res)
	}
}

func TestPercentileMS(t *testing.T) {
	tests := []struct {
		name string
		in   []int64
		p    float64
		want float64
	}{
		{"empty is zero", nil, 0.5, 0},
		{"single", []int64{42}, 0.95, 42},
		{"median of two", []int64{2, 10}, 0.5, 6},
		{"p95 of two", []int64{2, 10}, 0.95, 9.6},
		{"median of three", []int64{1, 2, 9}, 0.5, 2},
	}
	for _, tt := range tests {
		if got := percentileMS(tt.in, tt.p); got < tt.want-0.001 || got > tt.want+0.001 {
			t.Errorf("%s: percentileMS(%v, %v) = %v, want %v", tt.name, tt.in, tt.p, got, tt.want)
		}
	}
}

// taskNode returns a task node fixture with explicit status and created/updated
// timestamps, for exercising flow aggregation.
func taskNode(id core.NodeID, status core.NodeStatus, created, updated time.Time) core.Node {
	n := testNode(id, "")
	n.Kind = core.KindTask
	n.Status = status
	n.CreatedAt = created.UTC()
	n.UpdatedAt = updated.UTC()
	return n
}
