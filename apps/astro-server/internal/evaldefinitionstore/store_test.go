package evaldefinitionstore

import (
	"context"
	"testing"
	"time"

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

func TestCreateInsertsTheDefinition(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("INSERT INTO eval_definitions").
		WithArgs("agent/abc123", []byte(`{"evaluators":[]}`)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, store.Create(context.Background(), "agent/abc123", []byte(`{"evaluators":[]}`)))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateIsIdempotent(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectExec("INSERT INTO eval_definitions").
		WithArgs("agent/abc123", []byte(`{"evaluators":[]}`)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, store.Create(context.Background(), "agent/abc123", []byte(`{"evaluators":[]}`)))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetReturnsTheDefinition(t *testing.T) {
	store, mock := newStore(t)
	createdAt := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("FROM eval_definitions").
		WithArgs("agent/abc123").
		WillReturnRows(sqlmock.NewRows([]string{"evaluation_ref", "definition_json", "created_at"}).
			AddRow("agent/abc123", []byte(`{"evaluators":[]}`), createdAt))

	def, err := store.Get(context.Background(), "agent/abc123")
	require.NoError(t, err)
	require.NotNil(t, def)
	assert.Equal(t, "agent/abc123", def.EvaluationRef)
	assert.JSONEq(t, `{"evaluators":[]}`, string(def.DefinitionJSON))
	assert.Equal(t, createdAt, def.CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetReturnsNilWhenMissing(t *testing.T) {
	store, mock := newStore(t)
	mock.ExpectQuery("FROM eval_definitions").
		WithArgs("agent/missing").
		WillReturnRows(sqlmock.NewRows([]string{"evaluation_ref", "definition_json", "created_at"}))

	def, err := store.Get(context.Background(), "agent/missing")
	require.NoError(t, err)
	assert.Nil(t, def)
	require.NoError(t, mock.ExpectationsWereMet())
}
