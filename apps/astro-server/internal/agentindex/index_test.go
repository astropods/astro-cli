package agentindex

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreate_NewAgent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs("acct-1", "my-agent", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM agent_versions").
		WithArgs("acct-1", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := idx.Create("acct-1", "my-agent"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreate_ActiveAgentReturnsErrAlreadyExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	// ON CONFLICT DO UPDATE WHERE archived_at IS NOT NULL — no rows affected when agent is active.
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs("acct-1", "my-agent", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err = idx.Create("acct-1", "my-agent")
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCreate_ArchivedAgentUnarchivesAndClearsVersions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	idx := NewIndexWithDB(db)

	// ON CONFLICT DO UPDATE unarchives — 1 row affected.
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO agents").
		WithArgs("acct-1", "my-agent", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM agent_versions").
		WithArgs("acct-1", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 2)) // 2 stale versions cleared
	mock.ExpectCommit()

	if err := idx.Create("acct-1", "my-agent"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
