package fake

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// The fake exists to produce payment outcomes real Metronome cannot until its
// Stripe connection is configured. A fake that stopped covering one of them
// would leave that branch of the invoices table unreachable again.
func TestInvoicesCoverEveryPaymentOutcome(t *testing.T) {
	raw, err := New().Invoices(context.Background(), "fake-cus-1")
	if err != nil {
		t.Fatalf("invoices: %v", err)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var invoices []struct {
		Status          string `json:"status"`
		IssuedAt        string `json:"issued_at"`
		ExternalInvoice struct {
			BillingProviderType string `json:"billing_provider_type"`
			ExternalStatus      string `json:"external_status"`
		} `json:"external_invoice"`
	}
	if err := json.Unmarshal(encoded, &invoices); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	seen := map[string]bool{}
	for _, inv := range invoices {
		seen[inv.Status+"/"+inv.ExternalInvoice.ExternalStatus] = true
	}
	for _, want := range []string{
		"DRAFT/",                   // pending, still accruing
		"FINALIZED/PAID",           // settled
		"FINALIZED/PAYMENT_FAILED", // the recovery case
		"FINALIZED/",               // issued, awaiting an outcome
		"FINALIZED/PARTIALLY_PAID", // in flight
		"FINALIZED/UNCOLLECTIBLE",  // written off
		"VOID/VOID",                // void
	} {
		if !seen[want] {
			t.Errorf("no invoice with status/external %q", want)
		}
	}
}

// An invoice awaiting an outcome and one that will never get one look the same
// apart from the provider type, and the client polls on exactly that
// difference. Both shapes have to appear or that branch goes untested.
func TestAwaitingInvoiceCarriesAProviderAndDraftCarriesNoIssueDate(t *testing.T) {
	raw, _ := New().Invoices(context.Background(), "fake-cus-1")
	encoded, _ := json.Marshal(raw)
	var invoices []struct {
		Status          string `json:"status"`
		IssuedAt        string `json:"issued_at"`
		ExternalInvoice struct {
			BillingProviderType string `json:"billing_provider_type"`
			ExternalStatus      string `json:"external_status"`
		} `json:"external_invoice"`
	}
	if err := json.Unmarshal(encoded, &invoices); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var sawAwaiting bool
	for _, inv := range invoices {
		if inv.Status == "FINALIZED" && inv.ExternalInvoice.ExternalStatus == "" {
			sawAwaiting = true
			if inv.ExternalInvoice.BillingProviderType == "" {
				t.Error("finalized invoice awaiting an outcome reports no provider, so nothing will wait for it")
			}
		}
		if inv.Status == "DRAFT" && inv.IssuedAt != "0001-01-01T00:00:00Z" {
			t.Errorf("draft carries an issue date %q; it has not been issued", inv.IssuedAt)
		}
	}
	if !sawAwaiting {
		t.Error("no finalized invoice awaiting an outcome")
	}
}

// dollarsAsCents is how the handler passes a threshold: SetSpendThresholds
// validates against maxSpendThresholdCents, and the client sends
// Math.round(dollars * 100). A test that set dollars here would pass against a
// provider comparing the wrong unit, which is exactly the bug it has to catch.
func dollarsAsCents(d float64) float64 { return d * 100 }

// The provider is the only store for in_alarm, so a limit under the period's
// spend has to report crossed or the paused-agent path never renders.
func TestSpendLimitUnderCurrentSpendReportsCrossed(t *testing.T) {
	p := New()
	ctx := context.Background()
	const customer = "fake-cus-1"

	spend, err := p.CustomerSpend(ctx, customer)
	if err != nil {
		t.Fatalf("spend: %v", err)
	}

	if err := p.SetCustomerSpendThreshold(ctx, customer, billing.SpendThresholdLimit, dollarsAsCents(spend.UsageSpend-1)); err != nil {
		t.Fatalf("set limit: %v", err)
	}
	got, err := p.CustomerSpendThresholds(ctx, customer)
	if err != nil {
		t.Fatalf("read thresholds: %v", err)
	}
	if !got.HasLimit || !got.Limit.InAlarm {
		t.Errorf("limit below spend reports has=%v in_alarm=%v", got.HasLimit, got.Limit.InAlarm)
	}

	if err := p.SetCustomerSpendThreshold(ctx, customer, billing.SpendThresholdLimit, dollarsAsCents(spend.UsageSpend+1)); err != nil {
		t.Fatalf("raise limit: %v", err)
	}
	got, _ = p.CustomerSpendThresholds(ctx, customer)
	if got.Limit.InAlarm {
		t.Error("limit above spend still reports crossed, so raising it would never resume agents")
	}

	if err := p.ClearCustomerSpendThreshold(ctx, customer, billing.SpendThresholdLimit); err != nil {
		t.Fatalf("clear limit: %v", err)
	}
	got, _ = p.CustomerSpendThresholds(ctx, customer)
	if got.HasLimit {
		t.Error("cleared limit still reported")
	}
}

// The unit is the whole bug: a threshold set in dollars must not read as
// crossed, because the handler never sends dollars.
func TestThresholdIsReadAsCentsNotDollars(t *testing.T) {
	p := New()
	ctx := context.Background()
	const customer = "fake-cus-1"

	spend, _ := p.CustomerSpend(ctx, customer)
	// A $100 limit is 10000 cents, far above the period's spend, so nothing
	// should be crossed. A provider comparing dollars would see 100 < 6.30
	// false and agree by accident, so the check below uses the other direction.
	if err := p.SetCustomerSpendThreshold(ctx, customer, billing.SpendThresholdLimit, dollarsAsCents(100)); err != nil {
		t.Fatalf("set limit: %v", err)
	}
	got, _ := p.CustomerSpendThresholds(ctx, customer)
	if got.Limit.InAlarm {
		t.Error("a $100 limit reports crossed against a period spending single dollars")
	}

	// Just under the period's spend, in cents. A provider comparing dollars
	// would read this as 630-ish against 6.30 and report not crossed.
	if err := p.SetCustomerSpendThreshold(ctx, customer, billing.SpendThresholdLimit, dollarsAsCents(spend.UsageSpend)-1); err != nil {
		t.Fatalf("set limit: %v", err)
	}
	got, _ = p.CustomerSpendThresholds(ctx, customer)
	if !got.Limit.InAlarm {
		t.Error("a limit one cent under the period's spend does not report crossed, so the units disagree")
	}
}

// Credit has to be partly drawn down, or the card renders neither the credit
// applied line nor a gap between usage and net spend.
func TestSpendReportsCreditPartlyApplied(t *testing.T) {
	spend, err := New().CustomerSpend(context.Background(), "fake-cus-1")
	if err != nil {
		t.Fatalf("spend: %v", err)
	}
	if spend.UsageSpend <= spend.CurrentSpend {
		t.Errorf("usage %v is not above net %v, so no credit reads as applied", spend.UsageSpend, spend.CurrentSpend)
	}
	if !spend.HasCredit || spend.CreditRemaining <= 0 {
		t.Errorf("credit has=%v remaining=%v; the free-credit badge needs some left", spend.HasCredit, spend.CreditRemaining)
	}
	if !spend.CurrentPeriodStart.Before(spend.CurrentPeriodEnd) {
		t.Error("period start is not before its end, so the Usage page has no window")
	}
}

// A shape the modal cannot read leaves it silently closed, which looks the
// same as no grant.
func TestBalancesCarryAUsdGrantTheClientCanRead(t *testing.T) {
	raw, err := New().Balances(context.Background(), "fake-cus-1")
	if err != nil {
		t.Fatalf("balances: %v", err)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out struct {
		Credits []struct {
			Balance        float64 `json:"balance"`
			AccessSchedule struct {
				CreditType struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"credit_type"`
				ScheduleItems []struct {
					Amount float64 `json:"amount"`
				} `json:"schedule_items"`
			} `json:"access_schedule"`
		} `json:"credits"`
		Commits []json.RawMessage `json:"commits"`
	}
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", encoded, err)
	}

	if len(out.Credits) != 1 {
		t.Fatalf("credits = %d, want 1", len(out.Credits))
	}
	credit := out.Credits[0]
	// The label alone parses as USD, so a wrong id would pass by accident.
	if credit.AccessSchedule.CreditType.ID != usdCentsCreditTypeID {
		t.Errorf("credit type id = %q, want %q", credit.AccessSchedule.CreditType.ID, usdCentsCreditTypeID)
	}
	granted := 0.0
	for _, item := range credit.AccessSchedule.ScheduleItems {
		granted += item.Amount
	}
	if granted <= 0 {
		t.Error("no granted amount on the schedule, so the modal has nothing to announce")
	}
	if credit.Balance >= granted {
		t.Errorf("balance %v is not below granted %v, so the two are indistinguishable", credit.Balance, granted)
	}
}
