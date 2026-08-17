package handlers

import (
	"context"
	"errors"
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

// The rollup facts hold complete days only, so the reported window must end on
// the watermark. Anchoring it on today appended a day the facts can never fill.
func TestInsightsAsOfDayAnchorsOnWatermark(t *testing.T) {
	now := time.Date(2026, 8, 6, 9, 30, 0, 0, time.UTC)
	lastComplete := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		state insightsrollup.State
		want  time.Time
	}{
		{
			name: "healthy watermark",
			state: insightsrollup.State{
				RolledUpThrough: lastComplete,
			},
			want: lastComplete,
		},
		{
			// A held watermark is the whole point of the design: the page reports
			// the coverage it has rather than claiming days that never rolled up.
			name: "stalled watermark reports the older day",
			state: insightsrollup.State{
				RolledUpThrough: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC),
			},
			want: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "cold account falls back to the last complete day",
			state: insightsrollup.State{},
			want:  lastComplete,
		},
		{
			// Defensive: a watermark at today would mean a partial day was written.
			name: "watermark ahead of the clock is clamped",
			state: insightsrollup.State{
				RolledUpThrough: time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC),
			},
			want: lastComplete,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := insightsAsOfDay(tc.state, now); !got.Equal(tc.want) {
				t.Errorf("insightsAsOfDay = %s, want %s",
					got.Format(time.DateOnly), tc.want.Format(time.DateOnly))
			}
		})
	}
}

// insightsAsOfDay decides the horizon; this pins that ComputeInsightsFromRollups
// actually reads with it. The regression it guards is the caller, not the
// arithmetic: the window derivation was correct all along and was simply handed
// time.Now(), which on this path is always a day the facts cannot cover.
//
// The read is cut short deliberately. The window reaches the database as bind
// parameters $3/$4 on the first aggregate, so matching those args is the whole
// assertion — letting the rest of the pipeline run would add a dozen unrelated
// expectations without strengthening it. A WithArgs mismatch surfaces as
// sqlmock's own error instead of the sentinel, so the errors.Is check below is
// what proves the dates matched.
func TestComputeInsightsFromRollupsReadsThroughTheWatermark(t *testing.T) {
	now := time.Date(2026, 8, 6, 9, 30, 0, 0, time.UTC)
	errStopAfterWindow := errors.New("stop: window asserted")

	tests := []struct {
		name      string
		watermark any
		wantFrom  string
		wantTo    string
	}{
		{
			// 90 days ending on the watermark, not on 2026-08-06.
			name:      "healthy watermark",
			watermark: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC),
			wantFrom:  "2026-05-08",
			wantTo:    "2026-08-05",
		},
		{
			// A held watermark shortens the window rather than padding it with
			// days the roll-up never wrote.
			name:      "stalled watermark",
			watermark: time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
			wantFrom:  "2026-05-02",
			wantTo:    "2026-07-30",
		},
		{
			name:      "no watermark falls back to the last complete day",
			watermark: nil,
			wantFrom:  "2026-05-08",
			wantTo:    "2026-08-05",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer db.Close()

			mock.ExpectQuery("SELECT rolled_up_through").
				WithArgs("acct_1", insightsrollup.SourceAgents).
				WillReturnRows(sqlmock.NewRows(
					[]string{"rolled_up_through", "last_run_at", "last_error", "consecutive_errors"}).
					AddRow(tc.watermark, nil, "", 0))
			mock.ExpectQuery("SELECT day, deployment_id").
				WithArgs("acct_1", string(insightsrollup.GrainUsage), tc.wantFrom, tc.wantTo).
				WillReturnError(errStopAfterWindow)

			_, err = ComputeInsightsFromRollups(context.Background(), logger.New("error", "json"),
				nil, nil, nil, insightsrollup.NewStore(db),
				&account.Account{ID: "acct_1", Name: "acme"}, nil, now,
				insightsRequestParams{})
			if !errors.Is(err, errStopAfterWindow) {
				t.Fatalf("read window did not match %s..%s: %v", tc.wantFrom, tc.wantTo, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations: %v", err)
			}
		})
	}
}

// expectEmptyRollupRead queues the rest of the read after the watermark, all
// empty. Ordered rather than by-args, because several of these aggregates share
// a SELECT prefix and the call order is what tells them apart.
func expectEmptyRollupRead(mock sqlmock.Sqlmock) {
	empty := func(cols ...string) *sqlmock.Rows { return sqlmock.NewRows(cols) }
	dayCols := []string{"day", "key", "cost_usd", "requests", "tokens", "actors"}

	mock.ExpectQuery("SELECT day, deployment_id").WillReturnRows(empty(dayCols...))
	mock.ExpectQuery("SELECT day, source").WillReturnRows(empty(dayCols...))
	mock.ExpectQuery("SELECT DISTINCT deployment_id").
		WillReturnRows(empty("deployment_id", "actor_kind", "actor_key"))
	mock.ExpectQuery("WITH agg AS").
		WillReturnRows(empty("actor_kind", "actor_key", "requests", "cost_usd", "tokens", "last_seen_at", "cost_pct"))
	mock.ExpectQuery("SELECT DISTINCT deployment_id").
		WillReturnRows(empty("deployment_id", "actor_kind", "actor_key"))
	mock.ExpectQuery("SELECT DISTINCT actor_key").WillReturnRows(empty("actor_key", "source"))
	mock.ExpectQuery("SELECT day, actor_key").WillReturnRows(empty(dayCols...))
	mock.ExpectQuery("SELECT am.account_id, am.user_id").
		WillReturnRows(empty("account_id", "user_id", "workos_membership_id", "created_at"))
	// One Totals probe per dev-tool adapter, for the Sources filter.
	for range devtoolAdapters {
		mock.ExpectQuery("COALESCE\\(SUM\\(cost_usd\\)").
			WillReturnRows(sqlmock.NewRows([]string{"cost_usd", "requests", "tokens"}).AddRow(0.0, 0, 0))
	}
}

// as_of is the client's cue that "nothing today" means coverage rather than an
// outage, so when it is reported and when it is withheld are both contract.
//
// Withholding it on a cold account is the case worth pinning: the window still
// has to end somewhere, and it ends on the last complete day — but stamping
// as_of from that would claim coverage through a day the roll-up has never run.
func TestComputeInsightsFromRollupsReportsAsOf(t *testing.T) {
	now := time.Date(2026, 8, 6, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name      string
		watermark any
		wantAsOf  string
		wantEnd   string
	}{
		{
			name:      "watermark is reported as the coverage day",
			watermark: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC),
			wantAsOf:  "2026-08-05",
			wantEnd:   "2026-08-05T23:59:59.999Z",
		},
		{
			name:      "stalled watermark reports the day it actually reached",
			watermark: time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
			wantAsOf:  "2026-07-30",
			wantEnd:   "2026-07-30T23:59:59.999Z",
		},
		{
			name:      "cold account claims no coverage",
			watermark: nil,
			wantAsOf:  "",
			wantEnd:   "2026-08-05T23:59:59.999Z",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer db.Close()

			mock.ExpectQuery("SELECT rolled_up_through").
				WillReturnRows(sqlmock.NewRows(
					[]string{"rolled_up_through", "last_run_at", "last_error", "consecutive_errors"}).
					AddRow(tc.watermark, nil, "", 0))
			expectEmptyRollupRead(mock)

			resp, err := ComputeInsightsFromRollups(context.Background(), logger.New("error", "json"),
				account.NewAccountStore(db), nil, nil, insightsrollup.NewStore(db),
				&account.Account{ID: "acct_1", Name: "acme"}, nil, now,
				normalizeInsightsRequestParams(defaultInsightsRequestParams()))
			if err != nil {
				t.Fatalf("ComputeInsightsFromRollups: %v", err)
			}

			if resp.AsOf != tc.wantAsOf {
				t.Errorf("as_of = %q, want %q", resp.AsOf, tc.wantAsOf)
			}
			// Every range ends on the horizon whether or not as_of is reported —
			// the two are the same day, and only the claim of coverage differs.
			for _, key := range []string{"7d", "14d", "30d", "90d"} {
				if got := resp.Ranges[key].Period.End; got != tc.wantEnd {
					t.Errorf("%s period end = %q, want %q", key, got, tc.wantEnd)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations: %v", err)
			}
		})
	}
}

// The Sources filter is the one dev-tool surface with its own query: every other
// one reads the source's spend through the deployment entries or the actor
// grain. Its contract is a presence gate plus the brand icon, which is what the
// client renders as the logo, so a ref built without one yields a filter with a
// blank row.
func TestRollupPresentDevtoolSourcesGatesOnUsage(t *testing.T) {
	ad := devtoolAdapters[0]
	window := insightsrollup.Window{
		From: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
	}

	for _, tc := range []struct {
		name        string
		cost        float64
		tokens      int64
		wantPresent bool
	}{
		{name: "used", cost: 2.5, tokens: 100, wantPresent: true},
		// Tokens with no cost means an unpriced model, not an unused source:
		// hiding it would lose the row, the filter entry and the token counts.
		{name: "unpriced but used", cost: 0, tokens: 21388, wantPresent: true},
		{name: "unused", cost: 0, tokens: 0, wantPresent: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer db.Close()

			for range devtoolAdapters {
				mock.ExpectQuery("COALESCE\\(SUM\\(cost_usd\\)").
					WillReturnRows(sqlmock.NewRows([]string{"cost_usd", "requests", "tokens"}).
						AddRow(tc.cost, 0, tc.tokens))
			}

			refs, err := rollupPresentDevtoolSources(context.Background(),
				insightsrollup.NewStore(db), &account.Account{ID: "acct_1"}, window)
			if err != nil {
				t.Fatalf("rollupPresentDevtoolSources: %v", err)
			}
			if !tc.wantPresent {
				if len(refs) != 0 {
					t.Fatalf("refs = %+v, want none", refs)
				}
				return
			}
			if len(refs) != 1 {
				t.Fatalf("refs = %+v, want one entry", refs)
			}
			if refs[0].Key != ad.Key || refs[0].Label != ad.Label || refs[0].Icon != ad.Icon {
				t.Errorf("ref = %+v, want key %q, label %q, icon %q",
					refs[0], ad.Key, ad.Label, ad.Icon)
			}
		})
	}
}
