package billing

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// The spend-alert latch has exactly one clearing path, so a signal wired to the
// wrong flag leaves an account suspended forever while still looking handled.
// Assert the flag write, not just the recomputed status.
func TestApplySignal_AlertResolvedClearsTheAlertFlag(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectExec("alert_active = false").
		WithArgs("acct_1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").
		WithArgs("acct_1").
		WillReturnRows(recordRows(StatusSuspended, ReasonBalanceAlert, false, false, false))
	mock.ExpectExec("account_billing_status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	st, changed, err := ApplySignal(context.Background(), NewStatusStore(db, 7), "acct_1", SignalAlertResolved, time.Now())
	if err != nil {
		t.Fatalf("ApplySignal: %v", err)
	}
	if st != StatusActive || !changed {
		t.Fatalf("got (%q, changed=%v), want (active, true)", st, changed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("alert flag was not cleared: %v", err)
	}
}

// Resolving a spend alert must not also clear dunning or a write-off. Those
// track debt, which a drop in period spend does not pay.
func TestApplySignal_AlertResolvedLeavesTheOtherFlags(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectExec("alert_active = false").
		WithArgs("acct_1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").
		WithArgs("acct_1").
		WillReturnRows(recordRows(StatusSuspended, ReasonBalanceAlert, true, false, false))
	mock.ExpectExec("account_billing_status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	st, _, err := ApplySignal(context.Background(), NewStatusStore(db, 7), "acct_1", SignalAlertResolved, time.Now())
	if err != nil {
		t.Fatalf("ApplySignal: %v", err)
	}
	if st != StatusSuspended {
		t.Fatalf("status = %q, want suspended: force_suspended outranks a cleared alert", st)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected statements: %v", err)
	}
}
