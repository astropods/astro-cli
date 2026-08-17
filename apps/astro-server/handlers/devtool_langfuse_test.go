package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
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
