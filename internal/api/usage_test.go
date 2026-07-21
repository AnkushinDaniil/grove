package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/AnkushinDaniil/grove/internal/core"
	"github.com/AnkushinDaniil/grove/internal/store"
)

// bucketFor truncates t to its 5-minute rollup bucket start in unix ms.
func bucketFor(t time.Time) int64 {
	const bucketMS = int64(5 * 60 * 1000)
	ms := t.UnixMilli()
	return ms - (ms % bucketMS)
}

func seedProfile(t *testing.T, h *harness, id, name string) {
	t.Helper()
	p := core.Profile{
		ID: core.ProfileID(id), Driver: "claude", Name: name,
		ConfigDir: "/tmp/" + id, CreatedAt: time.Now(),
	}
	if err := h.store.SaveProfile(h.t.Context(), p); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
}

func seedBuckets(t *testing.T, h *harness, buckets []store.UsageBucket) {
	t.Helper()
	if err := h.store.UpsertUsageBuckets(h.t.Context(), buckets); err != nil {
		t.Fatalf("UpsertUsageBuckets: %v", err)
	}
}

func getUsage(t *testing.T, h *harness, query string) usageResponse {
	t.Helper()
	var body usageResponse
	h.decode(h.do(http.MethodGet, "/api/v1/usage"+query, nil), http.StatusOK, &body)
	return body
}

func TestUsageEndpointReturnsAggregatedData(t *testing.T) {
	h := newHarness(t, nil)
	seedProfile(t, h, "prof-1", "personal")

	recent := bucketFor(time.Now().Add(-10 * time.Minute))
	seedBuckets(t, h, []store.UsageBucket{
		{BucketStart: recent, ProfileID: "prof-1", Driver: "claude", Model: "sonnet", InputTokens: 100, OutputTokens: 40, CostUSD: 0.5},
	})

	body := getUsage(t, h, "?window=5h")
	if len(body.Profiles) != 1 {
		t.Fatalf("profiles = %d, want 1", len(body.Profiles))
	}
	p := body.Profiles[0]
	if p.ProfileID != "prof-1" || p.Name != "personal" || p.Driver != "claude" {
		t.Errorf("attribution = %s/%s/%s, want prof-1/personal/claude", p.ProfileID, p.Name, p.Driver)
	}
	if p.InputTokens != 100 || p.OutputTokens != 40 {
		t.Errorf("tokens = %d/%d, want 100/40", p.InputTokens, p.OutputTokens)
	}
	if p.CostUSD < 0.4999 || p.CostUSD > 0.5001 {
		t.Errorf("cost = %v, want ~0.5", p.CostUSD)
	}
	if p.Window != "5h" {
		t.Errorf("window = %q, want 5h", p.Window)
	}
	if p.Utilization != nil {
		t.Errorf("utilization = %v, want null", *p.Utilization)
	}
	if p.WindowStart == "" || p.WindowEnd == "" {
		t.Errorf("window bounds missing: start=%q end=%q", p.WindowStart, p.WindowEnd)
	}
}

// TestUsageEndpointWindowParam verifies the rolling window boundary: a bucket
// 6h old is outside the 5h window but inside the week window, and the default
// (no param) is 5h.
func TestUsageEndpointWindowParam(t *testing.T) {
	h := newHarness(t, nil)
	seedProfile(t, h, "prof-1", "personal")

	recent := bucketFor(time.Now().Add(-10 * time.Minute))
	old := bucketFor(time.Now().Add(-6 * time.Hour))
	seedBuckets(t, h, []store.UsageBucket{
		{BucketStart: recent, ProfileID: "prof-1", Driver: "claude", InputTokens: 10},
		{BucketStart: old, ProfileID: "prof-1", Driver: "claude", InputTokens: 100},
	})

	// 5h window sees only the recent bucket.
	if got := getUsage(t, h, "?window=5h").Profiles; len(got) != 1 || got[0].InputTokens != 10 {
		t.Fatalf("5h profiles = %+v, want one row with 10", got)
	}
	// No window param defaults to 5h.
	if got := getUsage(t, h, "").Profiles; len(got) != 1 || got[0].InputTokens != 10 {
		t.Fatalf("default profiles = %+v, want one row with 10 (5h default)", got)
	}
	// Week window sums both buckets.
	weekProfiles := getUsage(t, h, "?window=week").Profiles
	if len(weekProfiles) != 1 || weekProfiles[0].InputTokens != 110 {
		t.Fatalf("week profiles = %+v, want one row with 110", weekProfiles)
	}
	if weekProfiles[0].Window != "week" {
		t.Errorf("window = %q, want week", weekProfiles[0].Window)
	}
}

// TestUsageEndpointDefaultProfileName maps the empty (inherited) profile to the
// "default" display name.
func TestUsageEndpointDefaultProfileName(t *testing.T) {
	h := newHarness(t, nil)
	recent := bucketFor(time.Now().Add(-10 * time.Minute))
	seedBuckets(t, h, []store.UsageBucket{
		{BucketStart: recent, ProfileID: "", Driver: "claude", InputTokens: 5},
	})

	profiles := getUsage(t, h, "?window=5h").Profiles
	if len(profiles) != 1 || profiles[0].Name != "default" || profiles[0].ProfileID != "" {
		t.Fatalf("profiles = %+v, want one default-named row with empty profile id", profiles)
	}
}

func TestUsageEndpointBadWindow(t *testing.T) {
	h := newHarness(t, nil)
	h.decode(h.do(http.MethodGet, "/api/v1/usage?window=year", nil), http.StatusBadRequest, nil)
}

func TestUsageEndpointEmpty(t *testing.T) {
	h := newHarness(t, nil)
	body := getUsage(t, h, "?window=5h")
	if body.Profiles == nil {
		t.Fatal("profiles = null, want empty array")
	}
	if len(body.Profiles) != 0 {
		t.Errorf("profiles = %d, want 0", len(body.Profiles))
	}
}
