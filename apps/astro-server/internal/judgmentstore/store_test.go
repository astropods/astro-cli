package judgmentstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestIsJudged(t *testing.T) {
	for _, tt := range []struct {
		name   string
		judged bool
	}{
		{name: "not judged", judged: false},
		{name: "judged", judged: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			mock.ExpectQuery("SELECT EXISTS").
				WithArgs("dataset-1", "trace-1").
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(tt.judged))

			got, err := NewStore(db).IsJudged(context.Background(), "dataset-1", "trace-1")
			if err != nil {
				t.Fatalf("IsJudged: %v", err)
			}
			if got != tt.judged {
				t.Fatalf("IsJudged = %v, want %v", got, tt.judged)
			}
		})
	}
}

func TestIsJudgedReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnError(errors.New("query failed"))

	_, err = NewStore(db).IsJudged(context.Background(), "dataset-1", "trace-1")
	if err == nil || !strings.Contains(err.Error(), "query failed") {
		t.Fatalf("IsJudged error = %v", err)
	}
}

func TestGetPredictions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	createdAt := time.Date(2026, time.July, 21, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	columns := []string{
		"trace_id", "verdict_score", "confidence", "explanation", "judge_version",
		"created_at", "updated_at", "dimension_key", "dimension_value",
	}
	mock.ExpectQuery("FROM eval_dataset_judgment_predictions").
		WithArgs("dataset-1", pq.Array([]string{"trace-2", "trace-1"})).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("trace-1", -0.75, 91, "Misses a required constraint.", "dataset-review-v1", createdAt, updatedAt, "accuracy", -0.9).
			AddRow("trace-1", -0.75, 91, "Misses a required constraint.", "dataset-review-v1", createdAt, updatedAt, "tone", -0.2).
			AddRow("trace-2", 0.4, 62, "Useful response.", "dataset-review-v1", createdAt, createdAt, nil, nil))

	store := NewStore(db)
	got, err := store.GetPredictions(context.Background(), "dataset-1", []string{"trace-2", "trace-1"})
	if err != nil {
		t.Fatalf("GetPredictions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetPredictions len = %d, want 2", len(got))
	}

	first := got["trace-1"]
	if first.VerdictScore != -0.75 || first.Confidence != 91 || first.Explanation != "Misses a required constraint." || first.JudgeVersion != "dataset-review-v1" {
		t.Fatalf("trace-1 prediction = %+v", first)
	}
	if !first.CreatedAt.Equal(createdAt) || !first.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("trace-1 timestamps = %v/%v, want %v/%v", first.CreatedAt, first.UpdatedAt, createdAt, updatedAt)
	}
	wantCriteria := []PredictionCriterion{
		{Dimension: DimensionAccuracy, Value: -0.9},
		{Dimension: DimensionTone, Value: -0.2},
	}
	if len(first.Criteria) != len(wantCriteria) {
		t.Fatalf("trace-1 criteria = %+v, want %+v", first.Criteria, wantCriteria)
	}
	for i := range wantCriteria {
		if first.Criteria[i] != wantCriteria[i] {
			t.Errorf("trace-1 criteria[%d] = %+v, want %+v", i, first.Criteria[i], wantCriteria[i])
		}
	}

	second := got["trace-2"]
	if second.Criteria != nil {
		t.Fatalf("trace-2 criteria = %+v, want nil", second.Criteria)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGetPredictionsEmptyTraceIDsDoesNotQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStore(db)
	got, err := store.GetPredictions(context.Background(), "dataset-1", nil)
	if err != nil {
		t.Fatalf("GetPredictions: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetPredictions = %+v, want empty map", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPredictionTracesWithoutJudgmentsExcludesJudgmentsBeforeLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	asOf := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	before := &PredictionTrace{
		TraceID:        "trace-2",
		TraceTimestamp: asOf.Add(-time.Hour),
	}
	nextTimestamp := asOf.Add(-2 * time.Hour)
	mock.ExpectQuery("(?s)SELECT p.trace_id, p.trace_timestamp.*NOT EXISTS.*FROM eval_dataset_judgments j.*ORDER BY p.trace_timestamp DESC, p.trace_id DESC").
		WithArgs(
			"dataset-1",
			asOf,
			before.TraceTimestamp,
			before.TraceID,
			2,
		).
		WillReturnRows(sqlmock.NewRows([]string{"trace_id", "ordering_timestamp"}).
			AddRow("trace-1", nextTimestamp))

	got, err := NewStore(db).PredictionTracesWithoutJudgments(
		context.Background(),
		"dataset-1",
		asOf,
		before,
		2,
	)
	if err != nil {
		t.Fatalf("PredictionTracesWithoutJudgments: %v", err)
	}
	if len(got) != 1 || got[0] != (PredictionTrace{TraceID: "trace-1", TraceTimestamp: nextTimestamp}) {
		t.Fatalf("traces = %v", got)
	}
}

func TestPredictionTracesWithoutJudgmentsRejectsInvalidLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = NewStore(db).PredictionTracesWithoutJudgments(
		context.Background(),
		"dataset-1",
		time.Now(),
		nil,
		0,
	)
	if err == nil {
		t.Fatal("PredictionTracesWithoutJudgments error = nil, want invalid limit")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected query: %v", err)
	}
}

func TestUpsertPredictionReplacesCriteria(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	traceTimestamp := time.Date(2026, time.July, 20, 9, 0, 0, 0, time.UTC)
	mock.ExpectExec(`(?s)INSERT INTO eval_dataset_judgment_predictions.*ON CONFLICT.*updated_at = now\(\)`).
		WithArgs("dataset-1", "trace-1", traceTimestamp, -0.6, 88, "Incorrect result.", "dataset-review-v1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM eval_dataset_judgment_prediction_criteria").
		WithArgs("dataset-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO eval_dataset_judgment_prediction_criteria").
		WithArgs(
			"dataset-1",
			"trace-1",
			pq.Array([]string{"accuracy", "completeness"}),
			pq.Array([]float64{-0.8, -0.4}),
		).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	store := NewStore(db)
	err = store.UpsertPrediction(context.Background(), "dataset-1", "trace-1", Prediction{
		TraceTimestamp: traceTimestamp,
		VerdictScore:   -0.6,
		Confidence:     88,
		Explanation:    "Incorrect result.",
		JudgeVersion:   "dataset-review-v1",
		Criteria: []PredictionCriterion{
			{Dimension: DimensionAccuracy, Value: -0.8},
			{Dimension: DimensionCompleteness, Value: -0.4},
		},
	})
	if err != nil {
		t.Fatalf("UpsertPrediction: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUpsertPredictionClearsCriteria(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	traceTimestamp := time.Date(2026, time.July, 20, 9, 0, 0, 0, time.UTC)
	mock.ExpectExec("INSERT INTO eval_dataset_judgment_predictions").
		WithArgs("dataset-1", "trace-1", traceTimestamp, 0.1, 50, "Unclear.", "dataset-review-v2").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM eval_dataset_judgment_prediction_criteria").
		WithArgs("dataset-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	store := NewStore(db)
	if err := store.UpsertPrediction(context.Background(), "dataset-1", "trace-1", Prediction{
		TraceTimestamp: traceTimestamp,
		VerdictScore:   0.1,
		Confidence:     50,
		Explanation:    "Unclear.",
		JudgeVersion:   "dataset-review-v2",
	}); err != nil {
		t.Fatalf("UpsertPrediction: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUpsertPredictionRequiresTraceTimestamp(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	err = NewStore(db).UpsertPrediction(
		context.Background(),
		"dataset-1",
		"trace-1",
		Prediction{JudgeVersion: "dataset-review-v1"},
	)
	if err == nil || !strings.Contains(err.Error(), "trace timestamp is required") {
		t.Fatalf("UpsertPrediction error = %v, want required timestamp", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected query: %v", err)
	}
}

func TestUpsertPredictionRollsBackOnWriteFailures(t *testing.T) {
	tests := []struct {
		name  string
		setup func(sqlmock.Sqlmock)
	}{
		{
			name: "prediction",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO eval_dataset_judgment_predictions").WillReturnError(errors.New("prediction failed"))
			},
		},
		{
			name: "criteria delete",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO eval_dataset_judgment_predictions").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("DELETE FROM eval_dataset_judgment_prediction_criteria").WillReturnError(errors.New("delete failed"))
			},
		},
		{
			name: "criteria insert",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO eval_dataset_judgment_predictions").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("DELETE FROM eval_dataset_judgment_prediction_criteria").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("INSERT INTO eval_dataset_judgment_prediction_criteria").WillReturnError(errors.New("insert failed"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			mock.ExpectBegin()
			tt.setup(mock)
			mock.ExpectRollback()

			store := NewStore(db)
			err = store.UpsertPrediction(context.Background(), "dataset-1", "trace-1", Prediction{
				TraceTimestamp: time.Date(2026, time.July, 20, 9, 0, 0, 0, time.UTC),
				JudgeVersion:   "dataset-review-v1",
				Criteria: []PredictionCriterion{
					{Dimension: DimensionAccuracy, Value: -0.5},
				},
			})
			if err == nil {
				t.Fatal("UpsertPrediction error = nil, want write error")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet expectations: %v", err)
			}
		})
	}
}

func TestUpsertPredictionReturnsCommitError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO eval_dataset_judgment_predictions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM eval_dataset_judgment_prediction_criteria").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))

	store := NewStore(db)
	err = store.UpsertPrediction(context.Background(), "dataset-1", "trace-1", Prediction{
		TraceTimestamp: time.Date(2026, time.July, 20, 9, 0, 0, 0, time.UTC),
		JudgeVersion:   "dataset-review-v1",
	})
	if err == nil || !strings.Contains(err.Error(), "commit failed") {
		t.Fatalf("UpsertPrediction error = %v, want commit error", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertRejectsInvalidVerdict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := NewStore(db)
	err = store.Insert("dataset-1", "trace-1", Verdict("maybe"))
	if err == nil || !strings.Contains(err.Error(), `invalid verdict "maybe"`) {
		t.Fatalf("Insert error = %v, want invalid verdict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertReturnsAlreadyJudgedOnConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("INSERT INTO eval_dataset_judgments").
		WithArgs("dataset-1", "trace-1", "good").
		WillReturnResult(sqlmock.NewResult(0, 0))

	store := NewStore(db)
	if err := store.Insert("dataset-1", "trace-1", VerdictGood); err != ErrAlreadyJudged {
		t.Fatalf("Insert error = %v, want ErrAlreadyJudged", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDelete(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("DELETE FROM eval_dataset_judgments").
		WithArgs("dataset-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := NewStore(db)
	if err := store.Delete("dataset-1", "trace-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDeleteReturningVerdict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("DELETE FROM eval_dataset_judgments").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow("bad"))

	store := NewStore(db)
	verdict, found, err := store.DeleteReturningVerdict("dataset-1", "trace-1")
	if err != nil {
		t.Fatalf("DeleteReturningVerdict: %v", err)
	}
	if !found {
		t.Fatal("DeleteReturningVerdict found = false, want true")
	}
	if verdict != VerdictBad {
		t.Fatalf("DeleteReturningVerdict verdict = %q, want bad", verdict)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDeleteReturningVerdictNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("DELETE FROM eval_dataset_judgments").
		WithArgs("dataset-1", "trace-missing").
		WillReturnError(sql.ErrNoRows)

	store := NewStore(db)
	_, found, err := store.DeleteReturningVerdict("dataset-1", "trace-missing")
	if err != nil {
		t.Fatalf("DeleteReturningVerdict: %v", err)
	}
	if found {
		t.Fatal("DeleteReturningVerdict found = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSetVerdictAndReasonsClearsOnChange(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("WITH previous AS").
		WithArgs("dataset-1", "trace-1", "bad").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow("good"))
	mock.ExpectQuery("DELETE FROM eval_dataset_judgment_reasons").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"dimension_key", "dimension_value"}).
			AddRow("accuracy", 1.0).
			AddRow("tone", 1.0))
	mock.ExpectCommit()

	store := NewStore(db)
	previous, replaced, found, err := store.SetVerdictAndReasons("dataset-1", "trace-1", VerdictBad, nil)
	if err != nil {
		t.Fatalf("SetVerdictAndReasons: %v", err)
	}
	if !found || previous != VerdictGood {
		t.Fatalf("found=%v previous=%q, want true/good", found, previous)
	}
	want := []Reason{{Dimension: DimensionAccuracy, Value: 1}, {Dimension: DimensionTone, Value: 1}}
	if len(replaced) != len(want) {
		t.Fatalf("replaced len = %d, want %d", len(replaced), len(want))
	}
	for i, r := range replaced {
		if r != want[i] {
			t.Errorf("replaced[%d] = %+v, want %+v", i, r, want[i])
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSetVerdictAndReasonsReplacesWithGivenReasons(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("WITH previous AS").
		WithArgs("dataset-1", "trace-1", "good").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow("bad"))
	mock.ExpectQuery("DELETE FROM eval_dataset_judgment_reasons").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"dimension_key", "dimension_value"}))
	mock.ExpectExec("INSERT INTO eval_dataset_judgment_reasons").
		WithArgs("dataset-1", "trace-1", pq.Array([]string{"accuracy"}), pq.Array([]float64{1})).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	store := NewStore(db)
	previous, _, found, err := store.SetVerdictAndReasons("dataset-1", "trace-1", VerdictGood, []Reason{{Dimension: DimensionAccuracy, Value: 1}})
	if err != nil {
		t.Fatalf("SetVerdictAndReasons: %v", err)
	}
	if !found || previous != VerdictBad {
		t.Fatalf("found=%v previous=%q, want true/bad", found, previous)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSetVerdictAndReasonsSameVerdictLeavesReasons(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("WITH previous AS").
		WithArgs("dataset-1", "trace-1", "good").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow("good"))
	mock.ExpectCommit()

	store := NewStore(db)
	previous, replaced, found, err := store.SetVerdictAndReasons("dataset-1", "trace-1", VerdictGood, nil)
	if err != nil {
		t.Fatalf("SetVerdictAndReasons: %v", err)
	}
	if !found || previous != VerdictGood || replaced != nil {
		t.Fatalf("found=%v previous=%q replaced=%+v, want true/good/nil", found, previous, replaced)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSetVerdictAndReasonsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("WITH previous AS").
		WithArgs("dataset-1", "trace-missing", "bad").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	store := NewStore(db)
	_, _, found, err := store.SetVerdictAndReasons("dataset-1", "trace-missing", VerdictBad, nil)
	if err != nil {
		t.Fatalf("SetVerdictAndReasons: %v", err)
	}
	if found {
		t.Fatal("found = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestReplaceReasonsReplaces(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT verdict FROM eval_dataset_judgments").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow("bad"))
	mock.ExpectQuery("DELETE FROM eval_dataset_judgment_reasons").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"dimension_key", "dimension_value"}).
			AddRow("accuracy", -1.0))
	mock.ExpectExec("INSERT INTO eval_dataset_judgment_reasons").
		WithArgs("dataset-1", "trace-1", pq.Array([]string{"tone"}), pq.Array([]float64{-0.5})).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	store := NewStore(db)
	// A non-polar value is stored as given (not derived from the verdict).
	verdict, previous, found, err := store.ReplaceReasons("dataset-1", "trace-1", []Reason{{Dimension: DimensionTone, Value: -0.5}})
	if err != nil {
		t.Fatalf("ReplaceReasons: %v", err)
	}
	if !found || verdict != VerdictBad {
		t.Fatalf("found=%v verdict=%q, want true/bad", found, verdict)
	}
	want := []Reason{{Dimension: DimensionAccuracy, Value: -1}}
	if len(previous) != len(want) || previous[0] != want[0] {
		t.Fatalf("previous = %+v, want %+v", previous, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestReplaceReasonsClears(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT verdict FROM eval_dataset_judgments").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow("good"))
	mock.ExpectQuery("DELETE FROM eval_dataset_judgment_reasons").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"dimension_key", "dimension_value"}).
			AddRow("accuracy", 1.0))
	mock.ExpectCommit()

	store := NewStore(db)
	verdict, previous, found, err := store.ReplaceReasons("dataset-1", "trace-1", nil)
	if err != nil {
		t.Fatalf("ReplaceReasons: %v", err)
	}
	if !found || verdict != VerdictGood || len(previous) != 1 {
		t.Fatalf("found=%v verdict=%q previous=%+v, want true/good/1 prev", found, verdict, previous)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestReplaceReasonsUnknownVerdictDoesNotModify(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT verdict FROM eval_dataset_judgments").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow("unknown"))
	mock.ExpectRollback()

	store := NewStore(db)
	verdict, previous, found, err := store.ReplaceReasons("dataset-1", "trace-1", []Reason{{Dimension: DimensionTone, Value: -1}})
	if err != nil {
		t.Fatalf("ReplaceReasons: %v", err)
	}
	if !found || verdict != VerdictUnknown || previous != nil {
		t.Fatalf("found=%v verdict=%q previous=%+v, want true/unknown/nil", found, verdict, previous)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestReplaceReasonsNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT verdict FROM eval_dataset_judgments").
		WithArgs("dataset-1", "trace-missing").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	store := NewStore(db)
	_, _, found, err := store.ReplaceReasons("dataset-1", "trace-missing", nil)
	if err != nil {
		t.Fatalf("ReplaceReasons: %v", err)
	}
	if found {
		t.Fatal("found = true, want false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCriterionDimensionValid(t *testing.T) {
	for _, d := range CriterionDimensions {
		if !d.Valid() {
			t.Errorf("CriterionDimensions entry %q is not Valid()", d)
		}
	}
	if CriterionDimension("nonsense").Valid() {
		t.Fatal("unknown dimension reported Valid() = true")
	}
}

func TestCriterionCounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("FROM eval_dataset_judgment_reasons").
		WithArgs("dataset-1").
		WillReturnRows(sqlmock.NewRows([]string{"dimension_key", "good_count", "bad_count"}).
			AddRow("accuracy", 2, 1).
			AddRow("tone", 0, 3))

	store := NewStore(db)
	counts, err := store.CriterionCounts("dataset-1")
	if err != nil {
		t.Fatalf("CriterionCounts: %v", err)
	}
	want := []CriterionCounts{
		{Dimension: DimensionAccuracy, GoodCount: 2, BadCount: 1},
		{Dimension: DimensionTone, GoodCount: 0, BadCount: 3},
	}
	if len(counts) != len(want) {
		t.Fatalf("CriterionCounts len = %d, want %d", len(counts), len(want))
	}
	for i, count := range counts {
		if count != want[i] {
			t.Errorf("CriterionCounts[%d] = %+v, want %+v", i, count, want[i])
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
