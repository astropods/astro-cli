package riverqueue

import (
	"context"
	"errors"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// fakeSpendReader stands in for the provider's spend summary.
type fakeSpendReader struct {
	spend billing.Spend
	err   error
	calls int
}

func (f *fakeSpendReader) CustomerSpend(context.Context, string) (billing.Spend, error) {
	f.calls++
	return f.spend, f.err
}

func spendWorker(r spendReader) *MetronomeWebhookWorker {
	return &MetronomeWebhookWorker{spend: r, log: logger.New("error", "json")}
}

// The provider measures a spend threshold against usage before credit drawdown,
// but the amount it sends alongside the alert is the invoice total, which credit
// has already offset. An account on credit therefore crosses a threshold it is
// told it has spent nothing towards.
func TestSpentCents_StatesUsageNotTheCreditOffsetTotal(t *testing.T) {
	r := &fakeSpendReader{spend: billing.Spend{
		Currency:        "USD",
		CurrentSpend:    0, // the whole period is covered by credit
		HasCurrentSpend: true,
		UsageSpend:      4.987269620720156,
		HasUsageSpend:   true,
	}}
	// What the webhook reported: the credit-offset total, rounded to zero.
	got, err := spendWorker(r).spentCents(context.Background(), "cust_1", 0)
	if err != nil {
		t.Fatalf("spentCents: %v", err)
	}
	if got != 499 {
		t.Errorf("spent = %d cents, want 499: the message must state usage before credit", got)
	}
}

// Spend arrives scaled to the currency it names while a threshold arrives in the
// provider's minor units. The two share one message, so a missing conversion
// renders a real amount as a hundredth of itself.
func TestSpentCents_ConvertsCurrencyUnitsToMinorUnits(t *testing.T) {
	cases := []struct {
		name  string
		usage float64
		want  int64
	}{
		{"whole dollars", 50, 5000},
		{"fractional cents round", 4.987269620720156, 499},
		{"half cent rounds up", 0.125, 13},
		{"zero usage is a fact, not an absence", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeSpendReader{spend: billing.Spend{UsageSpend: tc.usage, HasUsageSpend: true}}
			got, err := spendWorker(r).spentCents(context.Background(), "cust_1", 999999)
			if err != nil {
				t.Fatalf("spentCents: %v", err)
			}
			if got != tc.want {
				t.Errorf("spent = %d, want %d", got, tc.want)
			}
		})
	}
}

// A threshold fires once. Stating a wrong amount is worse than stating a late
// one, so a failed read reaches River instead of falling back to the number that
// is known to read zero.
func TestSpentCents_AFailedReadRetries(t *testing.T) {
	boom := errors.New("metronome unavailable")
	_, err := spendWorker(&fakeSpendReader{err: boom}).spentCents(context.Background(), "cust_1", 8100)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the read error", err)
	}
}

// An invoice with no usage lines cannot say what was spent. The provider's own
// figure is then all there is, and it is still better than inventing a zero.
func TestSpentCents_FallsBackWhenThereIsNoUsageToState(t *testing.T) {
	r := &fakeSpendReader{spend: billing.Spend{HasUsageSpend: false}}
	got, err := spendWorker(r).spentCents(context.Background(), "cust_1", 8100)
	if err != nil {
		t.Fatalf("spentCents: %v", err)
	}
	if got != 8100 {
		t.Errorf("spent = %d, want the reported 8100", got)
	}
}

// A backend that reports no spend keeps the behaviour it had. Without this the
// nil reader would be dereferenced on every spend event.
func TestSpentCents_NoReaderKeepsTheReportedAmount(t *testing.T) {
	got, err := spendWorker(nil).spentCents(context.Background(), "cust_1", 8100)
	if err != nil {
		t.Fatalf("spentCents: %v", err)
	}
	if got != 8100 {
		t.Errorf("spent = %d, want the reported 8100", got)
	}
}

// The reader is keyed by customer, so asking without one would summarise the
// wrong account or error. Neither belongs in a message about this one.
func TestSpentCents_NoCustomerIsNotLookedUp(t *testing.T) {
	r := &fakeSpendReader{spend: billing.Spend{UsageSpend: 50, HasUsageSpend: true}}
	got, err := spendWorker(r).spentCents(context.Background(), "", 8100)
	if err != nil {
		t.Fatalf("spentCents: %v", err)
	}
	if r.calls != 0 {
		t.Errorf("read spend %d times for an empty customer id", r.calls)
	}
	if got != 8100 {
		t.Errorf("spent = %d, want the reported 8100", got)
	}
}
