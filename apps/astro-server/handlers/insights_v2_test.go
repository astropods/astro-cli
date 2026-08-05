package handlers

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// The People spend chart is built from summary.CostOverTimeByUser, not from a
// daily total — it reports how many distinct people were active each day, which
// a summed series cannot answer. rollupSummary originally populated only
// CostOverTime, so the chart rendered all zeros on every response.
func TestRollupSummaryPopulatesCostOverTimeByUser(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	d1 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	// Two actors on day one, one on day two, so the grouping is observable.
	rows := sqlmock.NewRows([]string{"day", "actor_key", "cost_usd", "requests", "tokens", "actors"}).
		AddRow(d1, "user_a", 1.5, 3, 300, 0).
		AddRow(d1, "user_b", 0.5, 1, 100, 0).
		AddRow(d2, "user_a", 2.0, 2, 200, 0)
	mock.ExpectQuery("SELECT day, actor_key").WillReturnRows(rows)

	summary, err := rollupSummary(context.Background(), insightsrollup.NewStore(db),
		&account.Account{ID: "acct_1"},
		insightsrollup.Window{From: d1, To: d2}, insightsrollup.Filter{})
	if err != nil {
		t.Fatalf("rollupSummary: %v", err)
	}

	if len(summary.CostOverTimeByUser) != 2 {
		t.Fatalf("CostOverTimeByUser days = %d, want 2: %+v", len(summary.CostOverTimeByUser), summary.CostOverTimeByUser)
	}
	first := summary.CostOverTimeByUser[0]
	if first.Date != "2026-08-01" {
		t.Errorf("first date = %q, want 2026-08-01", first.Date)
	}
	if len(first.Users) != 2 {
		t.Fatalf("day one users = %d, want 2", len(first.Users))
	}
	if first.Users[0].UserID != "user_a" || first.Users[0].CostUSD != 1.5 {
		t.Errorf("day one first user = %+v", first.Users[0])
	}
	if len(summary.CostOverTimeByUser[1].Users) != 1 {
		t.Errorf("day two users = %d, want 1", len(summary.CostOverTimeByUser[1].Users))
	}

	// The daily timeline must stay aligned with the per-user breakdown: one
	// entry per day that had activity, in the same order.
	if len(summary.CostOverTime) != 2 {
		t.Fatalf("CostOverTime days = %d, want 2", len(summary.CostOverTime))
	}
	if summary.CostOverTime[0].Date != "2026-08-01" || summary.CostOverTime[1].Date != "2026-08-02" {
		t.Errorf("CostOverTime dates = %q, %q", summary.CostOverTime[0].Date, summary.CostOverTime[1].Date)
	}

	if summary.Totals.CostUSD != 4.0 || summary.Totals.Requests != 6 || summary.Totals.TotalTokens != 600 {
		t.Errorf("totals = %+v", summary.Totals)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// A deployment that exists only in the facts has no name, and the view model
// derives its label from DisplayName-then-AgentName — so without a fallback it
// renders as a blank row rather than as a missing one, which reads as a bug.
// Spend must never be dropped to avoid that; it gets labelled by id instead.
func TestRollupDeploymentEntriesLabelsUnknownDeploymentByID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	day := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"day", "deployment_id", "cost_usd", "requests", "tokens", "actors"}).
		AddRow(day, "gone-123", 0.25, 4, 400, 0)
	mock.ExpectQuery("SELECT day, deployment_id").WillReturnRows(rows)
	// Dev-tool sources become their own entries from the (day, source) grain;
	// none here.
	mock.ExpectQuery("SELECT day, source").
		WillReturnRows(sqlmock.NewRows([]string{"day", "source", "cost_usd", "requests", "tokens", "actors"}))
	// used_by pairs come from the same grain; none here, since the point of this
	// case is the unnamed deployment rather than its actors.
	mock.ExpectQuery("SELECT DISTINCT deployment_id").
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "actor_kind", "actor_key"}))

	// deploymentStore nil: no visible list and no archived lookup, which is also
	// the shape when a deployment has been purged outright.
	resp, err := rollupDeploymentEntries(context.Background(), logger.New("error", "text"),
		insightsrollup.NewStore(db), nil, &account.Account{ID: "acct_1"},
		insightsrollup.Window{From: day, To: day}, insightsrollup.Filter{})
	if err != nil {
		t.Fatalf("rollupDeploymentEntries: %v", err)
	}

	if len(resp.Deployments) != 1 {
		t.Fatalf("deployments = %d, want 1 (spend must not be dropped)", len(resp.Deployments))
	}
	e := resp.Deployments[0]
	if e.AgentName != "gone-123" {
		t.Errorf("AgentName = %q, want the deployment id as fallback label", e.AgentName)
	}
	if !e.IsArchived {
		t.Error("IsArchived = false, want true for a deployment absent from the DB")
	}
	if e.CostUSD != 0.25 || e.Requests != 4 {
		t.Errorf("metrics lost: cost=%v requests=%d", e.CostUSD, e.Requests)
	}
}

// Agent chips on People rows render the avatar of the account that *published*
// the blueprint, not the account viewing the page. Hardcoding the viewer gave
// every cross-account agent the wrong icon.
func TestAgentAvatarAccount(t *testing.T) {
	acct := &account.Account{ID: "acct_viewer", Name: "viewer"}
	srcNames := map[string]string{"acct_pub": "publisher"}
	ptr := func(s string) *string { return &s }

	tests := []struct {
		name   string
		source *string
		want   string
	}{
		{"own deployment", nil, "viewer"},
		{"empty source id", ptr(""), "viewer"},
		// Self-sourced: the blueprint is this account's own, so no indirection.
		{"source is this account", ptr("acct_viewer"), "viewer"},
		{"public blueprint from another account", ptr("acct_pub"), "publisher"},
		// Unresolvable source falls back to the viewer: a chip with an empty
		// account segment would be a broken link, which is worse than a wrong
		// avatar.
		{"unresolved source name", ptr("acct_unknown"), "viewer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := agentAvatarAccount(acct, tt.source, srcNames); got != tt.want {
				t.Errorf("agentAvatarAccount(%v) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

// System spend arrives with an empty actor key. It must reach the chart rather
// than being dropped, or account cost silently shrinks by whatever ran without
// a user.
func TestRollupSummaryKeepsSystemSpend(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	day := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"day", "actor_key", "cost_usd", "requests", "tokens", "actors"}).
		AddRow(day, "", 0.75, 2, 150, 0)
	mock.ExpectQuery("SELECT day, actor_key").WillReturnRows(rows)

	summary, err := rollupSummary(context.Background(), insightsrollup.NewStore(db),
		&account.Account{ID: "acct_1"},
		insightsrollup.Window{From: day, To: day}, insightsrollup.Filter{})
	if err != nil {
		t.Fatalf("rollupSummary: %v", err)
	}
	if len(summary.CostOverTimeByUser) != 1 || len(summary.CostOverTimeByUser[0].Users) != 1 {
		t.Fatalf("system row dropped: %+v", summary.CostOverTimeByUser)
	}
	if summary.Totals.CostUSD != 0.75 {
		t.Errorf("total = %v, want 0.75", summary.Totals.CostUSD)
	}
}
