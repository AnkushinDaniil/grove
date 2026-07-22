package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/store"
)

// Stats range windows (docs/API.md GET /stats). An omitted range defaults to 7d;
// any other value is a 400.
const (
	statsRange24h = "24h"
	statsRange7d  = "7d"
	statsRange30d = "30d"
)

// statsCacheTTL is how long a computed stats payload is served from memory before
// being recomputed (docs/API.md: "cache the result 60s").
const statsCacheTTL = 60 * time.Second

// statsResponse is the GET /stats body — the frozen draft shape from docs/API.md.
type statsResponse struct {
	Range    string            `json:"range"`
	Scope    string            `json:"scope"`
	Tokens   tokensDTO         `json:"tokens"`
	Agents   agentsDTO         `json:"agents"`
	Flow     flowDTO           `json:"flow"`
	Tools    []toolStatDTO     `json:"tools"`
	Models   []modelStatDTO    `json:"models"`
	Skills   []skillStatDTO    `json:"skills"`
	Feedback []feedbackStatDTO `json:"feedback"`
}

type tokenTotalDTO struct {
	Input     int64   `json:"input"`
	Output    int64   `json:"output"`
	CacheRead int64   `json:"cache_read"`
	CostUSD   float64 `json:"cost_usd"`
}

type tokenDayDTO struct {
	Day     string  `json:"day"`
	Input   int64   `json:"input"`
	Output  int64   `json:"output"`
	CostUSD float64 `json:"cost_usd"`
}

type tokenDriverDTO struct {
	Driver  string  `json:"driver"`
	Input   int64   `json:"input"`
	Output  int64   `json:"output"`
	CostUSD float64 `json:"cost_usd"`
}

type tokenProfileDTO struct {
	ProfileID string  `json:"profile_id"`
	Name      string  `json:"name"`
	Input     int64   `json:"input"`
	Output    int64   `json:"output"`
	CostUSD   float64 `json:"cost_usd"`
}

type tokenNodeDTO struct {
	NodeID  string  `json:"node_id"`
	Title   string  `json:"title"`
	Input   int64   `json:"input"`
	Output  int64   `json:"output"`
	CostUSD float64 `json:"cost_usd"`
}

type tokensDTO struct {
	Total     tokenTotalDTO     `json:"total"`
	ByDay     []tokenDayDTO     `json:"by_day"`
	ByDriver  []tokenDriverDTO  `json:"by_driver"`
	ByProfile []tokenProfileDTO `json:"by_profile"`
	TopNodes  []tokenNodeDTO    `json:"top_nodes"`
}

type sessionDayDTO struct {
	Day     string `json:"day"`
	Started int    `json:"started"`
	Done    int    `json:"done"`
	Failed  int    `json:"failed"`
}

type driverCountDTO struct {
	Driver string `json:"driver"`
	Count  int    `json:"count"`
}

type agentsDTO struct {
	SessionsActive    int              `json:"sessions_active"`
	SessionsByDay     []sessionDayDTO  `json:"sessions_by_day"`
	AvgSessionMinutes float64          `json:"avg_session_minutes"`
	ByDriver          []driverCountDTO `json:"by_driver"`
}

type flowDTO struct {
	TasksCreated            int     `json:"tasks_created"`
	TasksDone               int     `json:"tasks_done"`
	TasksFailed             int     `json:"tasks_failed"`
	MedianTaskHours         float64 `json:"median_task_hours"`
	AttentionWaitP50Minutes float64 `json:"attention_wait_p50_minutes"`
	AttentionWaitP95Minutes float64 `json:"attention_wait_p95_minutes"`
	PRsOpened               int     `json:"prs_opened"`
	PRsMerged               int     `json:"prs_merged"`
}

type toolStatDTO struct {
	Name   string `json:"name"`
	Calls  int    `json:"calls"`
	Errors int    `json:"errors"`
}

type modelStatDTO struct {
	Model   string  `json:"model"`
	Input   int64   `json:"input"`
	Output  int64   `json:"output"`
	CostUSD float64 `json:"cost_usd"`
}

type skillStatDTO struct {
	Skill       string `json:"skill"`
	Invocations int    `json:"invocations"`
}

type feedbackStatDTO struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Open    int    `json:"open"`
	Total   int    `json:"total"`
}

// handleStats aggregates usage/session/flow metrics over a scope subtree and time
// range, entirely from the local DB. range is 24h|7d|30d (default 7d; else 400);
// scope defaults to the workspace root and 400s if the node is unknown. Results
// are cached in memory for statsCacheTTL, keyed by scope+range.
func (h *Handlers) handleStats(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = statsRange7d
	}
	span, ok := statsSpan(rng)
	if !ok {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "range must be 24h, 7d, or 30d")
		return
	}

	scopeID, ok := h.resolveStatsScope(w, r)
	if !ok {
		return
	}

	key := string(scopeID) + "|" + rng
	if cached, hit := h.stats.get(key); hit {
		writeJSON(w, h.logger, http.StatusOK, cached)
		return
	}

	subtree := h.tree.SubtreeIDs(scopeID)
	to := time.Now()
	from := to.Add(-span)
	data, err := h.store.Stats(r.Context(), subtree, from, to)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	names, err := h.profileNames(r.Context())
	if err != nil {
		writeError(w, h.logger, err)
		return
	}

	resp := h.statsToResponse(rng, string(scopeID), data, names)
	h.stats.put(key, resp)
	writeJSON(w, h.logger, http.StatusOK, resp)
}

// resolveStatsScope returns the scope node id, defaulting to the workspace root.
// It writes the error response and returns ok=false on a missing root (500) or an
// unknown scope node (400).
func (h *Handlers) resolveStatsScope(w http.ResponseWriter, r *http.Request) (core.NodeID, bool) {
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		root, ok := h.tree.Root()
		if !ok {
			writeErrorStatus(w, h.logger, http.StatusInternalServerError, "workspace root not found")
			return "", false
		}
		return root.ID, true
	}
	scopeID := core.NodeID(scope)
	if _, ok := h.tree.Get(scopeID); !ok {
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "unknown scope node")
		return "", false
	}
	return scopeID, true
}

// statsSpan maps a range string to its rolling window duration.
func statsSpan(rng string) (time.Duration, bool) {
	switch rng {
	case statsRange24h:
		return 24 * time.Hour, true
	case statsRange7d:
		return 7 * 24 * time.Hour, true
	case statsRange30d:
		return 30 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

// statsToResponse maps the store aggregation to the wire shape, decorating
// profile display names and node titles (which the store leaves to the API).
func (h *Handlers) statsToResponse(rng, scope string, data store.StatsResult, names map[string]string) statsResponse {
	return statsResponse{
		Range:    rng,
		Scope:    scope,
		Tokens:   h.tokensToDTO(data.Tokens, names),
		Agents:   agentsToDTO(data.Agents),
		Flow:     flowToDTO(data.Flow),
		Tools:    toolsToDTO(data.Tools),
		Models:   modelsToDTO(data.Models),
		Skills:   skillsToDTO(data.Skills),
		Feedback: feedbackStatsToDTO(data.Feedback),
	}
}

func (h *Handlers) tokensToDTO(t store.TokenStats, names map[string]string) tokensDTO {
	byDay := make([]tokenDayDTO, 0, len(t.ByDay))
	for _, d := range t.ByDay {
		byDay = append(byDay, tokenDayDTO{Day: d.Day, Input: d.Input, Output: d.Output, CostUSD: d.CostUSD})
	}
	byDriver := make([]tokenDriverDTO, 0, len(t.ByDriver))
	for _, d := range t.ByDriver {
		byDriver = append(byDriver, tokenDriverDTO{Driver: d.Driver, Input: d.Input, Output: d.Output, CostUSD: d.CostUSD})
	}
	byProfile := make([]tokenProfileDTO, 0, len(t.ByProfile))
	for _, p := range t.ByProfile {
		byProfile = append(byProfile, tokenProfileDTO{
			ProfileID: p.ProfileID,
			Name:      profileName(names, p.ProfileID),
			Input:     p.Input,
			Output:    p.Output,
			CostUSD:   p.CostUSD,
		})
	}
	topNodes := make([]tokenNodeDTO, 0, len(t.TopNodes))
	for _, n := range t.TopNodes {
		topNodes = append(topNodes, tokenNodeDTO{
			NodeID:  n.NodeID,
			Title:   h.nodeTitle(n.NodeID),
			Input:   n.Input,
			Output:  n.Output,
			CostUSD: n.CostUSD,
		})
	}
	return tokensDTO{
		Total: tokenTotalDTO{
			Input:     t.Total.Input,
			Output:    t.Total.Output,
			CacheRead: t.Total.CacheRead,
			CostUSD:   t.Total.CostUSD,
		},
		ByDay:     byDay,
		ByDriver:  byDriver,
		ByProfile: byProfile,
		TopNodes:  topNodes,
	}
}

// nodeTitle resolves a node's title from the live tree, or "" if it is gone.
func (h *Handlers) nodeTitle(id string) string {
	if n, ok := h.tree.Get(core.NodeID(id)); ok {
		return n.Title
	}
	return ""
}

func agentsToDTO(a store.AgentStats) agentsDTO {
	byDay := make([]sessionDayDTO, 0, len(a.SessionsByDay))
	for _, d := range a.SessionsByDay {
		byDay = append(byDay, sessionDayDTO{Day: d.Day, Started: d.Started, Done: d.Done, Failed: d.Failed})
	}
	byDriver := make([]driverCountDTO, 0, len(a.ByDriver))
	for _, d := range a.ByDriver {
		byDriver = append(byDriver, driverCountDTO{Driver: d.Driver, Count: d.Count})
	}
	return agentsDTO{
		SessionsActive:    a.SessionsActive,
		SessionsByDay:     byDay,
		AvgSessionMinutes: a.AvgSessionMinutes,
		ByDriver:          byDriver,
	}
}

func flowToDTO(f store.FlowStats) flowDTO {
	return flowDTO{
		TasksCreated:            f.TasksCreated,
		TasksDone:               f.TasksDone,
		TasksFailed:             f.TasksFailed,
		MedianTaskHours:         f.MedianTaskHours,
		AttentionWaitP50Minutes: f.AttentionWaitP50Minutes,
		AttentionWaitP95Minutes: f.AttentionWaitP95Minutes,
		PRsOpened:               f.PRsOpened,
		PRsMerged:               f.PRsMerged,
	}
}

func toolsToDTO(tools []store.ToolStat) []toolStatDTO {
	out := make([]toolStatDTO, 0, len(tools))
	for _, t := range tools {
		out = append(out, toolStatDTO{Name: t.Name, Calls: t.Calls, Errors: t.Errors})
	}
	return out
}

func modelsToDTO(models []store.ModelStat) []modelStatDTO {
	out := make([]modelStatDTO, 0, len(models))
	for _, m := range models {
		out = append(out, modelStatDTO{Model: m.Model, Input: m.Input, Output: m.Output, CostUSD: m.CostUSD})
	}
	return out
}

func skillsToDTO(skills []store.SkillStat) []skillStatDTO {
	out := make([]skillStatDTO, 0, len(skills))
	for _, s := range skills {
		out = append(out, skillStatDTO{Skill: s.Skill, Invocations: s.Invocations})
	}
	return out
}

func feedbackStatsToDTO(stats []store.FeedbackStat) []feedbackStatDTO {
	out := make([]feedbackStatDTO, 0, len(stats))
	for _, f := range stats {
		out = append(out, feedbackStatDTO{Kind: f.Kind, Subject: f.Subject, Open: f.Open, Total: f.Total})
	}
	return out
}

// statsCache is the tiny in-memory result guard for GET /stats: one entry per
// scope+range key, served for up to ttl before recomputation.
type statsCache struct {
	ttl     time.Duration
	mu      sync.Mutex
	entries map[string]statsCacheEntry
}

type statsCacheEntry struct {
	at      time.Time
	payload statsResponse
}

func newStatsCache(ttl time.Duration) *statsCache {
	return &statsCache{ttl: ttl, entries: make(map[string]statsCacheEntry)}
}

func (c *statsCache) get(key string) (statsResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Since(e.at) > c.ttl {
		return statsResponse{}, false
	}
	return e.payload, true
}

func (c *statsCache) put(key string, payload statsResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = statsCacheEntry{at: time.Now(), payload: payload}
}
