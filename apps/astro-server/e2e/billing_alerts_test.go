//go:build integration

// Integration coverage for the billing alert gating machine against a real
// Postgres. CI job: `Integration tests (astro-server + Postgres)` in test.yml.
//
// Every other test of this machine uses sqlmock, which asserts the SQL we send
// rather than what a database does with it. That distinction is load-bearing
// here for three reasons:
//
//  1. The latch columns have defaults and an upsert. A signal that writes the
//     wrong column, or an ON CONFLICT clause that clobbers a sibling flag,
//     passes a string assertion and loses a suspension.
//  2. computeStatus ranks five reasons. Only a round trip proves the rank the
//     code intends is the rank the stored row produces.
//  3. The dunning clock is a timestamp compared against a grace window.
//     sqlmock can assert a COALESCE appears in the statement; it cannot prove
//     the second delivery keeps the first delivery's timestamp.
//
// The cases below walk the paths an account actually takes: an alert stops it,
// the resolved edge starts it, and a redelivery does not move the deadline.
package e2e

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	_ "github.com/lib/pq"
)

// billingAccount creates an account with no billing row, which is how every
// account starts: absence means active.
func billingAccount(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(
		`WITH acct AS (INSERT INTO accounts (name, type, owner_user_id) VALUES ($1, 'organization', 'test-owner') RETURNING id), member AS (INSERT INTO account_members (account_id, user_id) SELECT id, 'test-owner' FROM acct ON CONFLICT DO NOTHING) SELECT id FROM acct`, name,
	).Scan(&id); err != nil {
		t.Fatalf("seed account %s: %v", name, err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM account_billing_status WHERE account_id = $1`, id)
		_, _ = db.Exec(`DELETE FROM accounts WHERE id = $1`, id)
	})
	return id
}

func readStatus(t *testing.T, store *billing.StatusStore, id string) (billing.Status, string) {
	t.Helper()
	st, reason, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	return st, reason
}

// A spend alert has to survive the round trip: set the latch, recompute, and
// read back a suspension whose reason names the spend threshold rather than
// whichever branch happens to be first.
func TestBillingAlerts_SpendAlertStopsAndResolvedStarts(t *testing.T) {
	db := testDB(t)
	store := billing.NewStatusStore(db, 7)
	ctx := context.Background()
	acct := billingAccount(t, db, "ba-spend-e2e")

	if st, _ := readStatus(t, store, acct); st != billing.StatusActive {
		t.Fatalf("a new account reads %q, want active: absence of a row means active", st)
	}

	if _, _, err := billing.ApplySignal(ctx, store, acct, billing.SignalAlert, time.Now()); err != nil {
		t.Fatalf("apply alert: %v", err)
	}
	st, reason := readStatus(t, store, acct)
	if st != billing.StatusSuspended || reason != billing.ReasonBalanceAlert {
		t.Fatalf("after the alert: %q/%q, want suspended/balance_alert", st, reason)
	}

	// The resolved edge is the only automatic way out. Without it the latch is
	// one-way and an account that stopped spending stays stopped.
	if _, _, err := billing.ApplySignal(ctx, store, acct, billing.SignalAlertResolved, time.Now()); err != nil {
		t.Fatalf("apply resolved: %v", err)
	}
	if st, reason := readStatus(t, store, acct); st != billing.StatusActive || reason != "" {
		t.Fatalf("after resolved: %q/%q, want active with no reason", st, reason)
	}
}

// Credit exhaustion gates only while no card is on file, so the two flags have
// to survive together in one row. An upsert that clobbered the sibling flag
// would either strand a paying account or let a card-less one run.
func TestBillingAlerts_CreditExhaustionGatesOnlyWithoutACard(t *testing.T) {
	db := testDB(t)
	store := billing.NewStatusStore(db, 7)
	ctx := context.Background()

	t.Run("no card on file suspends", func(t *testing.T) {
		acct := billingAccount(t, db, "ba-credits-nocard-e2e")
		if _, _, err := billing.ApplySignal(ctx, store, acct, billing.SignalCreditsExhausted, time.Now()); err != nil {
			t.Fatalf("apply exhaustion: %v", err)
		}
		if st, reason := readStatus(t, store, acct); st != billing.StatusSuspended || reason != billing.ReasonCreditsExhausted {
			t.Fatalf("got %q/%q, want suspended/credits_exhausted", st, reason)
		}
	})

	t.Run("a card keeps it running", func(t *testing.T) {
		acct := billingAccount(t, db, "ba-credits-card-e2e")
		if _, _, err := billing.ApplySignal(ctx, store, acct, billing.SignalCardAdded, time.Now()); err != nil {
			t.Fatalf("apply card: %v", err)
		}
		if _, _, err := billing.ApplySignal(ctx, store, acct, billing.SignalCreditsExhausted, time.Now()); err != nil {
			t.Fatalf("apply exhaustion: %v", err)
		}
		// Pay-as-you-go: the credit latch is set, but the card covers the overage.
		if st, _ := readStatus(t, store, acct); st != billing.StatusActive {
			t.Fatalf("got %q, want active: a card makes an exhausted account pay-as-you-go", st)
		}
	})
}

// A write-off outranks a spend alert, which outranks exhaustion. Only a stored
// row proves the rank, because every flag is set on the same row and the reason
// is whichever branch computeStatus reaches first.
func TestBillingAlerts_ReasonRankSurvivesTheRoundTrip(t *testing.T) {
	db := testDB(t)
	store := billing.NewStatusStore(db, 7)
	ctx := context.Background()
	acct := billingAccount(t, db, "ba-rank-e2e")

	for _, sig := range []billing.Signal{
		billing.SignalCreditsExhausted,
		billing.SignalAlert,
		billing.SignalUncollectible,
	} {
		if _, _, err := billing.ApplySignal(ctx, store, acct, sig, time.Now()); err != nil {
			t.Fatalf("apply %s: %v", sig, err)
		}
	}
	if _, reason := readStatus(t, store, acct); reason != billing.ReasonUncollectible {
		t.Fatalf("reason = %q, want uncollectible: a write-off outranks the others", reason)
	}

	// Voiding the invoice drops to the next-ranked reason still latched, rather
	// than clearing everything. Telling a spend-limited account it is fine
	// because an invoice was voided would resume it over its threshold.
	if _, _, err := billing.ApplySignal(ctx, store, acct, billing.SignalVoided, time.Now()); err != nil {
		t.Fatalf("apply voided: %v", err)
	}
	if st, reason := readStatus(t, store, acct); st != billing.StatusSuspended || reason != billing.ReasonCreditsExhausted {
		t.Fatalf("after the void: %q/%q, want the next latched reason, not active", st, reason)
	}
}

// The grace window is measured from the first failed payment. A provider
// redelivery must not restart that clock, or the suspension deadline walks
// forward on every retry and the account runs unpaid indefinitely.
func TestBillingAlerts_RedeliveryKeepsTheOriginalDunningClock(t *testing.T) {
	db := testDB(t)
	store := billing.NewStatusStore(db, 7)
	ctx := context.Background()
	acct := billingAccount(t, db, "ba-dunning-e2e")

	// ApplySignal stamps dunning_since with the time it is given and recomputes
	// against that same instant, so the first delivery is always within grace
	// however far back it is dated. The window only elapses on a later recompute.
	first := time.Now().Add(-8 * 24 * time.Hour)
	if _, _, err := billing.ApplySignal(ctx, store, acct, billing.SignalPaymentFailed, first); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if st, reason := readStatus(t, store, acct); st != billing.StatusPastDue || reason != billing.ReasonDunning {
		t.Fatalf("first delivery: %q/%q, want past_due within grace", st, reason)
	}

	// The same event again, stamped now, which is what a provider retry sends.
	// This call is the whole proof. COALESCE keeps the original stamp, so the
	// window it evaluates is eight days wide and the account suspends. A clock
	// that restarted would measure from now, span nothing, and stay past_due
	// while the account keeps running unpaid.
	st, _, err := billing.ApplySignal(ctx, store, acct, billing.SignalPaymentFailed, time.Now())
	if err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if st != billing.StatusSuspended {
		t.Fatalf("after redelivery: %q, want suspended; the clock restarted", st)
	}
	if _, reason := readStatus(t, store, acct); reason != billing.ReasonPaymentFailed {
		t.Fatalf("reason = %q, want payment_failed", reason)
	}

	var since time.Time
	if err := db.QueryRow(
		`SELECT dunning_since FROM account_billing_status WHERE account_id = $1`, acct,
	).Scan(&since); err != nil {
		t.Fatalf("read dunning_since: %v", err)
	}
	if since.After(first.Add(time.Minute)) {
		t.Errorf("dunning_since = %s, want the first delivery at %s", since, first)
	}
}

// Paying an invoice clears dunning and nothing else. Period spend is unchanged
// by a payment, so a spend-limited account must stay stopped.
func TestBillingAlerts_RecoveryDoesNotLiftASpendAlert(t *testing.T) {
	db := testDB(t)
	store := billing.NewStatusStore(db, 7)
	ctx := context.Background()
	acct := billingAccount(t, db, "ba-recovery-e2e")

	for _, sig := range []billing.Signal{billing.SignalPaymentFailed, billing.SignalAlert} {
		if _, _, err := billing.ApplySignal(ctx, store, acct, sig, time.Now().Add(-8*24*time.Hour)); err != nil {
			t.Fatalf("apply %s: %v", sig, err)
		}
	}
	if _, _, err := billing.ApplySignal(ctx, store, acct, billing.SignalRecovery, time.Now()); err != nil {
		t.Fatalf("apply recovery: %v", err)
	}
	if st, reason := readStatus(t, store, acct); st != billing.StatusSuspended || reason != billing.ReasonBalanceAlert {
		t.Fatalf("after recovery: %q/%q, want the spend alert still holding", st, reason)
	}
}
