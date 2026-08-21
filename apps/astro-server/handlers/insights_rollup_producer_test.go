package handlers

import (
	"context"
	"encoding/json"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func testProducer() *InsightsRollupProducer {
	return &InsightsRollupProducer{Log: logger.New("error", "text")}
}

// Langfuse returns the `tags` dimension as either a bare string or the full
// array, so both must map to the same deployment.
func TestDeploymentIDFromTags(t *testing.T) {
	p := testProducer()
	acct := &account.Account{ID: "acct_1"}

	tests := []struct {
		name string
		raw  any
		want string
	}{
		{"array with other tags", []any{"env:prod", "deployment:dep-1"}, "dep-1"},
		{"bare string", "deployment:dep-2", "dep-2"},
		{"no deployment tag", []any{"env:prod"}, ""},
		// A trace with no tags at all is real — SDK calls that didn't tag, or
		// spend outside a deployment. It aggregates under the '' sentinel.
		{"nil", nil, ""},
		{"empty array", []any{}, ""},
		// "deployment:" with no id must not become a row keyed on empty string
		// that pretends to be a real deployment.
		{"prefix only", []any{"deployment:"}, ""},
		{"non-string members ignored", []any{42, "deployment:dep-3"}, "dep-3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.deploymentIDFromTags(acct, tt.raw); got != tt.want {
				t.Errorf("deploymentIDFromTags(%v) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// The whole single-grain design assumes one deployment tag per trace. If that
// breaks, spend must still be attributed somewhere rather than dropped — but it
// has to be loud, so the producer logs an error and takes the first tag.
func TestDeploymentIDFromTagsMultipleDeploymentsPicksFirst(t *testing.T) {
	p := testProducer()
	got := p.deploymentIDFromTags(&account.Account{ID: "acct_1"},
		[]any{"deployment:dep-1", "deployment:dep-2"})
	if got != "dep-1" {
		t.Errorf("got %q, want dep-1 (first tag, not dropped)", got)
	}
}

// Actor columns must be derivable from the raw user id alone, because that is
// what keeps identity *display* resolution at read time.
func TestRollupActorFor(t *testing.T) {
	tests := []struct {
		name     string
		raw      any
		wantKind string
		wantKey  string
	}{
		// A trace with no user is the pinned system-spend row.
		{"empty is system", "", insightsrollup.ActorKindSystem, ""},
		{"nil is system", nil, insightsrollup.ActorKindSystem, ""},
		// WorkOS ids share a key space with dev-tool spend resolved from email,
		// which is exactly what lets the two merge into one People row.
		{"workos user is member", "user_123", insightsrollup.ActorKindMember, "user_123"},
		// Slack stores the bare id; the team is read-time enrichment, so baking
		// it in here would freeze a value the directory can still learn.
		{"bare slack id", "U024BE7LH", insightsrollup.ActorKindSlack, "U024BE7LH"},
		{"unknown id", "someone@example.com", insightsrollup.ActorKindUnidentified, "someone@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, key := rollupActorFor(tt.raw)
			if kind != tt.wantKind || key != tt.wantKey {
				t.Errorf("rollupActorFor(%v) = (%q, %q), want (%q, %q)",
					tt.raw, kind, key, tt.wantKind, tt.wantKey)
			}
		})
	}
}

func TestFetchUsageGrainExcludesDevtoolTaggedTraces(t *testing.T) {
	p := testProducer()
	acct := &account.Account{ID: "acct_1"}
	client := stubLangfuseMetrics(t, []map[string]any{
		metricRow("2026-08-11", "user_1", []any{"deployment:dep-1"}, 4.00, 400, 4),
		metricRow("2026-08-11", "dev@x.com", ccTags, 1100.00, 110000, 900),
		metricRow("2026-08-11", "user_2", nil, 0.50, 50, 1),
	})

	byDate, err := p.fetchUsageGrain(context.Background(), client, acct,
		"2026-08-11T00:00:00Z", "2026-08-12T00:00:00Z")
	if err != nil {
		t.Fatalf("fetchUsageGrain: %v", err)
	}
	facts := byDate["2026-08-11"]

	for _, f := range facts {
		if f.ActorKey == "dev@x.com" || f.CostUSD == 1100.00 {
			t.Errorf("dev-tool spend banked as agent usage: %+v", f)
		}
	}

	byActor := factsByActor(t, facts)
	tagged, ok := byActor[[2]string{insightsrollup.ActorKindMember, "user_1"}]
	if !ok || tagged.DeploymentID != "dep-1" || tagged.CostUSD != 4.00 {
		t.Errorf("deployment fact = %+v, want dep-1 at cost 4", tagged)
	}
	untagged, ok := byActor[[2]string{insightsrollup.ActorKindMember, "user_2"}]
	if !ok || untagged.DeploymentID != "" || untagged.CostUSD != 0.50 {
		t.Errorf("untagged fact = %+v, want empty deployment at cost 0.5", untagged)
	}

	assertFactsSumTo(t, facts, 4.50, 450, 5)
}

// factsByActor indexes one day's facts by (kind, key), which is the tuple the
// primary key is built from, so a duplicate would be a write error rather than a
// wrong number.
func factsByActor(t *testing.T, facts []insightsrollup.Fact) map[[2]string]insightsrollup.Fact {
	t.Helper()
	out := make(map[[2]string]insightsrollup.Fact, len(facts))
	for _, f := range facts {
		k := [2]string{f.ActorKind, f.ActorKey}
		if _, dup := out[k]; dup {
			t.Fatalf("duplicate fact for %v", k)
		}
		out[k] = f
	}
	return out
}

// Dev-tool attribution is what merges a developer's local usage with their agent
// usage: an email that resolves to a member is stored under the WorkOS user id,
// the same key space agent traces use, so the two become one People row by plain
// aggregation instead of a join. An email that resolves to nothing stays its own
// unidentified row rather than being dropped.
//
// The remainder row is the load-bearing part. byUser sees only cells that carry
// a userId, so usage reported without one would otherwise vanish and shrink the
// source's total silently. Every fact set here is checked to sum back to the
// window total for that reason.
func TestFetchDevtoolGrainAttributesSpendAndKeepsTheTotalWhole(t *testing.T) {
	ad := devtoolAdapters[0]
	day := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	tags := []any{ad.Key}

	t.Run("resolved member, unresolved email, and usage with no user", func(t *testing.T) {
		p := testProducer()
		client := stubLangfuseMetrics(t, []map[string]any{
			metricRow("2026-08-11", "Dev@X.com", tags, 3.00, 300, 3),
			metricRow("2026-08-11", "contractor@y.com", tags, 1.50, 150, 2),
			metricRow("2026-08-11", "", tags, 0.50, 50, 1),
		})

		// Mixed case on purpose: member_emails is lowercased, and a developer's
		// tool reports whatever case they typed.
		facts := devtoolFactsForDay(t, p, client, ad, "2026-08-11",
			map[string]string{"dev@x.com": "user_1"})

		byActor := factsByActor(t, facts)
		member, ok := byActor[[2]string{insightsrollup.ActorKindMember, "user_1"}]
		if !ok {
			t.Fatalf("no member fact for user_1; facts = %+v", facts)
		}
		if member.CostUSD != 3.00 || member.TotalTokens != 300 || member.Requests != 3 {
			t.Errorf("member fact = %+v, want cost 3 / tokens 300 / requests 3", member)
		}
		if member.DeploymentID != "" {
			t.Errorf("member fact deployment = %q, want empty: dev-tool spend has no deployment", member.DeploymentID)
		}
		if !member.LastSeenAt.Equal(day) {
			t.Errorf("last seen = %v, want %v", member.LastSeenAt, day)
		}

		unidentified, ok := byActor[[2]string{insightsrollup.ActorKindUnidentified, "contractor@y.com"}]
		if !ok {
			t.Fatalf("no unidentified fact for contractor@y.com; facts = %+v", facts)
		}
		if unidentified.CostUSD != 1.50 {
			t.Errorf("unidentified cost = %v, want 1.50", unidentified.CostUSD)
		}

		system, ok := byActor[[2]string{insightsrollup.ActorKindSystem, ""}]
		if !ok {
			t.Fatalf("no system remainder fact; facts = %+v", facts)
		}
		if system.CostUSD != 0.50 || system.TotalTokens != 50 || system.Requests != 1 {
			t.Errorf("remainder = %+v, want cost 0.5 / tokens 50 / requests 1", system)
		}

		assertFactsSumTo(t, facts, 5.00, 500, 6)
	})

	// A fully attributed day must not carry an empty system row: it would render
	// as system spend on the People table and read as a bug.
	t.Run("no remainder row when every cell has a user", func(t *testing.T) {
		p := testProducer()
		client := stubLangfuseMetrics(t, []map[string]any{
			metricRow("2026-08-11", "dev@x.com", tags, 2.00, 200, 2),
		})

		facts := devtoolFactsForDay(t, p, client, ad, "2026-08-11",
			map[string]string{"dev@x.com": "user_1"})
		if len(facts) != 1 {
			t.Fatalf("facts = %+v, want one member fact", facts)
		}
		if facts[0].ActorKind != insightsrollup.ActorKindMember {
			t.Errorf("actor kind = %q, want member", facts[0].ActorKind)
		}
		assertFactsSumTo(t, facts, 2.00, 200, 2)
	})

	// An unpriced model reports tokens at zero cost. The row still has to be
	// written, or the tokens disappear along with the source.
	t.Run("unpriced usage is still attributed", func(t *testing.T) {
		p := testProducer()
		client := stubLangfuseMetrics(t, []map[string]any{
			metricRow("2026-08-11", "dev@x.com", tags, 0, 21388, 2),
		})

		facts := devtoolFactsForDay(t, p, client, ad, "2026-08-11",
			map[string]string{"dev@x.com": "user_1"})
		assertFactsSumTo(t, facts, 0, 21388, 2)
	})

	// Presence gate: a source nobody used that day contributes no rows at all,
	// which is what keeps it out of the Sources filter downstream.
	t.Run("unused source yields no facts", func(t *testing.T) {
		p := testProducer()
		client := stubLangfuseMetrics(t, nil)

		if facts := devtoolFactsForDay(t, p, client, ad, "2026-08-11", nil); len(facts) != 0 {
			t.Errorf("facts = %+v, want none", facts)
		}
	})
}

// devtoolFactsForDay runs the fetch-then-split path the producer uses and
// returns one day's facts, so a test can assert on attribution without
// restating how a window is sliced.
func devtoolFactsForDay(
	t *testing.T,
	p *InsightsRollupProducer,
	client *langfuse.Client,
	ad devtoolAdapter,
	date string,
	emails map[string]string,
) []insightsrollup.Fact {
	t.Helper()
	day := dayInstant(date)
	usage, err := fetchDevtoolUsage(context.Background(), client, ad.Key, day, day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("fetchDevtoolUsage: %v", err)
	}
	return p.devtoolFactsFor(usage.byDate()[date], ad, "acct_1", date, emails)
}

// assertFactsSumTo is the invariant behind the remainder row: whatever the
// attribution managed to resolve, the day's facts still account for the source's
// whole window total.
func assertFactsSumTo(t *testing.T, facts []insightsrollup.Fact, cost float64, tokens, requests int64) {
	t.Helper()
	var gotCost float64
	var gotTokens, gotRequests int64
	for _, f := range facts {
		gotCost += f.CostUSD
		gotTokens += f.TotalTokens
		gotRequests += f.Requests
	}
	if math.Abs(gotCost-cost) > 0.0000005 || gotTokens != tokens || gotRequests != requests {
		t.Errorf("facts sum to cost %v / tokens %d / requests %d, want %v / %d / %d",
			gotCost, gotTokens, gotRequests, cost, tokens, requests)
	}
}

// ── range fetch: one query, one row per (day, group) ──────────────────────────

// The change these guard: a window is fetched in one request and split by the
// day bucket Langfuse returns. If the split were dropped, every day's spend
// would collapse onto whichever bucket happened to be written last, and the
// account's 90-day history would become one enormous day.

// usageRow is one (day, tags, userId) cell of the count or cost/tokens query.
func usageRow(date string, tags []any, userID string, extra map[string]any) map[string]any {
	row := map[string]any{
		langfuseTimeDimensionKey: date + "T00:00:00.000Z",
		"tags":                   tags,
		"userId":                 userID,
	}
	maps.Copy(row, extra)
	return row
}

func countCell(date string, tags []any, userID string, count int) map[string]any {
	return usageRow(date, tags, userID, map[string]any{"count_count": float64(count)})
}

func costCell(date string, tags []any, userID string, cost float64, tokens int) map[string]any {
	return usageRow(date, tags, userID, map[string]any{
		"sum_totalCost": cost, "sum_totalTokens": float64(tokens),
	})
}

// stubMetricsServer answers the count query first and the cost/tokens query
// second, which is the order fetchUsageGrain issues them in. Any further query
// (the model grain) gets the second response, which carries no model column and
// so yields no model facts.
func stubMetricsServer(t *testing.T, first, second []map[string]any) string {
	t.Helper()
	pinV3Langfuse(t)
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows := first
		if n > 0 {
			rows = second
		}
		n++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(langfuse.MetricsResponse{Data: rows})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func stubTwoMetricsResponses(t *testing.T, first, second []map[string]any) *langfuse.Client {
	t.Helper()
	return langfuse.NewClient(stubMetricsServer(t, first, second), "pk", "sk")
}

func TestFetchUsageGrainSplitsAWindowIntoDays(t *testing.T) {
	p := testProducer()
	acct := &account.Account{ID: "acct_1"}
	tags := []any{"deployment:dep-1"}

	client := stubTwoMetricsResponses(t,
		[]map[string]any{
			countCell("2026-08-10", tags, "user_1", 3),
			countCell("2026-08-11", tags, "user_1", 5),
			countCell("2026-08-12", tags, "user_1", 7),
		},
		[]map[string]any{
			costCell("2026-08-10", tags, "user_1", 1.00, 100),
			costCell("2026-08-11", tags, "user_1", 2.00, 200),
			costCell("2026-08-12", tags, "user_1", 3.00, 300),
		})

	byDate, err := p.fetchUsageGrain(context.Background(), client, acct,
		"2026-08-10T00:00:00Z", "2026-08-13T00:00:00Z")
	if err != nil {
		t.Fatalf("fetchUsageGrain: %v", err)
	}

	want := map[string]struct {
		requests int64
		cost     float64
		tokens   int64
	}{
		"2026-08-10": {3, 1.00, 100},
		"2026-08-11": {5, 2.00, 200},
		"2026-08-12": {7, 3.00, 300},
	}
	if len(byDate) != len(want) {
		t.Fatalf("got %d days, want %d: %+v", len(byDate), len(want), byDate)
	}
	for date, w := range want {
		facts := byDate[date]
		if len(facts) != 1 {
			t.Fatalf("%s: %d facts, want 1: %+v", date, len(facts), facts)
		}
		f := facts[0]
		if f.Requests != w.requests || f.CostUSD != w.cost || f.TotalTokens != w.tokens {
			t.Errorf("%s = %+v, want requests %d / cost %v / tokens %d",
				date, f, w.requests, w.cost, w.tokens)
		}
		if f.DeploymentID != "dep-1" {
			t.Errorf("%s deployment = %q, want dep-1", date, f.DeploymentID)
		}
		// last_seen is the row's own bucket, not the window's edge. Getting this
		// from the window would date every day in a 30-day fetch identically.
		if got := f.LastSeenAt.Format(time.DateOnly); got != date {
			t.Errorf("%s last_seen = %s, want the row's own day", date, got)
		}
	}
}

// The count and cost/tokens queries are joined back together by group key. That
// key has to include the day: without it, the same (tags, userId) on different
// days collides, and every day's requests get paired with one arbitrary day's
// cost.
func TestFetchUsageGrainJoinsCountAndCostWithinTheSameDay(t *testing.T) {
	p := testProducer()
	acct := &account.Account{ID: "acct_1"}
	tags := []any{"deployment:dep-1"}

	client := stubTwoMetricsResponses(t,
		[]map[string]any{
			countCell("2026-08-10", tags, "user_1", 1),
			countCell("2026-08-11", tags, "user_1", 2),
		},
		[]map[string]any{
			costCell("2026-08-10", tags, "user_1", 10.00, 1000),
			costCell("2026-08-11", tags, "user_1", 20.00, 2000),
		})

	byDate, err := p.fetchUsageGrain(context.Background(), client, acct,
		"2026-08-10T00:00:00Z", "2026-08-12T00:00:00Z")
	if err != nil {
		t.Fatalf("fetchUsageGrain: %v", err)
	}
	for _, want := range []struct {
		date     string
		requests int64
		cost     float64
	}{
		{"2026-08-10", 1, 10.00},
		{"2026-08-11", 2, 20.00},
	} {
		facts := byDate[want.date]
		if len(facts) != 1 {
			t.Errorf("%s: %d facts, want 1: %+v", want.date, len(facts), facts)
			continue
		}
		if facts[0].Requests != want.requests || facts[0].CostUSD != want.cost {
			t.Errorf("%s = %+v, want requests %d / cost %v (not the other day's)",
				want.date, facts[0], want.requests, want.cost)
		}
	}
}

// A group that appears only in the cost/tokens response is folded in rather
// than dropped, and it has to land on its own day like any other row.
func TestFetchUsageGrainKeepsCostOnlyGroupsOnTheirOwnDay(t *testing.T) {
	p := testProducer()
	acct := &account.Account{ID: "acct_1"}
	tags := []any{"deployment:dep-1"}

	client := stubTwoMetricsResponses(t,
		[]map[string]any{countCell("2026-08-10", tags, "user_1", 1)},
		[]map[string]any{
			costCell("2026-08-10", tags, "user_1", 10.00, 1000),
			costCell("2026-08-11", tags, "user_2", 5.00, 500),
		})

	byDate, err := p.fetchUsageGrain(context.Background(), client, acct,
		"2026-08-10T00:00:00Z", "2026-08-12T00:00:00Z")
	if err != nil {
		t.Fatalf("fetchUsageGrain: %v", err)
	}
	orphan := byDate["2026-08-11"]
	if len(orphan) != 1 || orphan[0].CostUSD != 5.00 {
		t.Fatalf("2026-08-11 = %+v, want the cost-only group folded in", orphan)
	}
	if orphan[0].Requests != 0 {
		t.Errorf("cost-only group requests = %d, want 0", orphan[0].Requests)
	}
	// No count row means no evidence of a request, so last_seen stays unset
	// rather than asserting activity the count query didn't see.
	if !orphan[0].LastSeenAt.IsZero() {
		t.Errorf("cost-only group last_seen = %v, want zero", orphan[0].LastSeenAt)
	}
}

func TestFetchModelGrainSplitsAWindowIntoDays(t *testing.T) {
	p := testProducer()
	client := stubLangfuseMetrics(t, []map[string]any{
		{
			langfuseTimeDimensionKey: "2026-08-10T00:00:00.000Z",
			"providedModelName":      "claude-opus-5",
			"sum_totalCost":          1.00, "sum_inputTokens": float64(10),
			"sum_outputTokens": float64(5), "count_count": float64(2),
		},
		{
			langfuseTimeDimensionKey: "2026-08-11T00:00:00.000Z",
			"providedModelName":      "claude-opus-5",
			"sum_totalCost":          2.00, "sum_inputTokens": float64(20),
			"sum_outputTokens": float64(10), "count_count": float64(4),
		},
		// Non-LLM observations carry no model and must not become a row: the
		// grain's CHECK constraint forbids an empty model.
		{
			langfuseTimeDimensionKey: "2026-08-11T00:00:00.000Z",
			"providedModelName":      nil,
			"sum_totalCost":          9.00, "count_count": float64(1),
		},
	})

	byDate, err := p.fetchModelGrain(context.Background(), client,
		"2026-08-10T00:00:00Z", "2026-08-12T00:00:00Z")
	if err != nil {
		t.Fatalf("fetchModelGrain: %v", err)
	}
	if len(byDate) != 2 {
		t.Fatalf("got %d days, want 2: %+v", len(byDate), byDate)
	}
	first := byDate["2026-08-10"]
	if len(first) != 1 || first[0].CostUSD != 1.00 || first[0].TotalTokens != 15 {
		t.Errorf("2026-08-10 = %+v, want one row at cost 1 / tokens 15", first)
	}
	second := byDate["2026-08-11"]
	if len(second) != 1 || second[0].CostUSD != 2.00 || second[0].TotalTokens != 30 {
		t.Errorf("2026-08-11 = %+v, want one row at cost 2 / tokens 30 (model-less row dropped)", second)
	}
}

// Dev-tool cells already carry their day. Splitting them back out is what lets
// one range query serve every day in the window.
func TestDevtoolUsageByDateSplitsDaysApart(t *testing.T) {
	ad := devtoolAdapters[0]
	tags := []any{ad.Key}
	client := stubLangfuseMetrics(t, []map[string]any{
		metricRow("2026-08-10", "dev@x.com", tags, 1.00, 100, 1),
		metricRow("2026-08-11", "dev@x.com", tags, 2.00, 200, 2),
		metricRow("2026-08-11", "other@x.com", tags, 0.50, 50, 1),
	})

	usage, err := fetchDevtoolUsage(context.Background(), client, ad.Key,
		time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("fetchDevtoolUsage: %v", err)
	}

	byDate := usage.byDate()
	if got := byDate["2026-08-10"].totals(); got.CostUSD != 1.00 || got.Requests != 1 {
		t.Errorf("2026-08-10 totals = %+v, want cost 1 / requests 1", got)
	}
	if got := byDate["2026-08-11"].totals(); got.CostUSD != 2.50 || got.Requests != 3 {
		t.Errorf("2026-08-11 totals = %+v, want cost 2.50 / requests 3", got)
	}

	// Each day's facts carry that day's last_seen, and together they still sum
	// to the window total the single fetch reported.
	p := testProducer()
	var cost float64
	for _, date := range []string{"2026-08-10", "2026-08-11"} {
		for _, f := range p.devtoolFactsFor(byDate[date], ad, "acct_1", date, nil) {
			cost += f.CostUSD
			if got := f.LastSeenAt.Format(time.DateOnly); got != date {
				t.Errorf("%s fact last_seen = %s", date, got)
			}
		}
	}
	if math.Abs(cost-3.50) > 0.0000005 {
		t.Errorf("facts across days sum to %v, want the window total 3.50", cost)
	}
}

// Every day in the window is replaced, including the days upstream reports
// nothing for. A range fetch only returns days that had activity, so writing
// just those would leave yesterday's rows in place on a day whose spend went to
// zero, and the page would keep charging for an agent that stopped running.
func TestRollUpAgentsRangeReplacesEveryRequestedDay(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close() //nolint:errcheck

	// Only the middle day carries usage; the other two came back empty.
	tags := []any{"deployment:dep-1"}
	baseURL := stubMetricsServer(t,
		[]map[string]any{countCell("2026-08-11", tags, "user_1", 5)},
		[]map[string]any{costCell("2026-08-11", tags, "user_1", 2.00, 200)})

	mock.ExpectQuery("SELECT account_id, langfuse_project_id").
		WithArgs("acct_1").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "langfuse_project_id", "langfuse_public_key", "langfuse_secret_key", "created_at"}).
			AddRow("acct_1", "proj_1", "pk", "sk", time.Now()))

	days := []time.Time{
		time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC),
	}
	// Two grains per day, each a transaction that clears the day first. The
	// empty days still get their DELETE; only the middle day also inserts.
	for _, d := range days {
		for _, grain := range []insightsrollup.Grain{insightsrollup.GrainUsage, insightsrollup.GrainModel} {
			mock.ExpectBegin()
			mock.ExpectExec("DELETE FROM insights_usage_daily").
				WithArgs("acct_1", string(grain), d.Format(time.DateOnly), insightsrollup.SourceAgents).
				WillReturnResult(sqlmock.NewResult(0, 0))
			if d.Day() == 11 && grain == insightsrollup.GrainUsage {
				mock.ExpectExec("INSERT INTO insights_usage_daily").
					WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()
		}
	}

	p := &InsightsRollupProducer{
		Log:           logger.New("error", "text"),
		Cfg:           &config.Config{Deployment: config.DeploymentConfig{LangfuseBaseURL: baseURL}},
		LangfuseStore: langfuse.NewStore(db),
		Rollups:       insightsrollup.NewStore(db),
	}
	if err := p.RollUpAgentsRange(context.Background(), &account.Account{ID: "acct_1"}, days); err != nil {
		t.Fatalf("RollUpAgentsRange: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("a requested day was not replaced: %v", err)
	}
}
