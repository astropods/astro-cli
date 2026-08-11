package metronome

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Metronome-Industries/metronome-go/v3"
	"github.com/Metronome-Industries/metronome-go/v3/packages/param"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// CustomerSpend summarises what a customer has left and is running up.
//
// Each part is read separately. A part that is missing and a part that failed
// both report no number, so the presence flags say which are real, and a
// failure does not discard the parts that succeeded.
func (p *Provider) CustomerSpend(ctx context.Context, customerID string) (billing.Spend, error) {
	var out billing.Spend
	var errs []error

	// Without include_balance every credit reads as zero, which is
	// indistinguishable from exhausted.
	credits := p.mc.V1.Customers.Credits.ListAutoPaging(ctx, metronome.V1CustomerCreditListParams{
		CustomerID:     customerID,
		CoveringDate:   param.NewOpt(time.Now()),
		IncludeBalance: param.NewOpt(true),
	})
	for credits.Next() {
		c := credits.Current()
		out.CreditRemaining += c.Balance
		out.HasCredit = true
	}
	// A part-read page understates the balance, so drop it rather than report
	// an incomplete total as a fact.
	if err := credits.Err(); err != nil {
		out.CreditRemaining, out.HasCredit = 0, false
		errs = append(errs, fmt.Errorf("metronome list credits: %w", err))
	}

	draft, err := p.latestInvoice(ctx, customerID, "DRAFT")
	switch {
	case err != nil:
		errs = append(errs, err)
	case draft != nil:
		out.CurrentSpend = draft.Total
		out.CurrentPeriodEnd = draft.EndTimestamp
		out.HasCurrentSpend = true
		out.Currency = draft.CreditType.Name
	}

	final, err := p.latestInvoice(ctx, customerID, "FINALIZED")
	switch {
	case err != nil:
		errs = append(errs, err)
	case final != nil:
		out.LastInvoiceTotal = final.Total
		out.LastInvoiceAt = final.IssuedAt
		out.HasLastInvoice = true
		if out.Currency == "" {
			out.Currency = final.CreditType.Name
		}
	}

	return out, errors.Join(errs...)
}

// latestInvoice returns the newest invoice in the given status, or nil when the
// customer has none.
func (p *Provider) latestInvoice(ctx context.Context, customerID, status string) (*metronome.Invoice, error) {
	page, err := p.mc.V1.Customers.Invoices.List(ctx, metronome.V1CustomerInvoiceListParams{
		CustomerID: customerID,
		Status:     param.NewOpt(status),
		Limit:      param.NewOpt(int64(1)),
		Sort:       metronome.V1CustomerInvoiceListParamsSortDateDesc,
	})
	if err != nil {
		return nil, fmt.Errorf("metronome list %s invoices: %w", status, err)
	}
	if len(page.Data) == 0 {
		return nil, nil
	}
	return &page.Data[0], nil
}
