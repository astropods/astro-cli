package admingrpc

import (
	"context"
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
)

// Approval must both mark the request approved and write the granted amount to
// account_limits (the table the quota checker enforces) in one transaction.
func TestApproveQuotaIncrease_AppliesAccountLimit(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT account_id, feature_key FROM quota_increase_requests`).
		WithArgs("req-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "feature_key"}).AddRow("acct-1", "blueprints"))
	mock.ExpectExec(`UPDATE quota_increase_requests`).
		WithArgs(float64(25), "ok", "req-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO account_limits`).
		WithArgs("acct-1", "blueprints", int64(25)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	srv := &Server{db: db, log: logger.New("error", "json")}
	resp, err := srv.ApproveQuotaIncrease(context.Background(), &adminv1.ApproveQuotaIncreaseRequest{
		RequestID:   "req-1",
		GrantAmount: 25,
		Note:        "ok",
	})
	if err != nil {
		t.Fatalf("ApproveQuotaIncrease: %v", err)
	}
	if resp.Status != "approved" {
		t.Errorf("status = %q, want approved", resp.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// A missing/already-resolved request rolls back and errors without touching
// account_limits.
func TestApproveQuotaIncrease_NotPending(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT account_id, feature_key FROM quota_increase_requests`).
		WithArgs("req-x").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	srv := &Server{db: db, log: logger.New("error", "json")}
	if _, err := srv.ApproveQuotaIncrease(context.Background(), &adminv1.ApproveQuotaIncreaseRequest{
		RequestID:   "req-x",
		GrantAmount: 5,
	}); err == nil {
		t.Fatal("expected error for non-pending request, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// A grant for a metered (non-count) feature is rejected — those have no
// account_limits row to raise — and nothing is written.
func TestApproveQuotaIncrease_NonManagedFeature(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT account_id, feature_key FROM quota_increase_requests`).
		WithArgs("req-2").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "feature_key"}).AddRow("acct-1", "compute"))
	mock.ExpectRollback()

	srv := &Server{db: db, log: logger.New("error", "json")}
	if _, err := srv.ApproveQuotaIncrease(context.Background(), &adminv1.ApproveQuotaIncreaseRequest{
		RequestID:   "req-2",
		GrantAmount: 5,
	}); err == nil {
		t.Fatal("expected error for non-managed feature, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestApproveQuotaIncrease_AppliesASpendLimitCeiling(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT account_id, feature_key FROM quota_increase_requests`).
		WithArgs("req-3").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "feature_key"}).AddRow("acct-1", "spend_limit"))
	mock.ExpectExec(`UPDATE quota_increase_requests`).
		WithArgs(float64(5000), "approved for the batch run", "req-3").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO account_limits`).
		WithArgs("acct-1", "spend_limit", int64(5000)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	srv := &Server{db: db, log: logger.New("error", "json")}
	resp, err := srv.ApproveQuotaIncrease(context.Background(), &adminv1.ApproveQuotaIncreaseRequest{
		RequestID:   "req-3",
		GrantAmount: 5000,
		Note:        "approved for the batch run",
	})
	if err != nil {
		t.Fatalf("ApproveQuotaIncrease: %v", err)
	}
	if resp.Status != "approved" {
		t.Errorf("status = %q, want approved", resp.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
