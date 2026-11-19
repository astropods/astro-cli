package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insightsPayload is a trimmed Insights page model: one 7-day range, two agents,
// and one person.
func insightsPayload() map[string]any {
	return map[string]any{
		"as_of": "2026-11-16",
		"ranges": map[string]any{
			"7d": map[string]any{
				"days":   7,
				"period": map[string]any{"start": "2026-11-10T00:00:00Z", "end": "2026-11-16T23:59:59.999Z", "days": 7},
				"stat_cards": map[string]any{
					"totals": map[string]any{"cost_usd": 4.5, "requests": 9, "total_tokens": 990, "active_agents": 2},
					"change": map[string]any{"cost_pct": 12.5, "requests_pct": nil, "tokens_pct": -3.0},
				},
				"agent_spend_chart": []any{
					map[string]any{"date": "2026-11-15", "models": []any{
						map[string]any{"model": "dep-1", "cost_usd": 3.0},
						map[string]any{"model": "dep-2", "cost_usd": 0.0},
					}},
					map[string]any{"date": "2026-11-16", "models": []any{
						map[string]any{"model": "dep-1", "cost_usd": 1.0},
						map[string]any{"model": "dep-2", "cost_usd": 0.5},
					}},
				},
				"series_labels": map[string]any{"dep-1": "sasbot", "dep-2": "triage"},
			},
		},
		"tables": map[string]any{
			"agents": map[string]any{
				"rows": []any{
					map[string]any{
						"identity": map[string]any{"kind": "agent", "id": "dep-1", "label": "sasbot"},
						"used_by":  []any{map[string]any{"label": "Ada"}},
						"metrics":  map[string]any{"requests": 8, "cost_usd": 4.0, "cost_pct": 88.9, "cost_per_request": 0.5, "tok_per_request": 112.5, "last_seen": "2026-11-16"},
					},
				},
				"total_cost": 4.5,
				"count":      2,
				"pagination": map[string]any{"limit": 1, "offset": 0, "has_more": true},
			},
			"people": map[string]any{
				"rows": []any{
					map[string]any{
						"identity":    map[string]any{"kind": "slack", "label": "Ada"},
						"agents_used": []any{map[string]any{"label": "sasbot"}},
						"metrics":     map[string]any{"requests": 9, "cost_usd": 4.5, "cost_pct": 100.0, "tokens": 990, "last_seen": "2026-11-16"},
					},
				},
				"total_cost": 4.5,
				"count":      1,
				"pagination": map[string]any{"limit": 25, "offset": 0, "has_more": false},
			},
		},
	}
}

// setupInsightsTest serves body and records each request's query.
func setupInsightsTest(t *testing.T, status int, body any) *[]url.Values {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("testaccount"))

	var queries []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/accounts/testaccount/insights", r.URL.Path)
		queries = append(queries, r.URL.Query())
		jsonHandler(status, body)(w, r)
	}))
	t.Cleanup(srv.Close)
	insightsServerURLOverride = srv.URL
	t.Cleanup(func() { insightsServerURLOverride = "" })
	return &queries
}

// runInsights sets flags on c, runs it, and restores the flags' defaults.
func runInsights(t *testing.T, c *cobra.Command, flags map[string]string) (string, error) {
	t.Helper()
	for name, value := range flags {
		require.NoError(t, c.Flags().Set(name, value))
	}
	t.Cleanup(func() {
		for name := range flags {
			f := c.Flags().Lookup(name)
			f.Value.Set(f.DefValue) //nolint:errcheck,gosec
			f.Changed = false
		}
	})
	buf := &bytes.Buffer{}
	c.SetOut(buf)
	c.SetContext(context.Background())
	err := c.RunE(c, nil)
	return buf.String(), err
}

func TestInsightsCommands(t *testing.T) {
	cases := []struct {
		name      string
		cmd       *cobra.Command
		flags     map[string]string
		wantQuery url.Values
		wantOut   []string
	}{
		{
			name:      "summary",
			cmd:       insightsSummaryCmd,
			flags:     map[string]string{"days": "7"},
			wantQuery: url.Values{"days": {"7"}},
			wantOut: []string{
				"testaccount", msgInsightsWindow(7, "2026-11-10", "2026-11-16"),
				msgUsageDollars(4.5), "9", "990",
				msgInsightsChange(ptrFloat(12.5)), msgInsightsChange(ptrFloat(-3.0)),
			},
		},
		{
			name:      "daily labels each agent",
			cmd:       insightsDailyCmd,
			flags:     map[string]string{"days": "7"},
			wantQuery: url.Values{"days": {"7"}},
			wantOut: []string{
				"2026-11-15", "sasbot " + msgUsageDollars(3.0),
				"2026-11-16", msgUsageDollars(1.5), "triage " + msgUsageDollars(0.5),
				msgInsightsDailyTopFive(),
			},
		},
		{
			name:      "agents pages with limit and offset",
			cmd:       insightsAgentsCmd,
			flags:     map[string]string{"days": "7", "limit": "1"},
			wantQuery: url.Values{"days": {"7"}, "agents_limit": {"1"}, "agents_offset": {"0"}},
			wantOut:   []string{"sasbot", msgUsageDollars(4.0), "88.9%", msgUsageDollars(4.5), msgInsightsMore(1, 2)},
		},
		{
			name:      "people",
			cmd:       insightsPeopleCmd,
			flags:     map[string]string{"days": "7"},
			wantQuery: url.Values{"days": {"7"}, "people_limit": {"25"}, "people_offset": {"0"}},
			wantOut:   []string{"Ada", msgUsageDollars(4.5), "990 tokens"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			queries := setupInsightsTest(t, http.StatusOK, insightsPayload())

			out, err := runInsights(t, tc.cmd, tc.flags)

			require.NoError(t, err)
			require.Len(t, *queries, 1)
			assert.Equal(t, tc.wantQuery, (*queries)[0])
			for _, want := range tc.wantOut {
				assert.Contains(t, out, want)
			}
		})
	}
}

func TestInsightsJSON(t *testing.T) {
	cases := []struct {
		name  string
		cmd   *cobra.Command
		into  any
		check func(t *testing.T, raw string, decoded any)
	}{
		{
			name: "summary",
			cmd:  insightsSummaryCmd,
			into: &insightsSummaryOutput{},
			check: func(t *testing.T, _ string, decoded any) {
				out := decoded.(*insightsSummaryOutput)
				assert.Equal(t, insightsWindow{Account: "testaccount", Days: 7, From: "2026-11-10", To: "2026-11-16"}, out.insightsWindow)
				assert.Equal(t, insightsTotals{CostUSD: 4.5, Requests: 9, TotalTokens: 990, ActiveAgents: 2}, out.insightsTotals)
				require.NotNil(t, out.CostChangePct)
				assert.InDelta(t, 12.5, *out.CostChangePct, 0.001)
				assert.Nil(t, out.RequestsChangePct)
			},
		},
		{
			name: "daily",
			cmd:  insightsDailyCmd,
			into: &insightsDailyOutput{},
			check: func(t *testing.T, _ string, decoded any) {
				out := decoded.(*insightsDailyOutput)
				require.Len(t, out.Daily, 2)
				assert.Equal(t, []insightsAgentSpend{{Name: "sasbot", CostUSD: 1.0}, {Name: "triage", CostUSD: 0.5}}, out.Daily[1].Agents)
			},
		},
		{
			name: "agents leave out who used them",
			cmd:  insightsAgentsCmd,
			into: &insightsAgentsOutput{},
			check: func(t *testing.T, raw string, decoded any) {
				out := decoded.(*insightsAgentsOutput)
				assert.NotContains(t, raw, "used_by")
				assert.Equal(t, insightsPage{TotalCostUSD: 4.5, Count: 2, Offset: 0, HasMore: true}, out.insightsPage)
				require.Len(t, out.Agents, 1)
				assert.Equal(t, insightsAgentRow{
					Name: "sasbot", Kind: "agent", ID: "dep-1", CostUSD: 4.0, CostPct: 88.9,
					Requests: 8, CostPerRequest: 0.5, TokPerRequest: 112.5, LastSeen: "2026-11-16",
				}, out.Agents[0])
			},
		},
		{
			name: "people",
			cmd:  insightsPeopleCmd,
			into: &insightsPeopleOutput{},
			check: func(t *testing.T, _ string, decoded any) {
				out := decoded.(*insightsPeopleOutput)
				require.Len(t, out.People, 1)
				assert.Equal(t, insightsPersonRow{
					Name: "Ada", Kind: "slack", CostUSD: 4.5, CostPct: 100, Requests: 9, Tokens: 990,
					LastSeen: "2026-11-16", AgentsUsed: []string{"sasbot"},
				}, out.People[0])
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupInsightsTest(t, http.StatusOK, insightsPayload())

			out, err := runInsights(t, tc.cmd, map[string]string{"days": "7", "json": "true"})

			require.NoError(t, err)
			require.NoError(t, json.Unmarshal([]byte(out), tc.into))
			tc.check(t, out, tc.into)
		})
	}
}

func TestInsightsRefusals(t *testing.T) {
	cases := []struct {
		name    string
		cmd     *cobra.Command
		flags   map[string]string
		status  int
		wantErr error
		// wantNoCall is true when the flags are refused before any request.
		wantNoCall bool
	}{
		{name: "days the page does not offer", cmd: insightsSummaryCmd, flags: map[string]string{"days": "45"}, wantErr: errInsightsDays(45), wantNoCall: true},
		{name: "limit too large", cmd: insightsAgentsCmd, flags: map[string]string{"limit": strconv.Itoa(insightsMaxLimit + 1)}, wantErr: errInsightsLimit(insightsMaxLimit), wantNoCall: true},
		{name: "negative offset", cmd: insightsPeopleCmd, flags: map[string]string{"offset": "-1"}, wantErr: errInsightsOffset(), wantNoCall: true},
		{name: "no access", cmd: insightsSummaryCmd, status: http.StatusForbidden, wantErr: errInsightsForbidden("testaccount")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := tc.status
			if status == 0 {
				status = http.StatusOK
			}
			queries := setupInsightsTest(t, status, map[string]any{"error": "forbidden"})

			_, err := runInsights(t, tc.cmd, tc.flags)

			require.Error(t, err)
			assert.Equal(t, tc.wantErr.Error(), err.Error())
			if tc.wantNoCall {
				assert.Empty(t, *queries)
			}
		})
	}
}
