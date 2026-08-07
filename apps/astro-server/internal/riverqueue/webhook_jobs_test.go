package riverqueue

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

func TestStripeSignalMapping(t *testing.T) {
	cases := []struct {
		event   string
		want    billing.Signal
		handled bool
	}{
		{"invoice.payment_failed", billing.SignalPaymentFailed, true},
		{"invoice.payment_action_required", billing.SignalActionRequired, true},
		{"invoice.marked_uncollectible", billing.SignalUncollectible, true},
		{"invoice.voided", billing.SignalVoided, true},
		{"invoice.paid", billing.SignalRecovery, true},
		{"payment_method.automatically_updated", billing.SignalCardUpdated, true},
		// Backstop for the inline, best-effort has_payment_method write.
		// detached is provisional — Work re-reads Stripe to settle it.
		{"payment_method.attached", billing.SignalCardAdded, true},
		{"payment_method.detached", billing.SignalCardRemoved, true},
		// Lifecycle events Metronome owns — no signal.
		{"invoice.payment_succeeded", "", false},
		{"invoice.created", "", false},
		{"charge.succeeded", "", false},
	}
	for _, tc := range cases {
		got, ok := stripeSignal(tc.event)
		if ok != tc.handled || got != tc.want {
			t.Errorf("stripeSignal(%q) = (%q, %v), want (%q, %v)", tc.event, got, ok, tc.want, tc.handled)
		}
	}
}

func TestMetronomeSignalMapping(t *testing.T) {
	cases := []struct {
		event   string
		want    billing.Signal
		handled bool
	}{
		{"alerts.spend_threshold_reached", billing.SignalAlert, true},
		// Gates only while no card is on file (billing.computeStatus). Both
		// contract-credit variants map, since we issue no commits.
		{"alerts.low_remaining_contract_credit_balance_reached", billing.SignalCreditsExhausted, true},
		{"alerts.low_remaining_contract_credit_and_commit_balance_reached", billing.SignalCreditsExhausted, true},
		// Not a real alert_type — guards against reintroducing the invented name.
		{"alerts.low_remaining_credit_balance_reached", "", false},
		// Non-suspend alerts are UI banners, not gating signals.
		{"alerts.usage_threshold_reached", "", false},
		{"alerts.low_remaining_commit_balance_reached", "", false},
		{"alerts.low_remaining_contract_credit_percentage_reached", "", false},
		{"invoice.finalized", "", false},
		{"contract.create", "", false},
		// Payment/recovery do not exist on the Metronome side (Stripe owns them).
		{"invoice.payment_failed", "", false},
		{"invoice.paid", "", false},
	}
	for _, tc := range cases {
		got, ok := metronomeSignal(tc.event)
		if ok != tc.handled || got != tc.want {
			t.Errorf("metronomeSignal(%q) = (%q, %v), want (%q, %v)", tc.event, got, ok, tc.want, tc.handled)
		}
	}
}
