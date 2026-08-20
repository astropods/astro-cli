package classification

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

var (
	testDay  = time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	testTime = time.Date(2026, 8, 3, 14, 30, 0, 0, time.UTC)
)

func result(unitID string, axis Axis, label string) Result {
	return Result{
		UnitKind: UnitTurn, UnitID: unitID, Axis: axis, Label: label,
		Score: 0.9, OccurredAt: testTime, UserEmail: "dev@example.com",
	}
}

// Inference is the expensive resource, so an empty batch must not open a
// transaction at all.
func TestSaveResultsEmptyIsNoOp(t *testing.T) {
	store, mock := newMockStore(t)
	if err := store.SaveResults(context.Background(), "acct", SourceClaudeCode, "v1", nil); err != nil {
		t.Fatalf("SaveResults: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database calls: %v", err)
	}
}

// Without a version stamp there is no way to detect a retrain, so rows must
// never be written unversioned.
func TestSaveResultsRequiresModelVersion(t *testing.T) {
	store, mock := newMockStore(t)
	err := store.SaveResults(context.Background(), "acct", SourceClaudeCode, "", []Result{result("t1", AxisPurpose, "work")})
	if err == nil {
		t.Fatal("want error for empty model version")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database calls: %v", err)
	}
}

// The DB has a CHECK on axis; catching it in Go keeps a bad batch from aborting
// a transaction that also carries good rows.
func TestSaveResultsRejectsInvalidAxis(t *testing.T) {
	store, mock := newMockStore(t)
	err := store.SaveResults(context.Background(), "acct", SourceClaudeCode, "v1", []Result{
		result("t1", Axis("sentiment"), "positive"),
	})
	if err == nil {
		t.Fatal("want error for invalid axis")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database calls: %v", err)
	}
}

// Postgres refuses to let one statement touch the same row twice, and
// ON CONFLICT cannot rescue it — duplicates must be folded before insert.
func TestSaveResultsFoldsDuplicateUnitAxis(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO trace_classifications`).
		WithArgs(
			"acct", SourceClaudeCode, "turn", "t1", "purpose", "personal", 0.9, "v1", testTime, "dev@example.com",
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := store.SaveResults(context.Background(), "acct", SourceClaudeCode, "v1", []Result{
		result("t1", AxisPurpose, "work"),
		result("t1", AxisPurpose, "personal"), // same unit+axis; last wins
	})
	if err != nil {
		t.Fatalf("SaveResults: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// Two axes for one trace are distinct rows and must both survive folding.
func TestSaveResultsKeepsDistinctAxes(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO trace_classifications`).
		WithArgs(
			"acct", SourceClaudeCode, "turn", "t1", "purpose", "work", 0.9, "v1", testTime, "dev@example.com",
			"acct", SourceClaudeCode, "turn", "t1", "topic", "software-engineering", 0.9, "v1", testTime, "dev@example.com",
		).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err := store.SaveResults(context.Background(), "acct", SourceClaudeCode, "v1", []Result{
		result("t1", AxisPurpose, "work"),
		result("t1", AxisTopic, "software-engineering"),
	})
	if err != nil {
		t.Fatalf("SaveResults: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestClassifiedAxesEmptyInputSkipsQuery(t *testing.T) {
	store, mock := newMockStore(t)
	got, err := store.ClassifiedAxes(context.Background(), "acct", SourceClaudeCode, "v1", UnitTurn, nil)
	if err != nil {
		t.Fatalf("ClassifiedAxes: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database calls: %v", err)
	}
}

// A unit is only "done" for the axes actually stored, so a partially classified
// trace still gets its missing axis sent to inference.
func TestClassifiedAxesGroupsByUnit(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectQuery(`SELECT unit_id, axis FROM trace_classifications`).
		WillReturnRows(sqlmock.NewRows([]string{"unit_id", "axis"}).
			AddRow("t1", "purpose").
			AddRow("t1", "topic").
			AddRow("t2", "purpose"))

	got, err := store.ClassifiedAxes(context.Background(), "acct", SourceClaudeCode, "v1", UnitTurn, []string{"t1", "t2", "t3"})
	if err != nil {
		t.Fatalf("ClassifiedAxes: %v", err)
	}
	if !got["t1"][AxisPurpose] || !got["t1"][AxisTopic] {
		t.Errorf("t1 = %v, want both axes", got["t1"])
	}
	if !got["t2"][AxisPurpose] || got["t2"][AxisTopic] {
		t.Errorf("t2 = %v, want purpose only", got["t2"])
	}
	if _, ok := got["t3"]; ok {
		t.Errorf("t3 should be absent, got %v", got["t3"])
	}
}

// Full replace is what makes the pass idempotent; the delete must run even when
// there are no facts, so a day whose labels vanished does not keep stale rows.
func TestReplaceDayAggregatesDeletesWithNoFacts(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM insights_classification_daily`).
		WithArgs("acct", testDay, SourceClaudeCode).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	if err := store.ReplaceDayAggregates(context.Background(), "acct", testDay, SourceClaudeCode, nil); err != nil {
		t.Fatalf("ReplaceDayAggregates: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// Same dimension tuple arriving twice must sum, not collide on the primary key.
func TestReplaceDayAggregatesFoldsAndSums(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM insights_classification_daily`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO insights_classification_daily`).
		WithArgs("acct", testDay, SourceClaudeCode, "purpose", "work", "member", "member:u1", int64(7), 3.5).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := store.ReplaceDayAggregates(context.Background(), "acct", testDay, SourceClaudeCode, []DailyFact{
		{Axis: AxisPurpose, Label: "work", ActorKind: "member", ActorKey: "member:u1", Traces: 5, CostUSD: 2.0},
		{Axis: AxisPurpose, Label: "work", ActorKind: "member", ActorKey: "member:u1", Traces: 2, CostUSD: 1.5},
	})
	if err != nil {
		t.Fatalf("ReplaceDayAggregates: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// A day boundary must be a half-open UTC window, or traces land in two days.
func TestCountsForDayUsesHalfOpenUTCWindow(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectQuery(`SELECT axis, label, user_email, count\(\*\)`).
		WithArgs("acct", SourceClaudeCode, "turn", "v1", testDay, testDay.AddDate(0, 0, 1)).
		WillReturnRows(sqlmock.NewRows([]string{"axis", "label", "user_email", "count"}).
			AddRow("topic", "software-engineering", "dev@example.com", 12))

	got, err := store.CountsForDay(context.Background(), "acct", SourceClaudeCode, "v1", UnitTurn, testTime)
	if err != nil {
		t.Fatalf("CountsForDay: %v", err)
	}
	if len(got) != 1 || got[0].Traces != 12 || got[0].Axis != AxisTopic {
		t.Fatalf("got %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// The table keeps every generation of a retrain, so an unscoped tally would
// count a re-labelled trace once per version.
func TestCountsForDayRequiresModelVersion(t *testing.T) {
	store, _ := newMockStore(t)
	if _, err := store.CountsForDay(context.Background(), "acct", SourceClaudeCode, "", UnitTurn, testTime); err == nil {
		t.Fatal("an unversioned tally must be refused, not silently summed across generations")
	}
}

func TestGetStateNoRowsIsZeroValue(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectQuery(`SELECT classified_through, backfilled_from`).
		WillReturnRows(sqlmock.NewRows([]string{
			"classified_through", "backfilled_from", "backfill_complete",
			"last_run_at", "last_error", "consecutive_errors"}))

	got, err := store.GetState(context.Background(), "acct", SourceClaudeCode)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if got.ClassifiedThrough != nil || got.ConsecutiveErrors != 0 {
		t.Errorf("got %+v, want zero State", got)
	}
}

// A failed day must be retried, so the watermark must not advance on failure.
func TestFailureWithNoProgressLeavesWatermark(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectExec(`INSERT INTO classification_state`).
		WithArgs("acct", SourceClaudeCode, nil, nil, false, "boom", true).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.SetCursorsPartial(
		context.Background(), "acct", SourceClaudeCode, nil, nil, false, "boom", true)
	if err != nil {
		t.Fatalf("SetCursorsPartial: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSetCursorsTruncatesToDay(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectExec(`INSERT INTO classification_state`).
		WithArgs("acct", SourceClaudeCode, testDay, testDay, false, "", false).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.SetCursors(context.Background(), "acct", SourceClaudeCode, &testTime, &testTime, false); err != nil {
		t.Fatalf("SetCursors: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// A tick that only moved the backfill must not touch the forward edge.
func TestSetCursorsNilLeavesEdgeUntouched(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectExec(`INSERT INTO classification_state`).
		WithArgs("acct", SourceClaudeCode, nil, testDay, true, "", false).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.SetCursors(context.Background(), "acct", SourceClaudeCode, nil, &testTime, true); err != nil {
		t.Fatalf("SetCursors: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// A pass that covered some days and failed on others keeps its cursor and its
// error text, but not the backoff: one stuck day must not throttle the days
// around it that complete.
func TestSetCursorsPartialKeepsCadenceWhenThePassMoved(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectExec(`INSERT INTO classification_state`).
		WithArgs("acct", SourceClaudeCode, testDay, nil, false, "boom", false).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.SetCursorsPartial(
		context.Background(), "acct", SourceClaudeCode, &testTime, nil, false, "boom", false)
	if err != nil {
		t.Fatalf("SetCursorsPartial: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
