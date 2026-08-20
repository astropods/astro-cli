package riverqueue

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
)

func TestUsageMessagesStateTheThresholdTheOwnerSet(t *testing.T) {
	facts := notifyFacts{
		UsageMetric:    billing.UsageMetricGateway,
		UsageThreshold: 25.5,
		ThresholdCents: 26, // what rounding would have shown
	}
	ev, ok := billingAlert(billing.SignalUsageLimit, "acct_1", "acme", facts)
	if !ok {
		t.Fatal("a usage limit produced no notification")
	}
	if got := ev.Payload[notify.PayloadThreshold]; got != "25.5" {
		t.Errorf("threshold = %v, want 25.5, the number the owner set", got)
	}
}

func TestASpendLimitKeepsTheSpendMessage(t *testing.T) {
	facts := notifyFacts{ThresholdCents: 5000, SpentCents: 5100, Period: "1 September 2026"}
	ev, ok := billingAlert(billing.SignalUsageLimit, "acct_1", "acme", facts)
	if !ok {
		t.Fatal("a spend limit produced no notification")
	}
	if got := ev.Payload[notify.PayloadThreshold]; got != "$50.00" {
		t.Errorf("threshold = %v, want $50.00", got)
	}
	if got := ev.Payload[notify.PayloadPeriod]; got != "1 September 2026" {
		t.Errorf("period = %v, want the period the spend accrued in", got)
	}
	if _, stated := ev.Payload[notify.PayloadUnit]; stated {
		t.Error("a spend message states no quantity unit")
	}
}
