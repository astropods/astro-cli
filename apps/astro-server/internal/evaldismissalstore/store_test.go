package evaldismissalstore

import (
	"context"
	"errors"
	"testing"

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

func TestDismissInsertsTheRow(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("INSERT INTO eval_dataset_dismissed_traces").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"is_item"}).AddRow(false))

	require.NoError(t, store.Dismiss(context.Background(), "dataset-1", "trace-1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDismissIsIdempotent(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("INSERT INTO eval_dataset_dismissed_traces").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"is_item"}).AddRow(false))

	require.NoError(t, store.Dismiss(context.Background(), "dataset-1", "trace-1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDismissRejectsADatasetItem(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("INSERT INTO eval_dataset_dismissed_traces").
		WithArgs("dataset-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"is_item"}).AddRow(true))

	err := store.Dismiss(context.Background(), "dataset-1", "trace-1")
	assert.True(t, errors.Is(err, ErrIsDatasetItem))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRestoreDeletesTheRow(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("DELETE FROM eval_dataset_dismissed_traces").
		WithArgs("dataset-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, store.Restore(context.Background(), "dataset-1", "trace-1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRestoreSucceedsWhenNotDismissed(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("DELETE FROM eval_dataset_dismissed_traces").
		WithArgs("dataset-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, store.Restore(context.Background(), "dataset-1", "trace-1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDismissedTraceIDsReturnsTheSubset(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("FROM eval_dataset_dismissed_traces").
		WithArgs("dataset-1", pq.Array([]string{"trace-1", "trace-2"})).
		WillReturnRows(sqlmock.NewRows([]string{"trace_id"}).AddRow("trace-2"))

	dismissed, err := store.DismissedTraceIDs(context.Background(), "dataset-1", []string{"trace-1", "trace-2"})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"trace-2": true}, dismissed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDismissedTraceIDsSkipsAnEmptyRequest(t *testing.T) {
	store, mock := newStore(t)

	dismissed, err := store.DismissedTraceIDs(context.Background(), "dataset-1", nil)
	require.NoError(t, err)
	assert.Empty(t, dismissed)
	require.NoError(t, mock.ExpectationsWereMet())
}
