package evalitemstore

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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

func item() Item {
	return Item{
		EvalDatasetID: "dataset-1",
		TraceID:       "trace-1",
		EvaluationRef: "preset/default-evaluation",
		AddedByUserID: "user-1",
	}
}

func TestAddWritesItemAndOutputs(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO eval_dataset_items").
		WithArgs("dataset-1", "trace-1", "preset/default-evaluation", nil, "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO eval_dataset_item_evaluator_outputs").
		WithArgs("dataset-1", "trace-1",
			`[{"key":"exposed_pii","value":false},{"key":"user_sentiment","value":"negative"}]`).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err := store.Add(context.Background(), item(), []Output{
		{EvaluatorKey: "exposed_pii", Value: json.RawMessage(`false`)},
		{EvaluatorKey: "user_sentiment", Value: json.RawMessage(`"negative"`)},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAddStoresTheSourceRun(t *testing.T) {
	store, mock := newStore(t)
	runID := "run-1"
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO eval_dataset_items").
		WithArgs("dataset-1", "trace-1", "preset/default-evaluation", runID, "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO eval_dataset_item_evaluator_outputs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	added := item()
	added.SourceEvaluationRunID = &runID
	err := store.Add(context.Background(), added, []Output{
		{EvaluatorKey: "exposed_pii", Value: json.RawMessage(`true`)},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAddRejectsATraceAlreadyInTheDataset(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO eval_dataset_items").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err := store.Add(context.Background(), item(), []Output{
		{EvaluatorKey: "exposed_pii", Value: json.RawMessage(`true`)},
	})
	assert.ErrorIs(t, err, ErrAlreadyAdded)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAddRollsBackWhenOutputsFail(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO eval_dataset_items").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO eval_dataset_item_evaluator_outputs").
		WillReturnError(errors.New("outputs failed"))
	mock.ExpectRollback()

	err := store.Add(context.Background(), item(), []Output{
		{EvaluatorKey: "exposed_pii", Value: json.RawMessage(`true`)},
	})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrAlreadyAdded)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDelete(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("DELETE FROM eval_dataset_items").
		WithArgs("dataset-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, store.Delete(context.Background(), "dataset-1", "trace-1"))
	require.NoError(t, mock.ExpectationsWereMet())
}
