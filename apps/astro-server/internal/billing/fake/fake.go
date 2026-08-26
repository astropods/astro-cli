// Package fake is a local-development billing provider. It answers every read
// the Billing and Usage pages make with deterministic data, including invoice
// payment outcomes that no other environment can produce: external_status is
// written by Metronome's own Stripe connection, so against real Metronome it is
// empty on every invoice until that connection exists.
//
// Thresholds and cards are held in memory, so a restart resets them. Nothing
// here talks to a network.
package fake

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// Provider is the local-development billing provider.
type Provider struct {
	mu         sync.Mutex
	aliases    map[string][]string
	thresholds map[string]billing.SpendThresholds
	// usageSpend is what the fake bills the open period, so a spend limit set
	// below it reports in_alarm the way a real crossed threshold would.
	usageSpend float64
}

var (
	_ billing.BillingProvider      = (*Provider)(nil)
	_ billing.SpendReporter        = (*Provider)(nil)
	_ billing.SpendThresholdReader = (*Provider)(nil)
	_ billing.SpendThresholdWriter = (*Provider)(nil)
	_ billing.PlanReporter         = (*Provider)(nil)
	_ billing.UsageThresholdReader = (*Provider)(nil)
)

// New returns a fake billing provider seeded with a plausible open period.
func New() *Provider {
	return &Provider{
		aliases:    map[string][]string{},
		thresholds: map[string]billing.SpendThresholds{},
		usageSpend: 6.30,
	}
}

// ---------------------------------------------------------------------------
// Period. Anchored to the current month so the Usage page's day axis and the
// picker's closed periods both land where a reader expects them.
// ---------------------------------------------------------------------------

func periodStart(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func periodEnd(now time.Time) time.Time {
	return periodStart(now).AddDate(0, 1, 0)
}

// ---------------------------------------------------------------------------
// BillingProvider
// ---------------------------------------------------------------------------

// CreateCustomer returns an id derived from the account so it is stable across
// restarts; the account store persists it.
func (p *Provider) CreateCustomer(ctx context.Context, a billing.Account) (string, error) {
	return "fake-cus-" + a.ID, nil
}

func (p *Provider) DeleteCustomer(ctx context.Context, customerID string) error { return nil }

func (p *Provider) SetIngestAliases(ctx context.Context, customerID string, aliases []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.aliases[customerID] = aliases
	return nil
}

func (p *Provider) GetIngestAliases(ctx context.Context, customerID string) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.aliases[customerID], nil
}

// IngestUsage accepts and drops. Reads are canned, so adding to them here
// would only make the page disagree with itself.
func (p *Provider) IngestUsage(ctx context.Context, events []billing.UsageEvent) error { return nil }

// UsageData mirrors Metronome's usage list: raw metric quantities, one row per
// metric per day window. Compute Units carries cu_hours, which is what the
// Usage page reads for its unit sub-line. LLM Usage carries dollars of upstream
// cost, not tokens, which is why only Compute has a quantity to show.
func (p *Provider) UsageData(ctx context.Context, customerID string, from, to time.Time) (any, error) {
	// No groups field. The real call passes no group_by, so Metronome never
	// populates one, and the Models breakdown it would feed does not render
	// anywhere. Inventing it here would put a section on the page that no
	// account can see.
	type row struct {
		BillableMetricID   string    `json:"billable_metric_id"`
		BillableMetricName string    `json:"billable_metric_name"`
		StartTimestamp     time.Time `json:"start_timestamp"`
		EndTimestamp       time.Time `json:"end_timestamp"`
		Value              float64   `json:"value"`
	}
	rows := []row{
		{
			BillableMetricID:   "fake-metric-compute",
			BillableMetricName: "Compute Units",
			StartTimestamp:     from,
			EndTimestamp:       to,
			Value:              219.05,
		},
		{
			BillableMetricID:   "fake-metric-gateway",
			BillableMetricName: "LLM Usage",
			StartTimestamp:     from,
			EndTimestamp:       to,
			Value:              4.32,
		},
	}
	return rows, nil
}

// DailySpend is rated, unlike UsageData: by_product carries each stream's own
// dollars so the header does not have to subtract one from a total.
func (p *Provider) DailySpend(ctx context.Context, customerID string, from, to time.Time) (any, error) {
	type point struct {
		Day       string             `json:"day"`
		Amount    float64            `json:"amount"`
		ByProduct map[string]float64 `json:"by_product,omitempty"`
	}
	var points []point
	now := time.Now().UTC()
	for d := from; d.Before(to) && d.Before(now); d = d.AddDate(0, 0, 1) {
		// A shaped series rather than a flat one, so the chart's axis, tooltip
		// and empty-day handling are all visible on one screen.
		compute := 0.18 + 0.06*float64(d.Day()%5)
		models := 0.05 + 0.03*float64(d.Day()%3)
		if d.Day()%7 == 0 {
			compute, models = 0, 0
		}
		points = append(points, point{
			Day:       d.Format("2006-01-02"),
			Amount:    compute + models,
			ByProduct: map[string]float64{"Compute Units": compute, "LLM Usage": models},
		})
	}
	return points, nil
}

// Invoices returns one invoice per payment outcome the table can render, plus
// the open draft. This is the reason the fake exists: against real Metronome
// every external_status is empty until its Stripe connection is configured, so
// Paid, Payment failed and the rest are otherwise unreachable.
func (p *Provider) Invoices(ctx context.Context, customerID string) (any, error) {
	type external struct {
		BillingProviderType  string `json:"billing_provider_type"`
		BillingProviderError string `json:"billing_provider_error,omitempty"`
		ExternalStatus       string `json:"external_status"`
		InvoiceID            string `json:"invoice_id"`
	}
	type creditType struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type invoice struct {
		ID              string     `json:"id"`
		Status          string     `json:"status"`
		Type            string     `json:"type"`
		Total           float64    `json:"total"`
		CreditType      creditType `json:"credit_type"`
		StartTimestamp  time.Time  `json:"start_timestamp"`
		EndTimestamp    time.Time  `json:"end_timestamp"`
		IssuedAt        time.Time  `json:"issued_at"`
		ExternalInvoice external   `json:"external_invoice"`
	}

	usd := creditType{ID: "2714e483-4ff1-48e4-9e25-ac732e8aa252", Name: "USD (cents)"}
	now := time.Now().UTC()
	start := periodStart(now)
	p.mu.Lock()
	draftTotal := p.usageSpend * 100
	p.mu.Unlock()

	// Newest first, matching what the provider returns.
	months := []struct {
		status   string
		external string
		total    float64
	}{
		{"DRAFT", "", draftTotal},
		{"FINALIZED", "PAID", 3184},
		{"FINALIZED", "PAYMENT_FAILED", 2790},
		{"FINALIZED", "", 1960},
		{"FINALIZED", "PARTIALLY_PAID", 4410},
		{"FINALIZED", "UNCOLLECTIBLE", 1205},
		{"VOID", "VOID", 0},
	}

	invoices := make([]invoice, 0, len(months))
	for i, m := range months {
		from := start.AddDate(0, -i, 0)
		to := from.AddDate(0, 1, 0)
		// Finalizing syncs the invoice downstream, so the external invoice
		// exists from that point. Its status starts empty and moves later,
		// which is the difference between waiting for an outcome and there
		// being no provider to report one.
		ext := external{}
		if m.status != "DRAFT" {
			ext = external{
				BillingProviderType: "stripe",
				ExternalStatus:      m.external,
				InvoiceID:           fmt.Sprintf("in_fake%02d", i),
			}
			if m.external == "PAYMENT_FAILED" {
				ext.BillingProviderError = "Your card was declined."
			}
		}
		inv := invoice{
			ID:              fmt.Sprintf("fake-inv-%08d-0000-0000-0000-000000000000", i),
			Status:          m.status,
			Type:            "USAGE",
			Total:           m.total,
			CreditType:      usd,
			StartTimestamp:  from,
			EndTimestamp:    to,
			ExternalInvoice: ext,
		}
		// A draft has not been issued, so it carries no issue date.
		if m.status != "DRAFT" {
			inv.IssuedAt = to
		}
		invoices = append(invoices, inv)
	}
	return invoices, nil
}

// InvoicePDF returns a one-page PDF so the download path is exercised end to
// end rather than erroring at the byte stream.
func (p *Provider) InvoicePDF(ctx context.Context, customerID, invoiceID string) (io.ReadCloser, error) {
	const pdf = "%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
		"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n" +
		"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 200 50]>>endobj\n" +
		"trailer<</Root 1 0 R>>\n%%EOF\n"
	return io.NopCloser(bytes.NewReader([]byte(pdf))), nil
}

// ---------------------------------------------------------------------------
// Optional interfaces the Billing and Usage pages assert for
// ---------------------------------------------------------------------------

func (p *Provider) CustomerSpend(ctx context.Context, customerID string) (billing.Spend, error) {
	p.mu.Lock()
	usage := p.usageSpend
	p.mu.Unlock()

	now := time.Now().UTC()
	// Credit covers part of the period, so UsageSpend and CurrentSpend differ.
	// That gap is what the card renders as "credit applied", and reporting one
	// number for both would hide it.
	const creditGranted = 10.0
	creditApplied := min(usage, creditGranted)

	return billing.Spend{
		Currency:           "USD",
		CreditRemaining:    creditGranted - creditApplied,
		HasCredit:          true,
		CurrentSpend:       usage - creditApplied,
		CurrentPeriodStart: periodStart(now),
		CurrentPeriodEnd:   periodEnd(now),
		HasCurrentSpend:    true,
		UsageSpend:         usage,
		HasUsageSpend:      true,
		LastInvoiceTotal:   31.84,
		LastInvoiceAt:      periodStart(now),
		HasLastInvoice:     true,
	}, nil
}

func (p *Provider) CustomerSpendThresholds(ctx context.Context, customerID string) (billing.SpendThresholds, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	t := p.thresholds[customerID]
	// Thresholds are carried in cents both ways, so the period's usage has to
	// be converted before it can be compared to one.
	usageCents := p.usageSpend * 100
	t.Warning.InAlarm = t.HasWarning && usageCents >= t.Warning.Amount
	t.Limit.InAlarm = t.HasLimit && usageCents >= t.Limit.Amount
	return t, nil
}

func (p *Provider) SetCustomerSpendThreshold(ctx context.Context, customerID string, kind billing.SpendThresholdKind, amount float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	t := p.thresholds[customerID]
	switch kind {
	case billing.SpendThresholdWarning:
		t.Warning = billing.SpendThreshold{Amount: amount}
		t.HasWarning = true
	case billing.SpendThresholdLimit:
		t.Limit = billing.SpendThreshold{Amount: amount}
		t.HasLimit = true
	}
	p.thresholds[customerID] = t
	return nil
}

func (p *Provider) ClearCustomerSpendThreshold(ctx context.Context, customerID string, kind billing.SpendThresholdKind) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	t := p.thresholds[customerID]
	switch kind {
	case billing.SpendThresholdWarning:
		t.Warning, t.HasWarning = billing.SpendThreshold{}, false
	case billing.SpendThresholdLimit:
		t.Limit, t.HasLimit = billing.SpendThreshold{}, false
	}
	p.thresholds[customerID] = t
	return nil
}

// CustomerPlan reports the credit-awarding plan, which is what provisioning
// puts a new account on.
func (p *Provider) CustomerPlan(ctx context.Context, customerID string) (billing.Plan, bool, error) {
	return billing.PlanCredit, true, nil
}

// CustomerUsageThresholds reports none. Per-metric caps are not self-serve, and
// an inherited one only exists on accounts that predate that decision.
func (p *Provider) CustomerUsageThresholds(ctx context.Context, customerID string) (map[billing.UsageMetric]billing.UsageThresholds, error) {
	return map[billing.UsageMetric]billing.UsageThresholds{}, nil
}
