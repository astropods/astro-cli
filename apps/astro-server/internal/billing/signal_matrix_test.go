package billing

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// The gating flags. A gate that can be raised but not lowered is a customer an
// operator has to rescue by hand, which is how both the credits-exhaustion and
// spend-alert latches shipped.
const (
	flagDunning = "dunning_since"
	flagAlert   = "alert_active"
	flagForce   = "force_suspended"
	flagCredits = "credits_exhausted"
	flagHasCard = "has_payment_method"
)

// write is one flag mutation a signal must perform. sql is matched against the
// statement the store actually issues, so the spec below cannot drift from the
// code without a test failure.
type write struct {
	flag   string
	raises bool // true = gate goes up (or the fact is written), false = gate comes down
	sql    string
}

// spec is what one signal does: the flag writes it is allowed to perform, and
// for a signal that raises a gate, the signal that undoes it.
//
// inverse must undo the *same condition*, not merely touch the same column.
// Requiring only "something clears this flag" is too weak to catch the bug it
// looks like it catches: SignalVoided has always cleared alert_active, so a
// spend alert with no resolved event of its own would still have passed.
type spec struct {
	writes  []write
	inverse Signal
}

// signalWrites is the specification for ApplySignal: what each signal is
// allowed to touch, in order. Anything absent here must not be written, which is
// the half that keeps a payment from silently clearing a write-off.
var signalWrites = map[Signal]spec{
	SignalPaymentFailed:  {writes: []write{{flagDunning, true, "COALESCE"}}, inverse: SignalRecovery},
	SignalActionRequired: {writes: []write{{flagDunning, true, "COALESCE"}}, inverse: SignalRecovery},
	SignalAlert:          {writes: []write{{flagAlert, true, "alert_active = true"}}, inverse: SignalAlertResolved},
	SignalAlertResolved:  {writes: []write{{flagAlert, false, "alert_active = false"}}},
	SignalUncollectible:  {writes: []write{{flagForce, true, "force_suspended = true"}}, inverse: SignalVoided},
	SignalVoided: {writes: []write{
		{flagDunning, false, "dunning_since = NULL"},
		{flagAlert, false, "alert_active = false"},
		{flagForce, false, "force_suspended = false"},
	}},
	SignalRecovery:         {writes: []write{{flagDunning, false, "dunning_since = NULL"}}},
	SignalCardUpdated:      {writes: []write{{flagDunning, false, "dunning_since = NULL"}}},
	SignalCreditsExhausted: {writes: []write{{flagCredits, true, "credits_exhausted = true"}}, inverse: SignalCreditsGranted},
	SignalCreditsGranted:   {writes: []write{{flagCredits, false, "credits_exhausted = false"}}},
	SignalCardAdded:        {writes: []write{{flagHasCard, true, "has_payment_method = EXCLUDED"}}, inverse: SignalCardRemoved},
	SignalCardRemoved:      {writes: []write{{flagHasCard, false, "has_payment_method = EXCLUDED"}}},
}

// A clear that only an operator can trigger is the other half of the latch bug.
// That check needs the provider event maps, which live in riverqueue, so it is
// asserted there against this same set.

// Every declared signal must be specified and handled. Without this, a new
// signal reaches ApplySignal's default and fails the webhook at runtime instead
// of at build time.
func TestApplySignal_EverySignalIsSpecifiedAndHandled(t *testing.T) {
	for _, sig := range AllSignals {
		if _, ok := signalWrites[sig]; !ok {
			t.Errorf("signal %q has no entry in signalWrites: decide what it writes", sig)
		}
	}
	if len(signalWrites) != len(AllSignals) {
		t.Errorf("signalWrites has %d entries, AllSignals has %d", len(signalWrites), len(AllSignals))
	}
}

// The invariant the spend-alert latch violated: a signal that raises a gate must
// name the signal that undoes it, and that signal must actually lower the same
// flag. Pairing by cause rather than by column is the point — an unrelated event
// that happens to touch the column is not an exit from the condition.
func TestApplySignal_EveryGateHasAnInverse(t *testing.T) {
	for _, sig := range AllSignals {
		for _, w := range signalWrites[sig].writes {
			if !w.raises {
				continue
			}
			inv := signalWrites[sig].inverse
			if inv == "" {
				t.Errorf("%s raises %s with no inverse: only an operator could un-gate the account", sig, w.flag)
				continue
			}
			if _, ok := signalWrites[inv]; !ok {
				t.Errorf("%s names inverse %q, which is not a declared signal", sig, inv)
				continue
			}
			var lowers bool
			for _, iw := range signalWrites[inv].writes {
				if iw.flag == w.flag && !iw.raises {
					lowers = true
				}
			}
			if !lowers {
				t.Errorf("%s raises %s but its inverse %s never lowers it", sig, w.flag, inv)
			}
		}
	}
}

// Holds ApplySignal to the spec statement by statement. A signal wired to the
// wrong clear still recomputes to a plausible status, so asserting the status
// alone would not catch it.
func TestApplySignal_WritesOnlyItsOwnFlags(t *testing.T) {
	for _, sig := range AllSignals {
		t.Run(string(sig), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close() //nolint:errcheck

			for _, w := range signalWrites[sig].writes {
				mock.ExpectExec(w.sql).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectBegin()
			// Stale-suspended with every flag clear, so the recompute always
			// transitions and therefore always persists. Recompute's own rules are
			// covered in recompute_test.go; this test is about the flag writes.
			mock.ExpectQuery("FOR UPDATE").
				WithArgs("acct_1").
				WillReturnRows(recordRows(StatusSuspended, ReasonUncollectible, false, false, false))
			mock.ExpectExec("account_billing_status").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()

			if _, _, err := ApplySignal(context.Background(), NewStatusStore(db, 7), "acct_1", sig, time.Now()); err != nil {
				t.Fatalf("ApplySignal(%s): %v", sig, err)
			}
			// Ordered expectations, so this also fails on an extra or reordered write.
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("%s wrote the wrong flags: %v", sig, err)
			}
		})
	}
}
