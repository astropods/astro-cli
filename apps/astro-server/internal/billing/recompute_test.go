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
		"usage_limit_active", "not_provisioned",
	}).AddRow(string(status), reason, nil, false, force, exhausted, hasPM, nil, false, false)
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

func TestComputeStatus_UsageLimitRanksBelowProviderGatesAndAboveDunning(t *testing.T) {
	past := time.Now().Add(-30 * 24 * time.Hour)
	cases := []struct {
		name       string
		in         signals
		wantStatus Status
		wantReason string
	}{
		{name: "a usage cap alone suspends", in: signals{usageLimitActive: true},
			wantStatus: StatusSuspended, wantReason: ReasonUsageLimit},
		{name: "a write-off outranks it", in: signals{usageLimitActive: true, forceSuspended: true},
			wantStatus: StatusSuspended, wantReason: ReasonUncollectible},
		{name: "a provider alert outranks it", in: signals{usageLimitActive: true, alertActive: true},
			wantStatus: StatusSuspended, wantReason: ReasonBalanceAlert},
		{name: "spent credits outrank it", in: signals{usageLimitActive: true, creditsExhausted: true},
			wantStatus: StatusSuspended, wantReason: ReasonCreditsExhausted},
		{name: "it outranks dunning", in: signals{usageLimitActive: true, dunningSince: &past},
			wantStatus: StatusSuspended, wantReason: ReasonUsageLimit},
		{name: "cleared, the account is active", in: signals{}, wantStatus: StatusActive},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, reason := computeStatus(tc.in, 7*24*time.Hour, time.Now())
			if status != tc.wantStatus || reason != tc.wantReason {
				t.Errorf("computeStatus = (%s, %q), want (%s, %q)", status, reason, tc.wantStatus, tc.wantReason)
			}
		})
	}
}

func TestComputeStatus_NoContractOutranksEveryOtherReason(t *testing.T) {
	past := time.Now().Add(-30 * 24 * time.Hour)
	cases := []struct {
		name string
		in   signals
	}{
		{name: "alone", in: signals{notProvisioned: true}},
		{name: "with a write-off", in: signals{notProvisioned: true, forceSuspended: true}},
		{name: "with a provider alert", in: signals{notProvisioned: true, alertActive: true}},
		{name: "with spent credits", in: signals{notProvisioned: true, creditsExhausted: true}},
		{name: "with the account's own limit", in: signals{notProvisioned: true, usageLimitActive: true}},
		{name: "with dunning", in: signals{notProvisioned: true, dunningSince: &past}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, reason := computeStatus(tc.in, 7*24*time.Hour, time.Now())
			if status != StatusSuspended || reason != ReasonNotProvisioned {
				t.Errorf("computeStatus = (%s, %q), want (suspended, %q)", status, reason, ReasonNotProvisioned)
			}
		})
	}
}
