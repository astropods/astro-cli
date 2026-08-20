package langfuse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStoreGetDecryptedPlaintext(t *testing.T) {
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
			"langfuse_secret_key", "encrypted_data_key", "nonce", "created_at",
		}).AddRow("acct-1", "project-1", "pk", "sk", nil, nil, now))

	got, err := NewStore(db).GetDecrypted(context.Background(), nil, "acct-1")
	if err != nil {
		t.Fatalf("GetDecrypted: %v", err)
	}
	if got == nil || got.PublicKey != "pk" || got.SecretKey != "sk" {
		t.Fatalf("GetDecrypted = %+v", got)
	}
}

func TestStoreGetDecryptedMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery("FROM account_langfuse").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "langfuse_project_id", "langfuse_public_key",
			"langfuse_secret_key", "encrypted_data_key", "nonce", "created_at",
		}))

	got, err := NewStore(db).GetDecrypted(context.Background(), nil, "acct-1")
	if err != nil || got != nil {
		t.Fatalf("GetDecrypted = %+v, %v", got, err)
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

func TestStoreGetDecryptedErrors(t *testing.T) {
	t.Run("query", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		mock.ExpectQuery("FROM account_langfuse").
			WillReturnError(errors.New("query failed"))

		_, err = NewStore(db).GetDecrypted(context.Background(), nil, "acct-1")
		if err == nil || !strings.Contains(err.Error(), "query failed") {
			t.Fatalf("GetDecrypted error = %v", err)
		}
	})

	t.Run("encrypted without KMS", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		mock.ExpectQuery("FROM account_langfuse").
			WillReturnRows(sqlmock.NewRows([]string{
				"account_id", "langfuse_project_id", "langfuse_public_key",
				"langfuse_secret_key", "encrypted_data_key", "nonce", "created_at",
			}).AddRow("acct-1", "project-1", "pk", "ciphertext", []byte("key"), []byte("nonce"), time.Now()))

		_, err = NewStore(db).GetDecrypted(context.Background(), nil, "acct-1")
		if err == nil || !strings.Contains(err.Error(), "KMS client required") {
			t.Fatalf("GetDecrypted error = %v", err)
		}
	})
}
