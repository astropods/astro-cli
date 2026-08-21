package insightsrollup

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db), mock
}

var testDay = time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)

// The zero Grain must be rejected rather than treated as "every grain": the
// table holds two descriptions of the same spend, so a missing grain filter
// would double-count.
func TestReplaceDayRejectsZeroGrain(t *testing.T) {
	store, mock := newMockStore(t)

	err := store.ReplaceDay(context.Background(), "acct_1", Grain(""), testDay, SourceAgents, []Fact{{}})
	if err == nil {
		t.Fatal("ReplaceDay with zero grain: want error, got nil")
	}
	// Nothing may reach the database on an invalid grain.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database calls: %v", err)
	}
}

// The delete must be scoped by grain as well as (account, day, source),
// otherwise a producer emitting one grain wipes the other's rows for that day.
func TestReplaceDayScopesDeleteByGrain(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM insights_usage_daily").
		WithArgs("acct_1", "usage", "2026-08-03", SourceAgents).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("INSERT INTO insights_usage_daily").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	facts := []Fact{{DeploymentID: "dep-1", ActorKind: ActorKindMember, ActorKey: "member:u1", Requests: 2}}
	if err := store.ReplaceDay(context.Background(), "acct_1", GrainUsage, testDay, SourceAgents, facts); err != nil {
		t.Fatalf("ReplaceDay: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// An empty fact slice still clears the day. A source that legitimately went to
// zero must not keep yesterday's rows alive.
func TestReplaceDayWithNoFactsStillClearsTheDay(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM insights_usage_daily").
		WithArgs("acct_1", "usage", "2026-08-03", "claude-code").
		WillReturnResult(sqlmock.NewResult(0, 5))
	mock.ExpectCommit()

	if err := store.ReplaceDay(context.Background(), "acct_1", GrainUsage, testDay, "claude-code", nil); err != nil {
		t.Fatalf("ReplaceDay: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// Langfuse groups by the whole `tags` array, so one deployment arrives as
// several groups ([deployment:x] and [deployment:x, env:prod]). They must be
// summed into one row: passing both through would violate the primary key, and
// ON CONFLICT can't rescue it because Postgres refuses to let one statement
// touch the same row twice.
func TestFoldFactsSumsDuplicateTagArrayGroups(t *testing.T) {
	older := testDay.Add(2 * time.Hour)
	newer := testDay.Add(9 * time.Hour)

	got := foldFacts([]Fact{
		{DeploymentID: "dep-1", ActorKey: "member:u1", Requests: 3, CostUSD: 1.50, TotalTokens: 100, LastSeenAt: newer},
		{DeploymentID: "dep-1", ActorKey: "member:u1", Requests: 2, CostUSD: 0.25, TotalTokens: 40, LastSeenAt: older},
		{DeploymentID: "dep-2", ActorKey: "member:u1", Requests: 7, CostUSD: 3.00, TotalTokens: 70, LastSeenAt: older},
	})

	if len(got) != 2 {
		t.Fatalf("folded rows = %d, want 2: %+v", len(got), got)
	}
	first := got[0]
	if first.DeploymentID != "dep-1" || first.ActorKey != "member:u1" {
		t.Fatalf("first row identity = %+v", first)
	}
	if first.Requests != 5 {
		t.Errorf("requests = %d, want 5", first.Requests)
	}
	if first.CostUSD != 1.75 {
		t.Errorf("cost = %v, want 1.75", first.CostUSD)
	}
	if first.TotalTokens != 140 {
		t.Errorf("tokens = %d, want 140", first.TotalTokens)
	}
	// last_seen must be the newest across folded rows, not the last one seen.
	if !first.LastSeenAt.Equal(newer) {
		t.Errorf("last seen = %v, want %v", first.LastSeenAt, newer)
	}
}

// Rows differing only by actor are separate people and must not be folded
// together, even under the same deployment.
func TestFoldFactsKeepsDistinctActorsApart(t *testing.T) {
	got := foldFacts([]Fact{
		{DeploymentID: "dep-1", ActorKind: ActorKindMember, ActorKey: "member:u1", CostUSD: 1},
		{DeploymentID: "dep-1", ActorKind: ActorKindSlack, ActorKey: "slack:T1:U1", CostUSD: 2},
	})
	if len(got) != 2 {
		t.Fatalf("folded rows = %d, want 2", len(got))
	}
}

// A missing state row is the initial state, not an error — a brand-new account
// has simply never been rolled up.
func TestStateMissingRowIsZeroValue(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectQuery("SELECT rolled_up_through").
		WithArgs("acct_new", SourceAgents).
		WillReturnRows(sqlmock.NewRows([]string{"rolled_up_through", "last_run_at", "last_error", "consecutive_errors"}))

	st, err := store.State(context.Background(), "acct_new", SourceAgents)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if !st.RolledUpThrough.IsZero() {
		t.Errorf("RolledUpThrough = %v, want zero", st.RolledUpThrough)
	}
	if st.ConsecutiveErrors != 0 {
		t.Errorf("ConsecutiveErrors = %d, want 0", st.ConsecutiveErrors)
	}
}

// A NULL watermark with a recorded failure reads back as "never rolled up, and
// here's why" rather than as a successful empty roll-up.
func TestStateNullWatermarkWithError(t *testing.T) {
	store, mock := newMockStore(t)
	rows := sqlmock.NewRows([]string{"rolled_up_through", "last_run_at", "last_error", "consecutive_errors"}).
		AddRow(nil, testDay, "langfuse unavailable", 3)
	mock.ExpectQuery("SELECT rolled_up_through").
		WithArgs("acct_1", SourceAgents).
		WillReturnRows(rows)

	st, err := store.State(context.Background(), "acct_1", SourceAgents)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if !st.RolledUpThrough.IsZero() {
		t.Errorf("RolledUpThrough = %v, want zero", st.RolledUpThrough)
	}
	if st.LastError != "langfuse unavailable" || st.ConsecutiveErrors != 3 {
		t.Errorf("error state = %q/%d", st.LastError, st.ConsecutiveErrors)
	}
}

// RecordFailure must not touch rolled_up_through: holding the watermark back is
// what stops the page claiming coverage the facts don't support.
func TestRecordFailurePreservesWatermark(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectExec("INSERT INTO insights_rollup_state").
		WithArgs("acct_1", SourceAgents, "boom").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.RecordFailure(context.Background(), "acct_1", SourceAgents, "boom"); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// RecordProgress must leave the error columns alone. Advance resets them, and a
// run that rolled some days has not earned that reset: if partial progress
// cleared consecutive_errors, an account failing on the same day forever would
// hold its counter at zero and never look unhealthy.
func TestRecordProgressLeavesErrorStateAlone(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectExec("INSERT INTO insights_rollup_state").
		WithArgs("acct_1", SourceAgents, "2026-06-20").
		WillReturnResult(sqlmock.NewResult(0, 1))

	day := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	if err := store.RecordProgress(context.Background(), "acct_1", SourceAgents, day); err != nil {
		t.Fatalf("RecordProgress: %v", err)
	}
	// Three bound arguments and no reason string is the observable difference
	// from RecordFailure and Advance, both of which write the error columns.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
