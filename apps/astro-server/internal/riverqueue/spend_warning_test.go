package riverqueue

import (
	"context"
	"errors"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
)

// fakeNotifier records what the warning path emits instead of enqueueing it.
type fakeNotifier struct {
	events []notify.Event
	err    error
}

func (f *fakeNotifier) EmitBillingNotify(_ context.Context, ev notify.Event) error {
	f.events = append(f.events, ev)
	return f.err
}

func lookupOne(acct *account.Account, err error) func(string) (*account.Account, error) {
	return func(string) (*account.Account, error) { return acct, err }
}

// The warning reaches the owner through this path and no other, so what it emits
// is the whole feature: the right type, the account that crossed the threshold,
// and both amounts.
func TestNotifySpendWarningEmitsTheEvent(t *testing.T) {
	q := &fakeNotifier{}
	acct := &account.Account{ID: "acct_1", Name: "acme"}
	args := MetronomeWebhookArgs{
		EventID:      "evt_1",
		EventType:    spendThresholdReachedEvent,
		CustomerID:   "cust_1",
		AlertName:    billing.SpendWarningAlertName,
		Threshold:    8000,
		CurrentSpend: 8100,
	}

	if err := notifySpendWarning(context.Background(), logger.New("error", "json"), lookupOne(acct, nil), q, args, args.CurrentSpend); err != nil {
		t.Fatalf("notifySpendWarning: %v", err)
	}
	if len(q.events) != 1 {
		t.Fatalf("emitted %d events, want 1", len(q.events))
	}
	ev := q.events[0]
	if ev.Type != notify.TypeBillingSpendWarning {
		t.Errorf("type = %q, want %q", ev.Type, notify.TypeBillingSpendWarning)
	}
	if ev.AccountID != "acct_1" {
		t.Errorf("account = %q, want acct_1", ev.AccountID)
	}
	if ev.Payload[notify.PayloadThreshold] != "$80.00" || ev.Payload[notify.PayloadSpent] != "$81.00" {
		t.Errorf("amounts = %v / %v", ev.Payload[notify.PayloadThreshold], ev.Payload[notify.PayloadSpent])
	}
	// A provider redelivery repeats the event id, so the key is what stops the
	// owner being warned twice for one crossing.
	if ev.DedupeKey != "billing:metronome:evt_1" {
		t.Errorf("dedupe key = %q", ev.DedupeKey)
	}
}

// An unknown customer is permanent. Returning the error would retry a webhook
// that can never resolve; emitting nothing and acking is the same choice the
// signal path makes.
func TestNotifySpendWarningSkipsAnUnknownCustomer(t *testing.T) {
	q := &fakeNotifier{}
	err := notifySpendWarning(context.Background(), logger.New("error", "json"),
		lookupOne(nil, account.ErrAccountNotFound), q, MetronomeWebhookArgs{CustomerID: "cust_gone"}, 0)
	if err != nil {
		t.Fatalf("notifySpendWarning: %v", err)
	}
	if len(q.events) != 0 {
		t.Errorf("emitted %d events for an unknown customer", len(q.events))
	}
}

// A database error is transient, so it has to reach River rather than being
// swallowed: the warning would otherwise be dropped on one bad read.
func TestNotifySpendWarningReturnsATransientLookupError(t *testing.T) {
	q := &fakeNotifier{}
	boom := errors.New("connection refused")
	err := notifySpendWarning(context.Background(), logger.New("error", "json"),
		lookupOne(nil, boom), q, MetronomeWebhookArgs{CustomerID: "cust_1"}, 0)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the lookup error", err)
	}
}

// A notification failure must not fail the webhook. The gating decision travels
// the same job, so returning here would retry a signal that already applied.
func TestNotifySpendWarningSurvivesAFailedEmit(t *testing.T) {
	q := &fakeNotifier{err: errors.New("queue down")}
	acct := &account.Account{ID: "acct_1", Name: "acme"}
	if err := notifySpendWarning(context.Background(), logger.New("error", "json"),
		lookupOne(acct, nil), q, MetronomeWebhookArgs{CustomerID: "cust_1"}, 0); err != nil {
		t.Fatalf("notifySpendWarning: %v", err)
	}
}

// The limit and the warning are the same alert type at different numbers, so the
// name is the only thing separating a heads-up from a suspension.
func TestIsSpendWarning(t *testing.T) {
	cases := []struct {
		event, alert string
		want         bool
	}{
		{spendThresholdReachedEvent, billing.SpendWarningAlertName, true},
		{spendThresholdReachedEvent, billing.SpendLimitAlertName, false},
		// An operator's hand-made alert is not the customer's warning, and gating
		// on it is the behaviour that predates customer-set controls.
		{spendThresholdReachedEvent, "Platform backstop", false},
		{spendThresholdReachedEvent, "", false},
		// The resolved edge notifies nobody: telling an owner they dropped back
		// under their own warning is noise.
		{"alerts.spend_threshold_resolved", billing.SpendWarningAlertName, false},
		{"alerts.low_remaining_contract_credit_balance_reached", billing.SpendWarningAlertName, false},
	}
	for _, tc := range cases {
		if got := isSpendWarning(tc.event, tc.alert); got != tc.want {
			t.Errorf("isSpendWarning(%q, %q) = %v, want %v", tc.event, tc.alert, got, tc.want)
		}
	}
}

// A warning must notify without gating, and the two paths are mutually exclusive
// by construction: the worker takes the notify branch and returns before reaching
// the signal. If both were ever true, an account would be suspended for crossing
// the line it asked to be warned about.
func TestASpendWarningNeverProducesAGatingSignal(t *testing.T) {
	for _, event := range []string{spendThresholdReachedEvent, "alerts.spend_threshold_resolved"} {
		sig, handled := metronomeSignal(event, billing.SpendWarningAlertName)
		if handled {
			t.Errorf("metronomeSignal(%q, warning) returned %q; a warning must not gate", event, sig)
		}
	}
	// The limit still gates, or the control that stops agents stops nothing.
	if sig, handled := metronomeSignal(spendThresholdReachedEvent, billing.SpendLimitAlertName); !handled || sig != billing.SignalAlert {
		t.Errorf("metronomeSignal(reached, limit) = %q/%v, want alert/true", sig, handled)
	}
}

// The event carries the numbers so the message can state them. Without them the
// reader is told they crossed a threshold, but not which one or by how much.
func TestSpendNotificationsCarryTheAmounts(t *testing.T) {
	facts := notifyFacts{ThresholdCents: 2500, SpentCents: 2600}
	ev, ok := billingAlert(billing.SignalAlert, "acct_1", "acme", facts)
	if !ok {
		t.Fatal("SignalAlert produced no notification")
	}
	if got := ev.Payload[notify.PayloadThreshold]; got != "$25.00" {
		t.Errorf("threshold = %v, want $25.00", got)
	}
	if got := ev.Payload[notify.PayloadSpent]; got != "$26.00" {
		t.Errorf("spent = %v, want $26.00", got)
	}
}

// Stripe shares applyWebhookSignal and has no spend amounts. Rendering "$0.00"
// would put a threshold of nothing in an unrelated message.
func TestNonSpendNotificationsOmitTheAmounts(t *testing.T) {
	ev, ok := billingAlert(billing.SignalPaymentFailed, "acct_1", "acme", notifyFacts{})
	if !ok {
		t.Fatal("SignalPaymentFailed produced no notification")
	}
	if _, present := ev.Payload[notify.PayloadThreshold]; present {
		t.Error("payment_failed carries a threshold")
	}
}

// A non-processed status the classifier cannot prove is permanent retries. That
// is the right call for a lost message, but River's default of 25 attempts backs
// off by attempt^4 seconds and spans weeks, so an environment answering that
// status churns on every notification it sends.
func TestNotifyRetriesAreBounded(t *testing.T) {
	opts := NotifyArgs{}.InsertOpts()
	if opts.MaxAttempts == 0 || opts.MaxAttempts > 10 {
		t.Errorf("notify MaxAttempts = %d, want a bound of 10 or fewer", opts.MaxAttempts)
	}
}
