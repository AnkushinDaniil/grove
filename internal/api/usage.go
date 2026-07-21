package api

import (
	"context"
	"net/http"
	"time"
)

const (
	usageWindow5h   = "5h"
	usageWindowWeek = "week"
	window5hSpan    = 5 * time.Hour
	windowWeekSpan  = 7 * 24 * time.Hour
)

// usageResponse is the GET /usage body: one entry per (profile, driver) with
// any usage in the window, or an empty list.
type usageResponse struct {
	Profiles []usageWindowDTO `json:"profiles"`
}

// usageWindowDTO is the frozen UsageWindow wire shape (docs/API.md).
// utilization is null until a plan-limit model exists (the UI renders token
// counts instead of a bar); cache_read_tokens is 0 until the source payload
// carries it; resets_at/cooldown_until are omitted until rate-limit detection
// lands.
type usageWindowDTO struct {
	ProfileID       string   `json:"profile_id"`
	Name            string   `json:"name"`
	Driver          string   `json:"driver"`
	Window          string   `json:"window"`
	WindowStart     string   `json:"window_start"`
	WindowEnd       string   `json:"window_end"`
	InputTokens     int64    `json:"input_tokens"`
	OutputTokens    int64    `json:"output_tokens"`
	CacheReadTokens int64    `json:"cache_read_tokens"`
	CostUSD         float64  `json:"cost_usd"`
	Utilization     *float64 `json:"utilization"`
}

// handleUsage sums usage_rollup over the requested rolling window (5h or week)
// and returns one UsageWindow per (profile, driver). An unknown window → 400;
// no usage in the window → {"profiles": []}.
func (h *Handlers) handleUsage(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	if window == "" {
		window = usageWindow5h
	}
	var span time.Duration
	switch window {
	case usageWindow5h:
		span = window5hSpan
	case usageWindowWeek:
		span = windowWeekSpan
	default:
		writeErrorStatus(w, h.logger, http.StatusBadRequest, "window must be 5h or week")
		return
	}

	to := time.Now()
	from := to.Add(-span)
	rows, err := h.store.QueryUsageWindow(r.Context(), from, to)
	if err != nil {
		writeError(w, h.logger, err)
		return
	}
	names, err := h.profileNames(r.Context())
	if err != nil {
		writeError(w, h.logger, err)
		return
	}

	profiles := make([]usageWindowDTO, 0, len(rows))
	for _, row := range rows {
		profiles = append(profiles, usageWindowDTO{
			ProfileID:    row.ProfileID,
			Name:         profileName(names, row.ProfileID),
			Driver:       row.Driver,
			Window:       window,
			WindowStart:  rfc3339(from),
			WindowEnd:    rfc3339(to),
			InputTokens:  row.InputTokens,
			OutputTokens: row.OutputTokens,
			CostUSD:      row.CostUSD,
			Utilization:  nil,
		})
	}
	writeJSON(w, h.logger, http.StatusOK, usageResponse{Profiles: profiles})
}

// profileNames maps profile id → display name for usage attribution.
func (h *Handlers) profileNames(ctx context.Context) (map[string]string, error) {
	profiles, err := h.store.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(profiles))
	for _, p := range profiles {
		names[string(p.ID)] = p.Name
	}
	return names, nil
}

// profileName resolves a display name; the empty (inherited) profile and any
// unknown id render as "default".
func profileName(names map[string]string, id string) string {
	if name, ok := names[id]; ok && name != "" {
		return name
	}
	return "default"
}
