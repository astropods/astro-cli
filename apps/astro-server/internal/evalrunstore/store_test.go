package evalrunstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db), mock
}

var traceTime = time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

func TestStartRunClaimsTheRunTheRequestRecorded(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("UPDATE eval_dataset_evaluation_runs").
		WithArgs("dataset-1", "trace-1", "preset/default-evaluation").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("run-1"))

	run, err := store.StartRun(context.Background(), "dataset-1", "trace-1", "preset/default-evaluation")
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, "run-1", run.ID)
	assert.Equal(t, StatusInProgress, run.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStartRunReturnsNothingWithoutAnActiveRun(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("UPDATE eval_dataset_evaluation_runs").
		WillReturnError(sql.ErrNoRows)

	run, err := store.StartRun(context.Background(), "dataset-1", "trace-1", "ref")
	require.NoError(t, err)
	assert.Nil(t, run, "the request records the run, so a worker never invents one")
}

func TestStartRunFailsWhenTheUpdateErrors(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("UPDATE eval_dataset_evaluation_runs").
		WillReturnError(errors.New("deadlock detected"))

	_, err := store.StartRun(context.Background(), "dataset-1", "trace-1", "ref")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start run")
}

func TestCreateResultsSeedsOneRowPerEvaluator(t *testing.T) {
	store, mock := newStore(t)
	keys := []string{"exposed_pii", "user_sentiment"}
	mock.ExpectExec("INSERT INTO eval_dataset_evaluator_results").
		WithArgs("run-1", pq.Array(keys)).
		WillReturnResult(sqlmock.NewResult(0, 2))

	require.NoError(t, store.CreateResults(context.Background(), "run-1", keys))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateResultsSkipsEmptySet(t *testing.T) {
	store, mock := newStore(t)
	require.NoError(t, store.CreateResults(context.Background(), "run-1", nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCompleteResultStoresTypedValue(t *testing.T) {
	cases := map[string]struct {
		value any
		want  string
	}{
		"boolean": {value: true, want: "true"},
		"enum":    {value: "negative", want: `"negative"`},
		"number":  {value: 0.25, want: "0.25"},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			store, mock := newStore(t)
			mock.ExpectExec("UPDATE eval_dataset_evaluator_results").
				WithArgs("run-1", "key", []byte(testCase.want), 0.9, "because").
				WillReturnResult(sqlmock.NewResult(0, 1))

			require.NoError(t, store.CompleteResult(context.Background(), "run-1", Result{
				EvaluatorKey: "key",
				Value:        testCase.value,
				Confidence:   0.9,
				Explanation:  "because",
			}))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCompleteResultFailsWhenRowMissing(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("UPDATE eval_dataset_evaluator_results").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := store.CompleteResult(context.Background(), "run-1", Result{EvaluatorKey: "gone", Value: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestFailResultRecordsMessage(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("UPDATE eval_dataset_evaluator_results").
		WithArgs("run-1", "key", "model refused").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, store.FailResult(context.Background(), "run-1", "key", "model refused"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMarkResultInProgress(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("UPDATE eval_dataset_evaluator_results").
		WithArgs("run-1", "key").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, store.MarkResultInProgress(context.Background(), "run-1", "key"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFailPendingResultsClosesOutNonTerminalRows(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("UPDATE eval_dataset_evaluator_results").
		WithArgs("run-1", "run failed").
		WillReturnResult(sqlmock.NewResult(0, 3))

	require.NoError(t, store.FailPendingResults(context.Background(), "run-1", "run failed"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFailPendingResultsToleratesNoPendingRows(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("UPDATE eval_dataset_evaluator_results").
		WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, store.FailPendingResults(context.Background(), "run-1", "run failed"))
}

func TestCompletedResultKeysReturnsOnlyCompleted(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("SELECT evaluator_key").
		WithArgs("run-1").
		WillReturnRows(sqlmock.NewRows([]string{"evaluator_key"}).
			AddRow("exposed_pii").
			AddRow("user_sentiment"))

	keys, err := store.CompletedResultKeys(context.Background(), "run-1")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"exposed_pii": true, "user_sentiment": true}, keys)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFailQueuedRunsSkipsTheStoreWithoutTraces(t *testing.T) {
	store, mock := newStore(t)

	require.NoError(t, store.FailQueuedRuns(context.Background(), "dataset-1", "ref-1", nil, "enqueue failed"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFailQueuedRunsLeavesAdoptedRuns(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("UPDATE eval_dataset_evaluation_runs").
		WithArgs("dataset-1", "ref-1", pq.Array([]string{"trace-1"}), "enqueue failed").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, store.FailQueuedRuns(
		context.Background(),
		"dataset-1",
		"ref-1",
		[]string{"trace-1"},
		"enqueue failed",
	))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFinalizeRunWritesTerminalStatus(t *testing.T) {
	for _, status := range []Status{StatusCompleted, StatusFailed} {
		t.Run(string(status), func(t *testing.T) {
			store, mock := newStore(t)
			mock.ExpectExec("UPDATE eval_dataset_evaluation_runs").
				WithArgs("run-1", string(status), nil).
				WillReturnResult(sqlmock.NewResult(0, 1))

			require.NoError(t, store.FinalizeRun(context.Background(), "run-1", status, nil))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestFinalizeRunRejectsNonTerminalStatus(t *testing.T) {
	store, _ := newStore(t)
	for _, status := range []Status{StatusQueued, StatusInProgress} {
		err := store.FinalizeRun(context.Background(), "run-1", status, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not terminal")
	}
}

func TestCreateQueuedRunsInsertsOneRowPerTrace(t *testing.T) {
	store, mock := newStore(t)
	traces := []RunTrace{
		{TraceID: "trace-1", TraceTimestamp: traceTime},
		{TraceID: "trace-2", TraceTimestamp: traceTime},
	}
	mock.ExpectQuery("INSERT INTO eval_dataset_evaluation_runs").
		WithArgs("dataset-1", "preset/default-evaluation",
			pq.Array([]string{"trace-1", "trace-2"}), pq.Array([]time.Time{traceTime, traceTime})).
		WillReturnRows(sqlmock.NewRows([]string{"trace_id"}).AddRow("trace-1").AddRow("trace-2"))

	got, err := store.CreateQueuedRuns(context.Background(), "dataset-1", "preset/default-evaluation", traces)
	require.NoError(t, err)
	assert.Equal(t, []string{"trace-1", "trace-2"}, got, "an active run is returned too, so a re-request is idempotent")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateQueuedRunsSkipsEmptyBatch(t *testing.T) {
	store, _ := newStore(t)
	got, err := store.CreateQueuedRuns(context.Background(), "dataset-1", "ref", nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestStatusCountsGroupsByStatus(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery(`DISTINCT ON \(trace_id\) status.*ORDER BY trace_id, created_at DESC`).
		WithArgs("dataset-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "count"}).
			AddRow("queued", 2).
			AddRow("in_progress", 1).
			AddRow("completed", 7).
			AddRow("failed", 3))

	counts, err := store.StatusCounts(context.Background(), "dataset-1")
	require.NoError(t, err)
	assert.Equal(t, StatusCounts{Queued: 2, InProgress: 1, Completed: 7, Failed: 3}, counts)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStatusCountsIgnoresUnknownStatus(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("SELECT status, COUNT").
		WithArgs("dataset-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "count"}).AddRow("something_else", 4))

	counts, err := store.StatusCounts(context.Background(), "dataset-1")
	require.NoError(t, err)
	assert.Equal(t, StatusCounts{}, counts)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLatestRunsReturnsOneRunPerTrace(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("DISTINCT ON").
		WithArgs("dataset-1", pq.Array([]string{"trace-1"})).
		WillReturnRows(sqlmock.NewRows([]string{"trace_id", "id", "evaluation_ref", "status", "error_message"}).
			AddRow("trace-1", "run-1", "preset/default-evaluation", "completed", nil))

	got, err := store.LatestRuns(context.Background(), "dataset-1", []string{"trace-1"})
	require.NoError(t, err)
	require.Contains(t, got, "trace-1")
	assert.Equal(t, StatusCompleted, got["trace-1"].Status)
	assert.Empty(t, got["trace-1"].ErrorMessage)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTracesWithCompletedRunsPagesByTimestampThenTrace(t *testing.T) {
	store, mock := newStore(t)
	before := &RunTrace{TraceID: "trace-9", TraceTimestamp: traceTime}
	mock.ExpectQuery(`DISTINCT ON \(trace_id\) trace_id, trace_timestamp, status.*WHERE status = 'completed'`).
		WithArgs("dataset-1", traceTime.Add(-time.Hour), traceTime, traceTime, "trace-9", 3).
		WillReturnRows(sqlmock.NewRows([]string{"trace_id", "trace_timestamp"}).
			AddRow("trace-2", traceTime))

	got, err := store.TracesWithCompletedRuns(context.Background(), "dataset-1", traceTime.Add(-time.Hour), traceTime, before, 3)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "trace-2", got[0].TraceID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEvaluatorResultsReturnsEveryRow(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("FROM eval_dataset_evaluator_results").
		WithArgs("run-1").
		WillReturnRows(sqlmock.NewRows([]string{"evaluator_key", "status", "value_json", "confidence", "explanation", "error_message"}).
			AddRow("exposed_pii", "completed", []byte("true"), 0.9, "found none", nil).
			AddRow("user_sentiment", "failed", nil, nil, nil, "bad output"))

	got, err := store.EvaluatorResults(context.Background(), "run-1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, true, got[0].Value)
	assert.InDelta(t, 0.9, got[0].Confidence, 0.001)
	assert.Equal(t, StatusFailed, got[1].Status)
	assert.Equal(t, "bad output", got[1].ErrorMessage)
	require.NoError(t, mock.ExpectationsWereMet())
}
