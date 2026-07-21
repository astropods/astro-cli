package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
)

// stubProm models the four query shapes computeDevtoolSource issues:
//   - instant window total   sum(increase(m[Nd]))         → one value, captures today
//   - instant today overlay  sum(increase(m[<s>s]))       → one value, today only
//   - instant per-user        sum by ("user.email")(…)    → per-developer vector
//   - daily range             sum(increase(m[1d])) @ 24h  → per-day matrix (may drop today)
//
// The window total (9) deliberately differs from the daily sum (3.5·2 = 7) so the
// test proves totals come from the window query, not the daily buckets; the
// overlay (2) is distinct again so the appended today bucket is recognizable.
func stubProm(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/query") { // instant
			switch {
			case strings.Contains(q, "by ("): // per-user breakdown
				val := "4.0"
				if strings.Contains(q, "token") {
					val = "2000"
				}
				fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"user.email":"dev1@x.com"},"value":[1700000000,"%s"]},{"metric":{"user.email":"dev2@x.com"},"value":[1700000000,"%s"]}]}}`, val, val)
			case strings.Contains(q, "s]"): // today overlay (seconds window)
				fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"2"]}]}}`)
			default: // window total (days window)
				val := "9"
				if strings.Contains(q, "token") {
					val = "12000"
				}
				fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"%s"]}]}}`, val)
			}
			return
		}
		val := "3.5" // cost/day
		if strings.Contains(q, "token") {
			val = "5000" // tokens/day
		}
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1700000000,"%s"],[1700086400,"%s"]]}]}}`, val, val)
	}))
}

func TestComputeDevtoolInsights(t *testing.T) {
	srv := stubProm(t)
	defer srv.Close()

	log := logger.New("error", "json")
	resp := computeDevtoolInsights(context.Background(), log, promquery.NewClient(srv.URL, ""), "acct-123", map[string]string{"dev1@x.com": "user_dev1"}, insightsRangeSpecs)

	if len(resp.Ranges) != len(insightsRangeSpecs) {
		t.Fatalf("expected %d ranges, got %d", len(insightsRangeSpecs), len(resp.Ranges))
	}
	r, ok := resp.Ranges["7d"]
	if !ok {
		t.Fatal("missing 7d range")
	}
	src, ok := r.Sources["claude-code"]
	if !ok {
		t.Fatalf("missing claude-code source; got %v", r.Sources)
	}
	if src.Label != "Claude Code" {
		t.Fatalf("unexpected source label: %q", src.Label)
	}
	// Two historical buckets from the daily range, plus the overlaid today bucket.
	if len(src.SpendByDay) != 3 {
		t.Fatalf("expected 3 spend points (2 daily + today overlay), got %d", len(src.SpendByDay))
	}
	if src.SpendByDay[0].Date != "2023-11-14" || src.SpendByDay[0].CostUSD != 3.5 {
		t.Fatalf("unexpected first spend point: %+v", src.SpendByDay[0])
	}
	if src.SpendByDay[2].CostUSD != 2.0 {
		t.Fatalf("expected overlaid today bucket at 2.0, got %+v", src.SpendByDay[2])
	}
	if src.Totals.CostUSD != 9.0 {
		t.Fatalf("expected window-total cost 9.0 (not the daily sum 7.0), got %v", src.Totals.CostUSD)
	}
	if src.Totals.TotalTokens != 12000 {
		t.Fatalf("expected 12000 window-total tokens, got %d", src.Totals.TotalTokens)
	}
	if src.Totals.Requests != 0 {
		t.Fatalf("expected requests 0 (no request metric), got %d", src.Totals.Requests)
	}
	if src.AgentRow.Identity.Kind != "system" || src.AgentRow.Identity.Label != "Claude Code" || src.AgentRow.Identity.Href != "" {
		t.Fatalf("unexpected agent row identity: %+v", src.AgentRow.Identity)
	}
	if src.AgentRow.Metrics.CostUSD != 9.0 {
		t.Fatalf("expected agent row cost 9.0, got %v", src.AgentRow.Metrics.CostUSD)
	}
	if len(src.ByUser) != 2 {
		t.Fatalf("expected 2 per-user entries, got %d", len(src.ByUser))
	}
	if src.ByUser[0].UserEmail != "dev1@x.com" || src.ByUser[0].CostUSD != 4.0 || src.ByUser[0].TotalTokens != 2000 {
		t.Fatalf("unexpected per-user entry: %+v", src.ByUser[0])
	}
	// dev1 resolves to an account member (email→user_id map) → matchable key; dev2 does not.
	if src.ByUser[0].UserID != "user_dev1" || src.ByUser[0].IdentityKey != "member:user_dev1" {
		t.Fatalf("expected dev1 resolved to member:user_dev1, got %+v", src.ByUser[0])
	}
	if src.ByUser[1].IdentityKey != "" {
		t.Fatalf("expected dev2 (no member match) to have empty identity_key, got %+v", src.ByUser[1])
	}
}

func TestComputeDevtoolInsightsGraceful(t *testing.T) {
	log := logger.New("error", "json")

	// No metrics backend → empty ranges, never a panic.
	nilResp := computeDevtoolInsights(context.Background(), log, nil, "acct", nil, insightsRangeSpecs)
	if nilResp.Ranges == nil || len(nilResp.Ranges) != 0 {
		t.Fatalf("nil client should yield empty ranges, got %+v", nilResp.Ranges)
	}

	// Query error (500) → empty ranges, still 200-safe (no range added).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	errResp := computeDevtoolInsights(context.Background(), log, promquery.NewClient(srv.URL, ""), "acct", nil, insightsRangeSpecs)
	if len(errResp.Ranges) != 0 {
		t.Fatalf("query error should yield empty ranges, got %d", len(errResp.Ranges))
	}
}

// TestComputeDevtoolSourceRecentOnly reproduces the widest-window bug: the daily
// range query returns no buckets (VM drops the current day for wide windows), but
// the instant window total is positive. The source must stay present (else a
// recent-only account loses its tables, Sources filter, and branding), and the
// today overlay must still yield a chart bucket (not a blank line).
func TestComputeDevtoolSourceRecentOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/query") { // instant: window total + today overlay
			val := "5"
			if strings.Contains(q, "token") {
				val = "8000"
			}
			fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"%s"]}]}}`, val)
			return
		}
		// Daily range: empty — the only day with data (today) was dropped.
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[]}}`)
	}))
	defer srv.Close()

	log := logger.New("error", "json")
	resp := computeDevtoolInsights(context.Background(), log, promquery.NewClient(srv.URL, ""), "acct", nil, []insightsRangeSpec{{key: "90d", days: 90}})
	r, ok := resp.Ranges["90d"]
	if !ok {
		t.Fatal("source must be present from the window total even with no daily buckets")
	}
	src := r.Sources["claude-code"]
	if src.Totals.CostUSD != 5 {
		t.Fatalf("expected window-total cost 5, got %v", src.Totals.CostUSD)
	}
	if len(src.SpendByDay) != 1 || src.SpendByDay[0].CostUSD != 5 {
		t.Fatalf("expected one overlaid today bucket at 5, got %+v", src.SpendByDay)
	}
}

func TestApplyTodayBucket(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	const today = "2026-07-21"

	// cost <= 0 → series untouched.
	if got := applyTodayBucket([]DevtoolSpendPoint{{Date: "2026-07-20", CostUSD: 3}}, now, 0); len(got) != 1 {
		t.Fatalf("cost 0 should leave the series unchanged, got %+v", got)
	}

	// today absent → appended.
	got := applyTodayBucket([]DevtoolSpendPoint{{Date: "2026-07-20", CostUSD: 3}}, now, 5)
	if len(got) != 2 || got[1].Date != today || got[1].CostUSD != 5 {
		t.Fatalf("expected an appended today bucket at 5, got %+v", got)
	}

	// today present but smaller → raised to the overlay value.
	got = applyTodayBucket([]DevtoolSpendPoint{{Date: today, CostUSD: 2}}, now, 5)
	if len(got) != 1 || got[0].CostUSD != 5 {
		t.Fatalf("expected today raised to 5, got %+v", got)
	}

	// today present and larger → kept (overlay must not drop data).
	got = applyTodayBucket([]DevtoolSpendPoint{{Date: today, CostUSD: 8}}, now, 5)
	if len(got) != 1 || got[0].CostUSD != 8 {
		t.Fatalf("expected today kept at 8 (max), got %+v", got)
	}
}
