package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
)

// statsTopNodesLimit caps tokens.top_nodes to the heaviest consumers so the
// payload stays small on large subtrees.
const statsTopNodesLimit = 10

const (
	msPerMinute = 60_000.0
	msPerHour   = 3_600_000.0
	dayLayout   = "2006-01-02"
)

// StatsResult is the aggregation backing GET /stats for one scope subtree and
// time range, all computed from the local DB. Node titles and profile display
// names are left to the API layer to decorate (it owns the tree and profile
// tables), so the token breakdowns here carry ids only.
type StatsResult struct {
	Tokens   TokenStats
	Agents   AgentStats
	Flow     FlowStats
	Tools    []ToolStat
	Models   []ModelStat
	Skills   []SkillStat
	Feedback []FeedbackStat
}

// TokenTotal is the summed token usage over the whole scope+range. CacheRead is
// always zero until usage payloads carry cache-read tokens (matching GET /usage).
type TokenTotal struct {
	Input     int64
	Output    int64
	CacheRead int64
	CostUSD   float64
}

// TokenDay is one calendar day's (UTC) token usage.
type TokenDay struct {
	Day     string
	Input   int64
	Output  int64
	CostUSD float64
}

// TokenDriver is one driver's token usage.
type TokenDriver struct {
	Driver  string
	Input   int64
	Output  int64
	CostUSD float64
}

// TokenProfile is one profile's token usage (Name is filled by the API).
type TokenProfile struct {
	ProfileID string
	Input     int64
	Output    int64
	CostUSD   float64
}

// TokenNode is one node's token usage (Title is filled by the API).
type TokenNode struct {
	NodeID  string
	Input   int64
	Output  int64
	CostUSD float64
}

// TokenStats is the tokens section: a grand total plus by-day, by-driver,
// by-profile and top-node breakdowns.
type TokenStats struct {
	Total     TokenTotal
	ByDay     []TokenDay
	ByDriver  []TokenDriver
	ByProfile []TokenProfile
	TopNodes  []TokenNode
}

// ModelStat is one model's token usage, parsed from UsagePayload.Model.
type ModelStat struct {
	Model   string
	Input   int64
	Output  int64
	CostUSD float64
}

// SessionDay is one day's session lifecycle counts.
type SessionDay struct {
	Day     string
	Started int
	Done    int
	Failed  int
}

// DriverCount is a session count for one driver.
type DriverCount struct {
	Driver string
	Count  int
}

// AgentStats is the agents section: live session count plus by-day lifecycle,
// average duration and per-driver counts over the range.
type AgentStats struct {
	SessionsActive    int
	SessionsByDay     []SessionDay
	AvgSessionMinutes float64
	ByDriver          []DriverCount
}

// FlowStats is the flow section: task throughput, task/attention latencies and
// PR counts. MedianTaskHours uses updated_at as the done timestamp; the
// attention percentiles measure the wait between a requires-attention event and
// its ack; PR counts are read from node meta (0 until PR urls are recorded).
type FlowStats struct {
	TasksCreated            int
	TasksDone               int
	TasksFailed             int
	MedianTaskHours         float64
	AttentionWaitP50Minutes float64
	AttentionWaitP95Minutes float64
	PRsOpened               int
	PRsMerged               int
}

// ToolStat is one tool's call and error counts (errors from tool_result.ok=false).
type ToolStat struct {
	Name   string
	Calls  int
	Errors int
}

// SkillStat is one skill's invocation count, from tool_call payloads where the
// tool name is "Skill".
type SkillStat struct {
	Skill       string
	Invocations int
}

// Stats aggregates every stats section for the given scope subtree over the
// half-open range [from, to). An empty scope yields a fully zeroed result. Each
// section is a small, indexed query whose JSON payloads are parsed in Go, the
// same way the usage aggregator handles usage events.
func (s *Store) Stats(ctx context.Context, nodeIDs []core.NodeID, from, to time.Time) (StatsResult, error) {
	placeholders, nodeArgs, ok := nodeIDPlaceholders(nodeIDs)
	if !ok {
		return StatsResult{}, nil
	}

	tokens, models, err := s.statsTokens(ctx, placeholders, nodeArgs, from, to)
	if err != nil {
		return StatsResult{}, err
	}
	agents, err := s.statsAgents(ctx, placeholders, nodeArgs, from, to)
	if err != nil {
		return StatsResult{}, err
	}
	flow, err := s.statsFlow(ctx, placeholders, nodeArgs, from, to)
	if err != nil {
		return StatsResult{}, err
	}
	tools, skills, err := s.statsToolsAndSkills(ctx, placeholders, nodeArgs, from, to)
	if err != nil {
		return StatsResult{}, err
	}
	feedback, err := s.FeedbackBreakdown(ctx, nodeIDs)
	if err != nil {
		return StatsResult{}, err
	}

	return StatsResult{
		Tokens:   tokens,
		Agents:   agents,
		Flow:     flow,
		Tools:    tools,
		Models:   models,
		Skills:   skills,
		Feedback: feedback,
	}, nil
}

// nodeIDPlaceholders builds a "?,?,..." placeholder list and matching args for a
// set of node ids, to scope a query to a subtree. ok is false for an empty set
// (an empty IN () is invalid SQL; callers short-circuit to a zeroed result).
func nodeIDPlaceholders(ids []core.NodeID) (placeholders string, args []any, ok bool) {
	if len(ids) == 0 {
		return "", nil, false
	}
	placeholders = strings.Repeat("?,", len(ids)-1) + "?"
	args = make([]any, len(ids))
	for i, id := range ids {
		args[i] = string(id)
	}
	return placeholders, args, true
}

// argsNodesFirst builds a query arg slice with the node scope args first (for
// queries whose IN clause precedes the fixed conditions).
func argsNodesFirst(nodeArgs []any, fixed ...any) []any {
	out := make([]any, 0, len(nodeArgs)+len(fixed))
	out = append(out, nodeArgs...)
	out = append(out, fixed...)
	return out
}

// argsNodesLast builds a query arg slice with the fixed args first (for queries
// whose IN clause follows the fixed conditions).
func argsNodesLast(nodeArgs []any, fixed ...any) []any {
	out := make([]any, 0, len(fixed)+len(nodeArgs))
	out = append(out, fixed...)
	out = append(out, nodeArgs...)
	return out
}

// statsUsageRow is one usage event with its owning session's attribution.
type statsUsageRow struct {
	NodeID    string
	Driver    string
	ProfileID string
	CreatedAt int64
	Payload   string
}

func scanStatsUsageRow(row rowScanner) (statsUsageRow, error) {
	var r statsUsageRow
	if err := row.Scan(&r.NodeID, &r.Driver, &r.ProfileID, &r.CreatedAt, &r.Payload); err != nil {
		return statsUsageRow{}, fmt.Errorf("scan stats usage row: %w", err)
	}
	return r, nil
}

// statsTokens aggregates usage events into the token breakdowns and the by-model
// list. Node attribution comes from events.node_id directly; driver/profile come
// from the joined session (usage_rollup carries no node_id, so events are the
// only node-scoped source).
func (s *Store) statsTokens(
	ctx context.Context, placeholders string, nodeArgs []any, from, to time.Time,
) (TokenStats, []ModelStat, error) {
	//nolint:gosec // G202: placeholders is only "?," separators; every id is a bound parameter passed via args.
	query := `
SELECT e.node_id, COALESCE(s.driver, ''), COALESCE(s.profile_id, ''), e.created_at, e.payload
FROM events e
LEFT JOIN sessions s ON s.id = e.session_id
WHERE e.type = ? AND e.created_at >= ? AND e.created_at < ? AND e.node_id IN (` + placeholders + `)`
	args := argsNodesLast(nodeArgs, string(core.EventUsage), from.UnixMilli(), to.UnixMilli())
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return TokenStats{}, nil, fmt.Errorf("stats tokens: %w", err)
	}
	usageRows, err := collect(rows, scanStatsUsageRow)
	if err != nil {
		return TokenStats{}, nil, fmt.Errorf("stats tokens: %w", err)
	}

	var total TokenTotal
	byDay := map[string]*TokenDay{}
	byDriver := map[string]*TokenDriver{}
	byProfile := map[string]*TokenProfile{}
	byNode := map[string]*TokenNode{}
	byModel := map[string]*ModelStat{}

	for _, r := range usageRows {
		p, err := core.UnmarshalPayload[core.UsagePayload](r.Payload)
		if err != nil {
			return TokenStats{}, nil, fmt.Errorf("stats tokens: %w", err)
		}
		total.Input += p.InputTokens
		total.Output += p.OutputTokens
		total.CostUSD += p.CostUSD

		day := timeFromMS(r.CreatedAt).Format(dayLayout)
		d := byDay[day]
		if d == nil {
			d = &TokenDay{Day: day}
			byDay[day] = d
		}
		d.Input += p.InputTokens
		d.Output += p.OutputTokens
		d.CostUSD += p.CostUSD

		dr := byDriver[r.Driver]
		if dr == nil {
			dr = &TokenDriver{Driver: r.Driver}
			byDriver[r.Driver] = dr
		}
		dr.Input += p.InputTokens
		dr.Output += p.OutputTokens
		dr.CostUSD += p.CostUSD

		pr := byProfile[r.ProfileID]
		if pr == nil {
			pr = &TokenProfile{ProfileID: r.ProfileID}
			byProfile[r.ProfileID] = pr
		}
		pr.Input += p.InputTokens
		pr.Output += p.OutputTokens
		pr.CostUSD += p.CostUSD

		nd := byNode[r.NodeID]
		if nd == nil {
			nd = &TokenNode{NodeID: r.NodeID}
			byNode[r.NodeID] = nd
		}
		nd.Input += p.InputTokens
		nd.Output += p.OutputTokens
		nd.CostUSD += p.CostUSD

		if p.Model != "" {
			m := byModel[p.Model]
			if m == nil {
				m = &ModelStat{Model: p.Model}
				byModel[p.Model] = m
			}
			m.Input += p.InputTokens
			m.Output += p.OutputTokens
			m.CostUSD += p.CostUSD
		}
	}

	return TokenStats{
		Total:     total,
		ByDay:     sortedTokenDays(byDay),
		ByDriver:  sortedTokenDrivers(byDriver),
		ByProfile: sortedTokenProfiles(byProfile),
		TopNodes:  topTokenNodes(byNode, statsTopNodesLimit),
	}, sortedModels(byModel), nil
}

// statsSessionRow is one session's driver/status/lifecycle timestamps.
type statsSessionRow struct {
	Driver    string
	Status    string
	StartedAt int64
	EndedAt   int64 // 0 when NULL (still live)
}

func scanStatsSessionRow(row rowScanner) (statsSessionRow, error) {
	var (
		r     statsSessionRow
		ended sql.NullInt64
	)
	if err := row.Scan(&r.Driver, &r.Status, &r.StartedAt, &ended); err != nil {
		return statsSessionRow{}, fmt.Errorf("scan stats session row: %w", err)
	}
	if ended.Valid {
		r.EndedAt = ended.Int64
	}
	return r, nil
}

// statsAgents counts active sessions (whole scope, no range) and aggregates the
// sessions that started or ended within the range into by-day lifecycle counts,
// average duration and per-driver counts.
func (s *Store) statsAgents(
	ctx context.Context, placeholders string, nodeArgs []any, from, to time.Time,
) (AgentStats, error) {
	active, err := s.sessionsActive(ctx, placeholders, nodeArgs)
	if err != nil {
		return AgentStats{}, err
	}

	//nolint:gosec // G202: placeholders is only "?," separators; every id is a bound parameter passed via args.
	query := `
SELECT driver, status, started_at, ended_at
FROM sessions
WHERE node_id IN (` + placeholders + `) AND (
	(started_at >= ? AND started_at < ?) OR
	(ended_at IS NOT NULL AND ended_at >= ? AND ended_at < ?)
)`
	fromMS, toMS := from.UnixMilli(), to.UnixMilli()
	args := argsNodesFirst(nodeArgs, fromMS, toMS, fromMS, toMS)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return AgentStats{}, fmt.Errorf("stats agents: %w", err)
	}
	sessRows, err := collect(rows, scanStatsSessionRow)
	if err != nil {
		return AgentStats{}, fmt.Errorf("stats agents: %w", err)
	}

	byDay := map[string]*SessionDay{}
	byDriver := map[string]int{}
	var durationSumMS, endedInRange int64
	for _, r := range sessRows {
		if r.StartedAt >= fromMS && r.StartedAt < toMS {
			byDriver[r.Driver]++
			day := byDay[dayFromMS(r.StartedAt)]
			if day == nil {
				day = &SessionDay{Day: dayFromMS(r.StartedAt)}
				byDay[day.Day] = day
			}
			day.Started++
		}
		if r.EndedAt >= fromMS && r.EndedAt < toMS {
			day := byDay[dayFromMS(r.EndedAt)]
			if day == nil {
				day = &SessionDay{Day: dayFromMS(r.EndedAt)}
				byDay[day.Day] = day
			}
			if r.Status == string(core.SessionExited) {
				day.Done++
			} else {
				day.Failed++
			}
			if r.StartedAt > 0 && r.EndedAt >= r.StartedAt {
				durationSumMS += r.EndedAt - r.StartedAt
				endedInRange++
			}
		}
	}

	avg := 0.0
	if endedInRange > 0 {
		avg = float64(durationSumMS) / float64(endedInRange) / msPerMinute
	}
	return AgentStats{
		SessionsActive:    active,
		SessionsByDay:     sortedSessionDays(byDay),
		AvgSessionMinutes: avg,
		ByDriver:          sortedDriverCounts(byDriver),
	}, nil
}

// sessionsActive counts sessions in scope currently in a live status, regardless
// of range (an "active now" gauge).
func (s *Store) sessionsActive(ctx context.Context, placeholders string, nodeArgs []any) (int, error) {
	query := `
SELECT COUNT(*)
FROM sessions
WHERE node_id IN (` + placeholders + `) AND status IN (?, ?, ?)`
	args := argsNodesFirst(nodeArgs,
		string(core.SessionStarting), string(core.SessionRunning), string(core.SessionAwaitingInput))
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("stats sessions active: %w", err)
	}
	return count, nil
}

// statsNodeRow is one node's kind/status/timestamps/meta, for task and PR counts.
type statsNodeRow struct {
	Kind      string
	Status    string
	CreatedAt int64
	UpdatedAt int64
	Meta      string
}

func scanStatsNodeRow(row rowScanner) (statsNodeRow, error) {
	var r statsNodeRow
	if err := row.Scan(&r.Kind, &r.Status, &r.CreatedAt, &r.UpdatedAt, &r.Meta); err != nil {
		return statsNodeRow{}, fmt.Errorf("scan stats node row: %w", err)
	}
	return r, nil
}

// prMeta is the forward-compatible shape statsFlow reads from node meta to count
// PRs. No writer records these yet, so PR counts are 0 in practice; the fields
// activate automatically once a PR-from-task flow stamps them.
type prMeta struct {
	PRURL      string `json:"pr_url"`
	PRMerged   bool   `json:"pr_merged"`
	PRMergedAt string `json:"pr_merged_at"`
}

// statsFlow counts task throughput (created/done/failed within the range),
// the median task duration, the attention-wait percentiles, and PRs from meta.
func (s *Store) statsFlow(
	ctx context.Context, placeholders string, nodeArgs []any, from, to time.Time,
) (FlowStats, error) {
	//nolint:gosec // G202: placeholders is only "?," separators; every id is a bound parameter passed via args.
	nodeQuery := `
SELECT kind, status, created_at, updated_at, meta
FROM nodes
WHERE id IN (` + placeholders + `)`
	rows, err := s.db.QueryContext(ctx, nodeQuery, nodeArgs...)
	if err != nil {
		return FlowStats{}, fmt.Errorf("stats flow: %w", err)
	}
	nodeRows, err := collect(rows, scanStatsNodeRow)
	if err != nil {
		return FlowStats{}, fmt.Errorf("stats flow: %w", err)
	}

	fromMS, toMS := from.UnixMilli(), to.UnixMilli()
	var flow FlowStats
	var taskDurationsMS []int64
	for _, r := range nodeRows {
		flow.PRsOpened, flow.PRsMerged = accumulatePR(r.Meta, flow.PRsOpened, flow.PRsMerged)
		if r.Kind != string(core.KindTask) {
			continue
		}
		if r.CreatedAt >= fromMS && r.CreatedAt < toMS {
			flow.TasksCreated++
		}
		if r.UpdatedAt < fromMS || r.UpdatedAt >= toMS {
			continue
		}
		switch r.Status {
		case string(core.StatusDone):
			flow.TasksDone++
			if r.UpdatedAt >= r.CreatedAt {
				taskDurationsMS = append(taskDurationsMS, r.UpdatedAt-r.CreatedAt)
			}
		case string(core.StatusFailed):
			flow.TasksFailed++
		}
	}
	flow.MedianTaskHours = percentileMS(taskDurationsMS, 0.5) / msPerHour

	waits, err := s.attentionWaitsMS(ctx, placeholders, nodeArgs, fromMS, toMS)
	if err != nil {
		return FlowStats{}, err
	}
	flow.AttentionWaitP50Minutes = percentileMS(waits, 0.50) / msPerMinute
	flow.AttentionWaitP95Minutes = percentileMS(waits, 0.95) / msPerMinute
	return flow, nil
}

// accumulatePR folds one node's meta into running PR counts. A non-empty pr_url
// counts as opened; a merged marker (pr_merged or pr_merged_at) additionally
// counts as merged. Invalid/opaque meta is ignored.
func accumulatePR(meta string, opened, merged int) (int, int) {
	if meta == "" {
		return opened, merged
	}
	pm, err := core.UnmarshalPayload[prMeta](meta)
	if err != nil || pm.PRURL == "" {
		return opened, merged
	}
	opened++
	if pm.PRMerged || pm.PRMergedAt != "" {
		merged++
	}
	return opened, merged
}

// attentionWaitsMS returns, for each acknowledged attention event in scope whose
// created_at is in range, the wait in milliseconds between the event and its ack.
func (s *Store) attentionWaitsMS(
	ctx context.Context, placeholders string, nodeArgs []any, fromMS, toMS int64,
) ([]int64, error) {
	//nolint:gosec // G202: placeholders is only "?," separators; every id is a bound parameter passed via args.
	query := `
SELECT acked_at - created_at
FROM events
WHERE requires_attention = 1 AND acked_at IS NOT NULL
	AND node_id IN (` + placeholders + `) AND created_at >= ? AND created_at < ?`
	args := argsNodesFirst(nodeArgs, fromMS, toMS)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("stats attention waits: %w", err)
	}
	waits, err := collect(rows, scanInt64)
	if err != nil {
		return nil, fmt.Errorf("stats attention waits: %w", err)
	}
	return waits, nil
}

// statsToolEventRow is one tool_call/tool_result event's type and payload.
type statsToolEventRow struct {
	Type    string
	Payload string
}

func scanStatsToolEventRow(row rowScanner) (statsToolEventRow, error) {
	var r statsToolEventRow
	if err := row.Scan(&r.Type, &r.Payload); err != nil {
		return statsToolEventRow{}, fmt.Errorf("scan stats tool event row: %w", err)
	}
	return r, nil
}

// statsToolsAndSkills counts per-tool calls (tool_call) and errors (tool_result
// with ok=false), and per-skill invocations (tool_call where the tool is
// "Skill", keyed by its input summary).
func (s *Store) statsToolsAndSkills(
	ctx context.Context, placeholders string, nodeArgs []any, from, to time.Time,
) ([]ToolStat, []SkillStat, error) {
	//nolint:gosec // G202: placeholders is only "?," separators; every id is a bound parameter passed via args.
	query := `
SELECT type, payload
FROM events
WHERE type IN (?, ?) AND created_at >= ? AND created_at < ? AND node_id IN (` + placeholders + `)`
	args := argsNodesLast(nodeArgs,
		string(core.EventToolCall), string(core.EventToolResult), from.UnixMilli(), to.UnixMilli())
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("stats tools: %w", err)
	}
	toolRows, err := collect(rows, scanStatsToolEventRow)
	if err != nil {
		return nil, nil, fmt.Errorf("stats tools: %w", err)
	}

	tools := map[string]*ToolStat{}
	skills := map[string]int{}
	for _, r := range toolRows {
		switch r.Type {
		case string(core.EventToolCall):
			p, err := core.UnmarshalPayload[core.ToolCallPayload](r.Payload)
			if err != nil {
				return nil, nil, fmt.Errorf("stats tools: %w", err)
			}
			toolStat(tools, p.Name).Calls++
			if p.Name == skillToolName {
				skills[skillKey(p.InputSummary)]++
			}
		case string(core.EventToolResult):
			p, err := core.UnmarshalPayload[core.ToolResultPayload](r.Payload)
			if err != nil {
				return nil, nil, fmt.Errorf("stats tools: %w", err)
			}
			if !p.OK {
				toolStat(tools, p.Name).Errors++
			}
		}
	}
	return sortedTools(tools), sortedSkills(skills), nil
}

// skillToolName is the tool name whose calls are counted as skill invocations.
const skillToolName = "Skill"

// skillKey resolves the skill identifier from a Skill tool call's input summary,
// falling back to the tool name when the summary is empty.
func skillKey(inputSummary string) string {
	if inputSummary == "" {
		return skillToolName
	}
	return inputSummary
}

// toolStat returns the (lazily created) ToolStat for name.
func toolStat(m map[string]*ToolStat, name string) *ToolStat {
	t := m[name]
	if t == nil {
		t = &ToolStat{Name: name}
		m[name] = t
	}
	return t
}

// scanInt64 scans a single INTEGER column.
func scanInt64(row rowScanner) (int64, error) {
	var v int64
	if err := row.Scan(&v); err != nil {
		return 0, fmt.Errorf("scan int64: %w", err)
	}
	return v, nil
}

// dayFromMS formats a unix-millisecond timestamp as a UTC calendar day.
func dayFromMS(ms int64) string { return timeFromMS(ms).Format(dayLayout) }

// percentileMS returns the p-quantile (p in [0,1]) of the millisecond durations
// using linear interpolation between the two closest ranks (NumPy's default
// method). An empty input is 0.
func percentileMS(durations []int64, p float64) float64 {
	if len(durations) == 0 {
		return 0
	}
	sorted := make([]int64, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	if len(sorted) == 1 {
		return float64(sorted[0])
	}
	rank := p * float64(len(sorted)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo < 0 {
		lo = 0
	}
	if hi > len(sorted)-1 {
		hi = len(sorted) - 1
	}
	return float64(sorted[lo]) + (rank-float64(lo))*float64(sorted[hi]-sorted[lo])
}

func sortedTokenDays(m map[string]*TokenDay) []TokenDay {
	out := make([]TokenDay, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out
}

func sortedTokenDrivers(m map[string]*TokenDriver) []TokenDriver {
	out := make([]TokenDriver, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		li, lj := out[i].Input+out[i].Output, out[j].Input+out[j].Output
		if li != lj {
			return li > lj
		}
		return out[i].Driver < out[j].Driver
	})
	return out
}

func sortedTokenProfiles(m map[string]*TokenProfile) []TokenProfile {
	out := make([]TokenProfile, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		li, lj := out[i].Input+out[i].Output, out[j].Input+out[j].Output
		if li != lj {
			return li > lj
		}
		return out[i].ProfileID < out[j].ProfileID
	})
	return out
}

func topTokenNodes(m map[string]*TokenNode, limit int) []TokenNode {
	out := make([]TokenNode, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		li, lj := out[i].Input+out[i].Output, out[j].Input+out[j].Output
		if li != lj {
			return li > lj
		}
		return out[i].NodeID < out[j].NodeID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func sortedModels(m map[string]*ModelStat) []ModelStat {
	out := make([]ModelStat, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		li, lj := out[i].Input+out[i].Output, out[j].Input+out[j].Output
		if li != lj {
			return li > lj
		}
		return out[i].Model < out[j].Model
	})
	return out
}

func sortedSessionDays(m map[string]*SessionDay) []SessionDay {
	out := make([]SessionDay, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out
}

func sortedDriverCounts(m map[string]int) []DriverCount {
	out := make([]DriverCount, 0, len(m))
	for driver, count := range m {
		out = append(out, DriverCount{Driver: driver, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Driver < out[j].Driver
	})
	return out
}

func sortedTools(m map[string]*ToolStat) []ToolStat {
	out := make([]ToolStat, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Calls != out[j].Calls {
			return out[i].Calls > out[j].Calls
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func sortedSkills(m map[string]int) []SkillStat {
	out := make([]SkillStat, 0, len(m))
	for skill, n := range m {
		out = append(out, SkillStat{Skill: skill, Invocations: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Invocations != out[j].Invocations {
			return out[i].Invocations > out[j].Invocations
		}
		return out[i].Skill < out[j].Skill
	})
	return out
}
