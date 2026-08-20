package evalrunstore

import (
	"context"
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

func TestEnsureRunInsertsNewAttempt(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("INSERT INTO eval_dataset_evaluation_runs").
		WithArgs("dataset-1", "trace-1", traceTime, "preset/default-evaluation").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow("run-1", "in_progress"))

	run, err := store.EnsureRun(context.Background(), "dataset-1", "trace-1", "preset/default-evaluation", traceTime)
	require.NoError(t, err)
	assert.Equal(t, "run-1", run.ID)
	assert.Equal(t, StatusInProgress, run.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// A live attempt is adopted in the same statement, so its id and status come
// back from the conflicting row rather than a follow-up read.
func TestEnsureRunAdoptsActiveAttemptOnConflict(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("ON CONFLICT").
		WithArgs("dataset-1", "trace-1", traceTime, "preset/default-evaluation").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow("run-existing", "queued"))

	run, err := store.EnsureRun(context.Background(), "dataset-1", "trace-1", "preset/default-evaluation", traceTime)
	require.NoError(t, err)
	assert.Equal(t, "run-existing", run.ID)
	assert.Equal(t, StatusQueued, run.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEnsureRunFailsWhenInsertErrors(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("INSERT INTO eval_dataset_evaluation_runs").
		WillReturnError(errors.New("deadlock detected"))

	_, err := store.EnsureRun(context.Background(), "dataset-1", "trace-1", "ref", traceTime)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ensure run")
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
