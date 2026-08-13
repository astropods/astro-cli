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
		// IN_ALARM -> OK clears the exhaustion latch, whichever variant fires.
		{"alerts.low_remaining_contract_credit_balance_resolved", billing.SignalCreditsGranted, true},
		{"alerts.low_remaining_contract_credit_and_commit_balance_resolved", billing.SignalCreditsGranted, true},
		// Clears the alert latch, which a payment deliberately does not.
		{"alerts.spend_threshold_resolved", billing.SignalAlertResolved, true},
		// Prepaid credit balance, not contract credit: a UI banner, not a gate.
		{"alerts.low_remaining_credit_balance_reached", "", false},
		// Resolved fires for every threshold type once enabled, so the non-gating
		// alerts get one too. They stay unhandled, same as their _reached twin.
		{"alerts.usage_threshold_resolved", "", false},
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

// A gate-clearing signal that no provider event produces is only reachable by an
// operator, which is how a topped-up account stayed suspended until someone
// forced a provisioning re-run. Assert each clear is reachable from a real event.
// The set mirrors billing's signalWrites spec (see signal_matrix_test.go).
func TestWebhookEvents_ReachEveryGateClearingSignal(t *testing.T) {
	corpus := []string{
		// Stripe payment-collection events.
		"invoice.payment_failed", "invoice.payment_action_required", "invoice.paid",
		"invoice.voided", "invoice.marked_uncollectible",
		"payment_method.automatically_updated", "payment_method.attached", "payment_method.detached",
		// Metronome alert events, both edges.
		"alerts.spend_threshold_reached", "alerts.spend_threshold_resolved",
		"alerts.low_remaining_contract_credit_balance_reached",
		"alerts.low_remaining_contract_credit_balance_resolved",
		"alerts.low_remaining_contract_credit_and_commit_balance_reached",
		"alerts.low_remaining_contract_credit_and_commit_balance_resolved",
	}
	reachable := map[billing.Signal]string{}
	for _, ev := range corpus {
		if sig, ok := stripeSignal(ev); ok {
			reachable[sig] = ev
		}
		if sig, ok := metronomeSignal(ev); ok {
			reachable[sig] = ev
		}
	}
	for _, sig := range []billing.Signal{
		billing.SignalRecovery, billing.SignalVoided, billing.SignalCardUpdated,
		billing.SignalCreditsGranted, billing.SignalAlertResolved,
	} {
		if _, ok := reachable[sig]; !ok {
			t.Errorf("no provider event maps to %q: the gate it clears can only be lifted by an operator", sig)
		}
	}
}

// Redelivery is normal: providers retry, and River collapses repeats by event id.
// An id-less event must skip dedupe instead of hashing to a shared key, or two
// unrelated events collapse into one job and the second signal is silently lost.
// ApplySignal is idempotent, so double-processing is the safe side of this trade.
func TestWebhookInsertOpts_DedupesOnlyWithAnEventID(t *testing.T) {
	if got := (MetronomeWebhookArgs{EventID: "evt_1"}).InsertOpts(); !got.UniqueOpts.ByArgs {
		t.Error("event with an id must dedupe: a provider retry would run the job twice")
	}
	if got := (MetronomeWebhookArgs{}).InsertOpts(); got.UniqueOpts.ByArgs {
		t.Error("id-less event must not dedupe: distinct events would collapse into one job")
	}
	if got := (StripeWebhookArgs{EventID: "evt_1"}).InsertOpts(); !got.UniqueOpts.ByArgs {
		t.Error("stripe event with an id must dedupe")
	}
	if got := (StripeWebhookArgs{}).InsertOpts(); got.UniqueOpts.ByArgs {
		t.Error("id-less stripe event must not dedupe")
	}
}
