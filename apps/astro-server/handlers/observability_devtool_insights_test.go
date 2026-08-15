package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// metricRow is one (day, tags, developer) cell as Langfuse returns it.
func metricRow(date, email string, tags []any, cost float64, tokens, count int) map[string]any {
	return map[string]any{
		langfuseTimeDimensionKey: date + "T00:00:00.000Z",
		"tags":                   tags,
		"userId":                 email,
		"sum_totalCost":          cost,
		"sum_totalTokens":        float64(tokens),
		"count_count":            float64(count),
	}
}

func stubLangfuseMetrics(t *testing.T, rows []map[string]any) *langfuse.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(langfuse.MetricsResponse{Data: rows})
	}))
	t.Cleanup(srv.Close)
	return langfuse.NewClient(srv.URL, "pk", "sk")
}

var ccTags = []any{"claude-code"}

func TestFetchDevtoolUsageFoldsCells(t *testing.T) {
	client := stubLangfuseMetrics(t, []map[string]any{
		metricRow("2026-08-10", "a@x.com", ccTags, 1.50, 100, 3),
		metricRow("2026-08-11", "a@x.com", ccTags, 2.00, 200, 4),
		metricRow("2026-08-11", "b@x.com", ccTags, 0.50, 50, 1),
	})

	usage, err := fetchDevtoolUsage(context.Background(), client, "claude-code",
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("fetchDevtoolUsage: %v", err)
	}

	if got := usage.totals(); got.CostUSD != 4.0 || got.Tokens != 350 || got.Requests != 8 {
		t.Errorf("totals = %+v, want cost 4.0 / tokens 350 / requests 8", got)
	}
	if got := usage.byDay()["2026-08-11"]; got.CostUSD != 2.50 {
		t.Errorf("2026-08-11 cost = %v, want 2.50 (both developers)", got.CostUSD)
	}
	if got := usage.byUser()["a@x.com"]; got.CostUSD != 3.50 || got.Tokens != 300 {
		t.Errorf("a@x.com = %+v, want cost 3.50 / tokens 300 (both days)", got)
	}
}

// Dev-tool traces share the account's Langfuse project with agent traces, so the
// tag is the only thing separating them.
func TestFetchDevtoolUsageFiltersByTag(t *testing.T) {
	client := stubLangfuseMetrics(t, []map[string]any{
		metricRow("2026-08-11", "a@x.com", ccTags, 2.00, 200, 4),
		metricRow("2026-08-11", "a@x.com", []any{"deployment:yzh-vea-3wn"}, 99.00, 9999, 99),
		metricRow("2026-08-11", "a@x.com", []any{"codex"}, 5.00, 500, 5),
	})

	usage, err := fetchDevtoolUsage(context.Background(), client, "claude-code",
		time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("fetchDevtoolUsage: %v", err)
	}
	if got := usage.totals().CostUSD; got != 2.00 {
		t.Errorf("cost = %v, want 2.00 — agent and other-source rows must be excluded", got)
	}
}

// One fetch of the widest window serves every narrower range.
func TestDevtoolUsageSinceSlicesWindow(t *testing.T) {
	usage := devtoolUsage{Cells: []devtoolCell{
		{Date: "2026-08-01", Email: "a@x.com", devtoolBucket: devtoolBucket{CostUSD: 10, Requests: 1}},
		{Date: "2026-08-10", Email: "a@x.com", devtoolBucket: devtoolBucket{CostUSD: 3, Requests: 1}},
		{Date: "2026-08-11", Email: "a@x.com", devtoolBucket: devtoolBucket{CostUSD: 2, Requests: 1}},
	}}

	got := usage.since(time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)).totals()
	if got.CostUSD != 5 || got.Requests != 2 {
		t.Errorf("since(08-10) = %+v, want cost 5 / requests 2", got)
	}
	if all := usage.totals().CostUSD; all != 15 {
		t.Errorf("slicing mutated the source: totals = %v, want 15", all)
	}
}

// Langfuse prices from a model definition, so an unpriced model yields a real
// zero. That must be distinguishable from no usage.
func TestDevtoolUsageCostUnavailable(t *testing.T) {
	unpriced := devtoolUsage{Cells: []devtoolCell{
		{Date: "2026-08-11", devtoolBucket: devtoolBucket{CostUSD: 0, Tokens: 21388, Requests: 2}},
	}}
	if !unpriced.costUnavailable() {
		t.Error("tokens with zero cost should flag costUnavailable")
	}

	priced := devtoolUsage{Cells: []devtoolCell{
		{Date: "2026-08-11", devtoolBucket: devtoolBucket{CostUSD: 0.13, Tokens: 21388, Requests: 2}},
	}}
	if priced.costUnavailable() {
		t.Error("priced usage should not flag costUnavailable")
	}

	if (devtoolUsage{}).costUnavailable() {
		t.Error("no usage at all is not a pricing fault")
	}
}

func TestDevtoolSourceForBuildsBlock(t *testing.T) {
	usage := devtoolUsage{Cells: []devtoolCell{
		{Date: "2026-08-10", Email: "dev@x.com", devtoolBucket: devtoolBucket{CostUSD: 1.5, Tokens: 100, Requests: 3}},
		{Date: "2026-08-11", Email: "dev@x.com", devtoolBucket: devtoolBucket{CostUSD: 2.5, Tokens: 200, Requests: 5}},
	}}
	ad := devtoolAdapter{Key: "claude-code", Label: "Claude Code", Icon: "anthropic"}

	src, ok := devtoolSourceFor(ad, usage, map[string]string{"dev@x.com": "user_1"})
	if !ok {
		t.Fatal("source should be present")
	}
	if src.Totals.CostUSD != 4.0 || src.Totals.Requests != 8 {
		t.Errorf("totals = %+v, want cost 4.0 / requests 8", src.Totals)
	}
	// Requests came from PromQL as a hardcoded zero; Langfuse supplies real counts.
	if src.AgentRow.Metrics.Requests != 8 {
		t.Errorf("agent row requests = %d, want 8", src.AgentRow.Metrics.Requests)
	}
	if len(src.SpendByDay) != 2 || src.SpendByDay[0].Date != "2026-08-10" {
		t.Errorf("spend series = %+v, want two ascending days", src.SpendByDay)
	}
	if src.AgentRow.Metrics.LastSeen != "2026-08-11" {
		t.Errorf("last seen = %q, want 2026-08-11", src.AgentRow.Metrics.LastSeen)
	}
	if len(src.ByUser) != 1 || src.ByUser[0].IdentityKey != "member:user_1" {
		t.Errorf("by-user = %+v, want one member-resolved row", src.ByUser)
	}
}

func TestDevtoolSourceForOmitsUnusedSource(t *testing.T) {
	ad := devtoolAdapter{Key: "claude-code", Label: "Claude Code"}
	if _, ok := devtoolSourceFor(ad, devtoolUsage{}, nil); ok {
		t.Error("a source with no usage must be omitted")
	}
}

// An unpriced model still shows the source — hiding it would lose the row, the
// Sources filter entry, and the token counts.
func TestDevtoolSourceForKeepsUnpricedSource(t *testing.T) {
	usage := devtoolUsage{Cells: []devtoolCell{
		{Date: "2026-08-11", Email: "dev@x.com", devtoolBucket: devtoolBucket{CostUSD: 0, Tokens: 21388, Requests: 2}},
	}}
	src, ok := devtoolSourceFor(devtoolAdapter{Key: "claude-code", Label: "Claude Code"}, usage, nil)
	if !ok {
		t.Fatal("unpriced source must still be present")
	}
	if !src.CostUnavailable {
		t.Error("unpriced source must set CostUnavailable")
	}
	if src.Totals.TotalTokens != 21388 {
		t.Errorf("tokens = %d, want 21388", src.Totals.TotalTokens)
	}
}

func TestComputeDevtoolInsightsGraceful(t *testing.T) {
	log := logger.New("error", "json")
	if got := computeDevtoolInsights(context.Background(), log, nil, "acct", nil, insightsRangeSpecs); len(got.Ranges) != 0 {
		t.Errorf("nil client should yield no ranges, got %v", got.Ranges)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	failing := langfuse.NewClient(srv.URL, "pk", "sk")
	if got := computeDevtoolInsights(context.Background(), log, failing, "acct", nil, insightsRangeSpecs); len(got.Ranges) != 0 {
		t.Errorf("query failure should yield no ranges, got %v", got.Ranges)
	}
}
