package billing

import (
	"context"
	"fmt"
	"time"
)

// Signal is a provider-neutral billing-collection event, mapped from a
// Metronome or Stripe webhook. It drives the cached gating status without the
// server ever reading a balance — the provider owns money; we react to signals.
type Signal string

const (
	SignalPaymentFailed  Signal = "payment_failed"  // an auto-charge failed → enter/keep dunning
	SignalActionRequired Signal = "action_required" // 3DS/further action needed → keep dunning (URL surfaced by caller)
	SignalAlert          Signal = "alert"           // Metronome hard threshold/spend alert → suspend
	SignalUncollectible  Signal = "uncollectible"   // retries exhausted, invoice written off → force suspend
	SignalVoided         Signal = "voided"          // invoice voided → debt gone, clear all collection flags
	SignalRecovery       Signal = "recovery"        // payment succeeded → clear dunning + alert
	SignalCardUpdated    Signal = "card_updated"    // card network auto-updated an expired card → clear dunning
)

// ApplySignal writes the collection flags for a signal and recomputes the cached
// status. It performs no provider calls and does not touch workloads — the caller
// reconciles suspend/resume off the returned (status, changed). Idempotent: safe
// to re-run for a redelivered webhook (flag writes and Recompute converge).
func ApplySignal(ctx context.Context, store *StatusStore, accountID string, sig Signal, now time.Time) (Status, bool, error) {
	if store == nil {
		return StatusActive, false, nil
	}
	var err error
	switch sig {
	case SignalPaymentFailed, SignalActionRequired:
		err = store.SetDunningSince(ctx, accountID, now)
	case SignalAlert:
		err = store.SetAlert(ctx, accountID)
	case SignalUncollectible:
		err = store.SetForceSuspend(ctx, accountID)
	case SignalVoided:
		// Voiding removes the debt entirely — clear every collection flag,
		// including a write-off suspension keyed to that invoice.
		if err = store.ClearDunning(ctx, accountID); err == nil {
			if err = store.ClearAlert(ctx, accountID); err == nil {
				err = store.ClearForceSuspend(ctx, accountID)
			}
		}
	case SignalRecovery:
		// A successful payment resolves the payment-failure track (dunning) only.
		// It does NOT clear a spend/balance alert (paying an invoice doesn't lower
		// period spend) nor a terminal write-off (force-suspend) — those clear via
		// their own signals (void) or the gating plan, not an unrelated payment.
		err = store.ClearDunning(ctx, accountID)
	case SignalCardUpdated:
		// A refreshed card is a payment-path fix, not a balance fix — clear
		// dunning so the next Stripe/Metronome retry starts clean; leave any
		// balance alert intact.
		err = store.ClearDunning(ctx, accountID)
	default:
		return StatusActive, false, fmt.Errorf("unknown billing signal: %q", sig)
	}
	if err != nil {
		return StatusActive, false, err
	}
	return store.Recompute(ctx, accountID, now)
}
