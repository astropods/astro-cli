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
		EvalDatasetID:    "dataset-1",
		TraceID:          "trace-1",
		EvaluationRef:    "preset/default-evaluation",
		VerifiedByUserID: "user-1",
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

func TestGet(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("FROM eval_dataset_items").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(itemRows().AddRow("preset/default-evaluation", "run-1", "user-1"))

	got, err := store.Get(context.Background(), "dataset-1", "trace-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "preset/default-evaluation", got.EvaluationRef)
	require.NotNil(t, got.SourceEvaluationRunID)
	assert.Equal(t, "run-1", *got.SourceEvaluationRunID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetReturnsNilForATraceNotInTheDataset(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("FROM eval_dataset_items").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(itemRows())

	got, err := store.Get(context.Background(), "dataset-1", "trace-1")
	require.NoError(t, err)
	assert.Nil(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectReplaceOutputsStamp(mock sqlmock.Sqlmock, affected int64) {
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE eval_dataset_items").
		WithArgs("dataset-1", "trace-1", "user-2").
		WillReturnResult(sqlmock.NewResult(0, affected))
}

func TestReplaceOutputsSwapsTheValuesAndRecordsTheReviewer(t *testing.T) {
	store, mock := newStore(t)
	expectReplaceOutputsStamp(mock, 1)
	mock.ExpectExec("DELETE FROM eval_dataset_item_evaluator_outputs").
		WithArgs("dataset-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO eval_dataset_item_evaluator_outputs").
		WithArgs("dataset-1", "trace-1", `[{"key":"exposed_pii","value":true}]`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := store.ReplaceOutputs(context.Background(), "dataset-1", "trace-1", "user-2", []Output{
		{EvaluatorKey: "exposed_pii", Value: json.RawMessage(`true`)},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplaceOutputsRejectsATraceNotInTheDataset(t *testing.T) {
	store, mock := newStore(t)
	expectReplaceOutputsStamp(mock, 0)
	mock.ExpectRollback()

	err := store.ReplaceOutputs(context.Background(), "dataset-1", "trace-1", "user-2", []Output{
		{EvaluatorKey: "exposed_pii", Value: json.RawMessage(`true`)},
	})
	assert.ErrorIs(t, err, ErrItemNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectRemove(mock sqlmock.Sqlmock, outputs *sqlmock.Rows, deleted *sqlmock.Rows) {
	mock.ExpectBegin()
	mock.ExpectQuery("FROM eval_dataset_item_evaluator_outputs").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(outputs)
	mock.ExpectQuery("DELETE FROM eval_dataset_items").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(deleted)
}

func outputRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"evaluator_key", "value_json"})
}

func itemRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"evaluation_ref", "source_evaluation_run_id", "verified_by_user_id"})
}

func TestRemoveReturnsTheDeletedItemAndOutputs(t *testing.T) {
	store, mock := newStore(t)
	expectRemove(mock,
		outputRows().
			AddRow("exposed_pii", []byte(`false`)).
			AddRow("user_sentiment", []byte(`"negative"`)),
		itemRows().AddRow("preset/default-evaluation", "run-1", "user-1"))
	mock.ExpectCommit()

	removed, outputs, err := store.Remove(context.Background(), "dataset-1", "trace-1")
	require.NoError(t, err)
	assert.Equal(t, "preset/default-evaluation", removed.EvaluationRef)
	assert.Equal(t, "user-1", removed.VerifiedByUserID)
	require.NotNil(t, removed.SourceEvaluationRunID)
	assert.Equal(t, "run-1", *removed.SourceEvaluationRunID)
	assert.Equal(t, []Output{
		{EvaluatorKey: "exposed_pii", Value: json.RawMessage(`false`)},
		{EvaluatorKey: "user_sentiment", Value: json.RawMessage(`"negative"`)},
	}, outputs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRemoveWithoutASourceRun(t *testing.T) {
	store, mock := newStore(t)
	expectRemove(mock, outputRows(),
		itemRows().AddRow("preset/default-evaluation", nil, "user-1"))
	mock.ExpectCommit()

	removed, outputs, err := store.Remove(context.Background(), "dataset-1", "trace-1")
	require.NoError(t, err)
	assert.Nil(t, removed.SourceEvaluationRunID)
	assert.Empty(t, outputs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRemoveRejectsATraceNotInTheDataset(t *testing.T) {
	store, mock := newStore(t)
	expectRemove(mock, outputRows(), itemRows())
	mock.ExpectRollback()

	removed, _, err := store.Remove(context.Background(), "dataset-1", "trace-1")
	assert.ErrorIs(t, err, ErrItemNotFound)
	assert.Nil(t, removed)
	require.NoError(t, mock.ExpectationsWereMet())
}
