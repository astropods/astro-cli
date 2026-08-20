package langfuse

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStoreGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("FROM account_langfuse").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "langfuse_project_id", "langfuse_public_key",
			"langfuse_secret_key", "created_at",
		}).AddRow("acct-1", "project-1", "pk", "sk-lf-key", now))

	got, err := NewStore(db).Get("acct-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.PublicKey != "pk" || got.SecretKey != "sk-lf-key" {
		t.Fatalf("Get = %+v", got)
	}
}

// An account without a Langfuse project is a normal state, not an error: the
// callers read it as "nothing to fetch" rather than failing their job.
func TestStoreGetMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("FROM account_langfuse").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "langfuse_project_id", "langfuse_public_key",
			"langfuse_secret_key", "created_at",
		}))

	got, err := NewStore(db).Get("acct-1")
	if err != nil || got != nil {
		t.Fatalf("Get = %+v, %v", got, err)
	}
}

// The credential row outlives a soft delete, so listing account_langfuse alone
// hands the discovery workers accounts that every downstream lookup then
// refuses to load. Each one becomes a fan-out job that fails and retries daily.
func TestListAccountIDsExcludesDeletedAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(`JOIN accounts a ON a\.id = al\.account_id AND a\.deleted_at IS NULL`).
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct-live"))

	got, err := NewStore(db).ListAccountIDs()
	if err != nil {
		t.Fatalf("ListAccountIDs: %v", err)
	}
	if len(got) != 1 || got[0] != "acct-live" {
		t.Fatalf("ListAccountIDs = %v, want [acct-live]", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("query did not filter deleted accounts: %v", err)
	}
}

func TestStoreGetQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("FROM account_langfuse").
		WillReturnError(errors.New("query failed"))

	if _, err = NewStore(db).Get("acct-1"); err == nil || !strings.Contains(err.Error(), "query failed") {
		t.Fatalf("Get error = %v", err)
	}
}

// The secret key is the password half of Langfuse basic auth, so it has to
// arrive at the client exactly as it was minted.
func TestStoreSaveWritesTheKeyAsMinted(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec("INSERT INTO account_langfuse").
		WithArgs("acct-1", "project-1", "pk-lf-1", "sk-lf-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = NewStore(db).Save(&AccountLangfuse{
		AccountID:         "acct-1",
		LangfuseProjectID: "project-1",
		PublicKey:         "pk-lf-1",
		SecretKey:         "sk-lf-1",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
