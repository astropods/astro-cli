package account

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSetClusterID_Assign(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := NewAccountStore(db)

	mock.ExpectExec("UPDATE accounts SET cluster_id").
		WithArgs(sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.SetClusterID("acct-1", "eu-west-1-managed"); err != nil {
		t.Fatalf("SetClusterID: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSetClusterID_Clear(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := NewAccountStore(db)

	mock.ExpectExec("UPDATE accounts SET cluster_id").
		WithArgs(sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.SetClusterID("acct-1", ""); err != nil {
		t.Fatalf("SetClusterID clear: %v", err)
	}
}

func TestSetClusterID_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store := NewAccountStore(db)

	mock.ExpectExec("UPDATE accounts SET cluster_id").
		WithArgs(sqlmock.AnyArg(), "missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = store.SetClusterID("missing", "eu-west-1-managed")
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}
