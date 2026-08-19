package billing

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func recordRows(status Status, reason string, force, exhausted, hasPM bool) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"status", "reason", "dunning_since", "alert_active",
		"force_suspended", "credits_exhausted", "has_payment_method", "pay_link",
	}).AddRow(string(status), reason, nil, false, force, exhausted, hasPM, nil)
}

// A write-off on an account already suspended for spent credits keeps the
// status but must change the reason. The banner renders reason, and "add a
// card" cannot lift force_suspended, so a stale reason tells the user to do
// something that provably won't work.
func TestRecompute_PersistsReasonChangeWithinSameStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").
		WithArgs("acct_1").
		WillReturnRows(recordRows(StatusSuspended, ReasonCreditsExhausted, true, true, false))
	mock.ExpectExec("account_billing_status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	st, changed, err := NewStatusStore(db, 7).Recompute(context.Background(), "acct_1", time.Now())
	if err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	if st != StatusSuspended {
		t.Fatalf("status = %q, want suspended", st)
	}
	// The status did not transition, so callers that notify on a transition
	// must not fire again.
	if changed {
		t.Error("reason-only change reported as a status transition")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("reason change was not persisted: %v", err)
	}
}

// Recompute must read under a row lock and write in the same transaction, or
// two concurrent signals race and the loser's stale status wins.
func TestRecompute_LocksTheRowAndWritesInOneTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").
		WithArgs("acct_1").
		WillReturnRows(recordRows(StatusActive, "", false, true, false))
	mock.ExpectExec("account_billing_status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	st, changed, err := NewStatusStore(db, 7).Recompute(context.Background(), "acct_1", time.Now())
	if err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	if st != StatusSuspended || !changed {
		t.Fatalf("got (%q, changed=%v), want (suspended, true)", st, changed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("recompute was not transactional: %v", err)
	}
}

// Nothing to do means no write at all, so a redelivered webhook doesn't churn
// updated_at or take a write lock.
func TestRecompute_NoWriteWhenNothingChanged(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").
		WithArgs("acct_1").
		WillReturnRows(recordRows(StatusSuspended, ReasonCreditsExhausted, false, true, false))
	mock.ExpectRollback()

	if _, changed, err := NewStatusStore(db, 7).Recompute(context.Background(), "acct_1", time.Now()); err != nil || changed {
		t.Fatalf("Recompute = (changed=%v, err=%v), want (false, nil)", changed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected statements: %v", err)
	}
}
