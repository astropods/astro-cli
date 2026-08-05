package handlers

import (
	"testing"

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
