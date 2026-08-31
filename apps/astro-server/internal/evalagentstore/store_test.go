package evalagentstore

import (
	"context"
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

func TestGetReturnsTheRow(t *testing.T) {
	store, mock := newStore(t)
	updatedAt := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("FROM agent_evaluations").
		WithArgs("account-1", "agent-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "agent_name", "evaluation_ref", "updated_at"}).
			AddRow("account-1", "agent-1", "agent/abc123", updatedAt))

	ae, err := store.Get(context.Background(), "account-1", "agent-1")
	require.NoError(t, err)
	require.NotNil(t, ae)
	assert.Equal(t, "agent/abc123", ae.EvaluationRef)
	assert.Equal(t, updatedAt, ae.UpdatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetReturnsNilWhenNoRow(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("FROM agent_evaluations").
		WithArgs("account-1", "agent-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "agent_name", "evaluation_ref", "updated_at"}))

	ae, err := store.Get(context.Background(), "account-1", "agent-1")
	require.NoError(t, err)
	assert.Nil(t, ae)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSetUpsertsTheRow(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("INSERT INTO agent_evaluations").
		WithArgs("account-1", "agent-1", "agent/abc123").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, store.Set(context.Background(), "account-1", "agent-1", "agent/abc123"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSetReturnsErrDefinitionNotFoundOnMissingDefinition(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("INSERT INTO agent_evaluations").
		WithArgs("account-1", "agent-1", "agent/abc123").
		WillReturnError(&pq.Error{Code: "23503", Constraint: "agent_evaluations_definition_fkey"})

	err := store.Set(context.Background(), "account-1", "agent-1", "agent/abc123")
	assert.ErrorIs(t, err, ErrDefinitionNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClearDeletesTheRow(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("DELETE FROM agent_evaluations").
		WithArgs("account-1", "agent-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, store.Clear(context.Background(), "account-1", "agent-1"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClearSucceedsWhenNoRow(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("DELETE FROM agent_evaluations").
		WithArgs("account-1", "agent-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, store.Clear(context.Background(), "account-1", "agent-1"))
	require.NoError(t, mock.ExpectationsWereMet())
}
