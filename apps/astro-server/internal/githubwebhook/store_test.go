package githubwebhook

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func cols() []string {
	return []string{"repo_base", "webhook_id", "webhook_secret", "created_at"}
}

func TestStore_Get_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	s := New(db)

	now := time.Now()
	mock.ExpectQuery(`SELECT repo_base, webhook_id, webhook_secret, created_at FROM github_webhooks`).
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows(cols()).AddRow("owner/repo", int64(42), "supersecret", now))

	w, err := s.Get(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if w.RepoBase != "owner/repo" {
		t.Errorf("RepoBase = %q, want %q", w.RepoBase, "owner/repo")
	}
	if w.WebhookID != 42 {
		t.Errorf("WebhookID = %d, want 42", w.WebhookID)
	}
	if w.WebhookSecret != "supersecret" {
		t.Errorf("WebhookSecret = %q, want %q", w.WebhookSecret, "supersecret")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	s := New(db)

	mock.ExpectQuery(`SELECT repo_base, webhook_id, webhook_secret, created_at FROM github_webhooks`).
		WithArgs("owner/unknown").
		WillReturnRows(sqlmock.NewRows(cols()))

	_, err = s.Get(context.Background(), "owner/unknown")
	if err != sql.ErrNoRows {
		t.Errorf("Get returned %v, want sql.ErrNoRows", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestStore_Insert_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	s := New(db)

	mock.ExpectExec(`INSERT INTO github_webhooks`).
		WithArgs("owner/repo", int64(7), "mysecret").
		WillReturnResult(sqlmock.NewResult(0, 1))

	inserted, err := s.Insert(context.Background(), "owner/repo", 7, "mysecret")
	if err != nil {
		t.Errorf("Insert: %v", err)
	}
	if !inserted {
		t.Error("expected inserted=true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestStore_Insert_Conflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	s := New(db)

	// ON CONFLICT DO NOTHING returns 0 rows affected — race loser.
	mock.ExpectExec(`INSERT INTO github_webhooks`).
		WithArgs("owner/repo", int64(7), "mysecret").
		WillReturnResult(sqlmock.NewResult(0, 0))

	inserted, err := s.Insert(context.Background(), "owner/repo", 7, "mysecret")
	if err != nil {
		t.Errorf("Insert: %v", err)
	}
	if inserted {
		t.Error("expected inserted=false on conflict")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestStore_DeleteIfNoConnections_Deleted(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	s := New(db)

	mock.ExpectQuery(`DELETE FROM github_webhooks`).
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"webhook_id"}).AddRow(int64(99)))

	id, deleted, err := s.DeleteIfNoConnections(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("DeleteIfNoConnections: %v", err)
	}
	if !deleted {
		t.Error("expected deleted=true")
	}
	if id != 99 {
		t.Errorf("webhookID = %d, want 99", id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestStore_DeleteIfNoConnections_ConnectionsStillExist(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	s := New(db)

	// No row returned means connections still exist (WHERE NOT EXISTS prevented deletion).
	mock.ExpectQuery(`DELETE FROM github_webhooks`).
		WithArgs("owner/repo").
		WillReturnRows(sqlmock.NewRows([]string{"webhook_id"}))

	id, deleted, err := s.DeleteIfNoConnections(context.Background(), "owner/repo")
	if err != nil {
		t.Fatalf("DeleteIfNoConnections: %v", err)
	}
	if deleted {
		t.Error("expected deleted=false when connections still exist")
	}
	if id != 0 {
		t.Errorf("webhookID = %d, want 0", id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
