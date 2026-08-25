package billing

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// Every signal that suspends, so a case cannot pass by picking a weak one.
func everySuspendingSignal() signals {
	dunning := time.Now().Add(-30 * 24 * time.Hour)
	return signals{
		dunningSince:     &dunning,
		alertActive:      true,
		forceSuspended:   true,
		creditsExhausted: true,
		usageLimitActive: true,
		notProvisioned:   true,
		hasPaymentMethod: false,
	}
}

// The exemption exists to survive the case where every other reason fires.
func TestComputeStatus_ExemptOutrunsEveryReason(t *testing.T) {
	sig := everySuspendingSignal()
	if st, _ := computeStatus(sig, 7*24*time.Hour, time.Now()); st != StatusSuspended {
		t.Fatalf("without the exemption status = %q, want suspended: the case proves nothing", st)
	}

	sig.exempt = true
	st, reason := computeStatus(sig, 7*24*time.Hour, time.Now())
	if st != StatusActive {
		t.Errorf("status = %q, want active", st)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty: an active account is not gated", reason)
	}
}

// notProvisioned is the reason a lapsed contract or an unreachable provider
// produces, which is when the exemption matters most.
func TestComputeStatus_ExemptSurvivesAnUnprovisionedAccount(t *testing.T) {
	sig := signals{notProvisioned: true, exempt: true}
	if st, _ := computeStatus(sig, 7*24*time.Hour, time.Now()); st != StatusActive {
		t.Errorf("status = %q, want active with no contract at all", st)
	}
}

// anyFlagSet drives collection behaviour elsewhere, so an exempt account has to
// read as having nothing to collect.
func TestSignals_ExemptAccountRaisesNoFlag(t *testing.T) {
	sig := everySuspendingSignal()
	if !sig.anyFlagSet() {
		t.Fatal("the fixture raises no flag, so the exempt case proves nothing")
	}
	sig.exempt = true
	if sig.anyFlagSet() {
		t.Error("an exempt account reports a flag to collect on")
	}
}

func TestIsExempt_OnlyTheNamedAccounts(t *testing.T) {
	s := (&StatusStore{}).WithExemptAccounts([]string{"acct-1", "  ", "acct-2"})
	for _, id := range []string{"acct-1", "acct-2"} {
		if !s.IsExempt(id) {
			t.Errorf("%s is not exempt", id)
		}
	}
	if s.IsExempt("acct-3") {
		t.Error("an unnamed account is exempt")
	}
	// An empty id would otherwise match an account whose id failed to resolve.
	if s.IsExempt("") {
		t.Error("the empty id is exempt")
	}
}

// The gate reads Get, not computeStatus. A row written before the account was
// exempt must not still block it.
func TestGet_ExemptAccountReadsActiveOverASuspendedRow(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	rows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"status", "reason"}).AddRow("suspended", ReasonCreditsExhausted)
	}

	// The same stored row, read by a store that does not exempt the account and
	// then by one that does. Without the first read the case would pass on a
	// store that never suspends anything.
	mock.ExpectQuery("SELECT status, reason FROM account_billing_status").
		WithArgs("acct-1").WillReturnRows(rows())
	st, _, err := NewStatusStore(db, 7).Get(context.Background(), "acct-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if st != StatusSuspended {
		t.Fatalf("status = %q, want suspended without the exemption", st)
	}

	mock.ExpectQuery("SELECT status, reason FROM account_billing_status").
		WithArgs("acct-1").WillReturnRows(rows())
	exempt := NewStatusStore(db, 7).WithExemptAccounts([]string{"acct-1"})
	st, reason, err := exempt.Get(context.Background(), "acct-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if st != StatusActive {
		t.Errorf("status = %q, want active", st)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
}

// Recompute writes the row the gate reads, so the exemption has to reach it or
// the next sweep would persist a suspension.
func TestRecompute_ExemptAccountIsNeverWrittenSuspended(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status, reason, dunning_since").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"status", "reason", "dunning_since", "alert_active", "force_suspended",
			"credits_exhausted", "has_payment_method", "pay_link", "usage_limit_active", "not_provisioned",
		}).AddRow("active", "", nil, true, true, true, false, "", true, true))

	// No UPDATE is queued. An exempt account computes active, which already
	// matches the stored row, so Recompute must write nothing at all.
	st, changed, err := NewStatusStore(db, 7).
		WithExemptAccounts([]string{"acct-1"}).
		Recompute(context.Background(), "acct-1", time.Now())
	if err != nil {
		t.Fatalf("Recompute: %v", err)
	}
	if st != StatusActive {
		t.Errorf("status = %q, want active", st)
	}
	if changed {
		t.Error("changed = true, but the account was already active")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected writes: %v", err)
	}
}
