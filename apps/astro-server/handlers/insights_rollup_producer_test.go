package handlers

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
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
		facts, err := p.fetchDevtoolGrain(context.Background(), client, ad, "acct_1", day,
			map[string]string{"dev@x.com": "user_1"})
		if err != nil {
			t.Fatalf("fetchDevtoolGrain: %v", err)
		}

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

		facts, err := p.fetchDevtoolGrain(context.Background(), client, ad, "acct_1", day,
			map[string]string{"dev@x.com": "user_1"})
		if err != nil {
			t.Fatalf("fetchDevtoolGrain: %v", err)
		}
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

		facts, err := p.fetchDevtoolGrain(context.Background(), client, ad, "acct_1", day,
			map[string]string{"dev@x.com": "user_1"})
		if err != nil {
			t.Fatalf("fetchDevtoolGrain: %v", err)
		}
		assertFactsSumTo(t, facts, 0, 21388, 2)
	})

	// Presence gate: a source nobody used that day contributes no rows at all,
	// which is what keeps it out of the Sources filter downstream.
	t.Run("unused source yields no facts", func(t *testing.T) {
		p := testProducer()
		client := stubLangfuseMetrics(t, nil)

		facts, err := p.fetchDevtoolGrain(context.Background(), client, ad, "acct_1", day, nil)
		if err != nil {
			t.Fatalf("fetchDevtoolGrain: %v", err)
		}
		if len(facts) != 0 {
			t.Errorf("facts = %+v, want none", facts)
		}
	})
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
