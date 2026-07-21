package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
)

// stubProm returns a 2-point daily matrix for every range query; cost and token
// queries are distinguished by the metric name in the query string.
func stubProm(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		// Per-user queries are instant (vector); daily-spend queries are range (matrix).
		if strings.HasSuffix(r.URL.Path, "/query") {
			val := "4.0" // per-user cost
			if strings.Contains(q, "token") {
				val = "2000" // per-user tokens
			}
			fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"user.email":"dev1@x.com"},"value":[1700000000,"%s"]},{"metric":{"user.email":"dev2@x.com"},"value":[1700000000,"%s"]}]}}`, val, val)
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
	if len(src.SpendByDay) != 2 {
		t.Fatalf("expected 2 spend points, got %d", len(src.SpendByDay))
	}
	if src.SpendByDay[0].Date != "2023-11-14" || src.SpendByDay[0].CostUSD != 3.5 {
		t.Fatalf("unexpected first spend point: %+v", src.SpendByDay[0])
	}
	if src.Totals.CostUSD != 7.0 {
		t.Fatalf("expected total cost 7.0, got %v", src.Totals.CostUSD)
	}
	if src.Totals.TotalTokens != 10000 {
		t.Fatalf("expected 10000 tokens, got %d", src.Totals.TotalTokens)
	}
	if src.Totals.Requests != 0 {
		t.Fatalf("expected requests 0 (no request metric), got %d", src.Totals.Requests)
	}
	if src.AgentRow.Identity.Kind != "system" || src.AgentRow.Identity.Label != "Claude Code" || src.AgentRow.Identity.Href != "" {
		t.Fatalf("unexpected agent row identity: %+v", src.AgentRow.Identity)
	}
	if src.AgentRow.Metrics.CostUSD != 7.0 {
		t.Fatalf("expected agent row cost 7.0, got %v", src.AgentRow.Metrics.CostUSD)
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
