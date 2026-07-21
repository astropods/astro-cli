package memberemails

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db), mock
}

func TestUpsertWorkOSLowercasesAndPrunesStale(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectBegin()
	// Email is lowercased/trimmed; stale workos rows for the user are pruned.
	mock.ExpectExec("DELETE FROM account_member_emails").
		WithArgs("user_1", "dev@x.com").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO account_member_emails").
		WithArgs("user_1", "dev@x.com", true).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := store.UpsertWorkOS(context.Background(), "user_1", "  Dev@X.com ", true); err != nil {
		t.Fatalf("UpsertWorkOS: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUpsertWorkOSNoopOnEmptyEmail(t *testing.T) {
	store, mock := newMockStore(t)
	// No DB interaction expected for empty email.
	if err := store.UpsertWorkOS(context.Background(), "user_1", "   ", false); err != nil {
		t.Fatalf("UpsertWorkOS: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestEmailsForAccount(t *testing.T) {
	store, mock := newMockStore(t)
	rows := sqlmock.NewRows([]string{"email", "user_id"}).
		AddRow("alice@x.com", "u1").
		AddRow("bob@x.com", "u2")
	mock.ExpectQuery("SELECT me.email").WithArgs("acct-1").WillReturnRows(rows)

	got, err := store.EmailsForAccount(context.Background(), "acct-1")
	if err != nil {
		t.Fatalf("EmailsForAccount: %v", err)
	}
	if got["alice@x.com"] != "u1" || got["bob@x.com"] != "u2" || len(got) != 2 {
		t.Fatalf("unexpected map: %+v", got)
	}
}

func TestUserIDsMissingEmail(t *testing.T) {
	store, mock := newMockStore(t)
	rows := sqlmock.NewRows([]string{"user_id"}).AddRow("u1").AddRow("u2")
	mock.ExpectQuery("SELECT DISTINCT am.user_id").WithArgs(500, sqlmock.AnyArg()).WillReturnRows(rows)

	got, err := store.UserIDsMissingEmail(context.Background(), 500, time.Now())
	if err != nil {
		t.Fatalf("UserIDsMissingEmail: %v", err)
	}
	if len(got) != 2 || got[0] != "u1" || got[1] != "u2" {
		t.Fatalf("unexpected ids: %+v", got)
	}
}

func TestRecordReconcileAttempt(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectExec("INSERT INTO member_email_reconcile_attempts").
		WithArgs("u1").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.RecordReconcileAttempt(context.Background(), "u1"); err != nil {
		t.Fatalf("RecordReconcileAttempt: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
