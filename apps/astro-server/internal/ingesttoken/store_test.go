package ingesttoken

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() }) //nolint:errcheck
	return NewStore(db), mock
}

func TestUpdateExclusionsReplacesList(t *testing.T) {
	store, mock := newMockStore(t)
	emails := []string{"dev@x.com", "ops@x.com"}
	mock.ExpectExec("UPDATE otel_ingest_tokens\\s+SET excluded_emails").
		WithArgs(pq.Array(emails), "key-1", "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.UpdateExclusions("acct-1", "key-1", emails); err != nil {
		t.Fatalf("UpdateExclusions: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUpdateExclusionsNoActiveKeyReturnsErrNoRows(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectExec("UPDATE otel_ingest_tokens").
		WithArgs(pq.Array([]string{}), "missing", "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := store.UpdateExclusions("acct-1", "missing", []string{})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want sql.ErrNoRows, got %v", err)
	}
}

func TestRenameUpdatesName(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectExec("UPDATE otel_ingest_tokens\\s+SET name").
		WithArgs("Contractor laptops", "key-1", "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.Rename("acct-1", "key-1", "Contractor laptops"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRenameNoActiveKeyReturnsErrNoRows(t *testing.T) {
	store, mock := newMockStore(t)
	mock.ExpectExec("UPDATE otel_ingest_tokens").
		WithArgs("whatever", "missing", "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := store.Rename("acct-1", "missing", "whatever")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want sql.ErrNoRows, got %v", err)
	}
}

func TestCreateStoresExclusions(t *testing.T) {
	store, mock := newMockStore(t)
	emails := []string{"dev@x.com"}
	cols := []string{"id", "account_id", "name", "token_prefix", "created_at", "created_by", "last_used_at", "revoked_at", "excluded_emails"}
	mock.ExpectQuery("INSERT INTO otel_ingest_tokens").
		WithArgs("acct-1", "laptops", []byte("hash"), "astotel_abcd", sqlmock.AnyArg(), pq.Array(emails)).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(
			"key-1", "acct-1", "laptops", "astotel_abcd", time.Unix(0, 0), nil, nil, nil, "{dev@x.com}",
		))

	tok, err := store.Create("acct-1", "laptops", []byte("hash"), "astotel_abcd", "user_1", emails)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(tok.ExcludedEmails) != 1 || tok.ExcludedEmails[0] != "dev@x.com" {
		t.Fatalf("ExcludedEmails = %v, want [dev@x.com]", tok.ExcludedEmails)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
