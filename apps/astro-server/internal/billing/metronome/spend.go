package metronome

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Metronome-Industries/metronome-go/v3"
	"github.com/Metronome-Industries/metronome-go/v3/packages/param"
	"github.com/Metronome-Industries/metronome-go/v3/shared"

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

	// covering_date drops credits whose access schedule has not started or has
	// ended; without include_balance every one reads as zero remaining, which is
	// indistinguishable from exhausted.
	credits := p.mc.V1.Customers.Credits.ListAutoPaging(ctx, metronome.V1CustomerCreditListParams{
		CustomerID:     customerID,
		CoveringDate:   param.NewOpt(time.Now()),
		IncludeBalance: param.NewOpt(true),
	})
	for credits.Next() {
		c := credits.Current()
		amount, unit := scaleAmount(c.Balance, c.AccessSchedule.CreditType)
		out.CreditRemaining += amount
		out.HasCredit = true
		if out.Currency == "" {
			out.Currency = unit
		}
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
		amount, unit := scaleAmount(draft.Total, draft.CreditType)
		out.CurrentSpend = amount
		out.CurrentPeriodEnd = draft.EndTimestamp
		out.HasCurrentSpend = true
		if out.Currency == "" {
			out.Currency = unit
		}
	}

	final, err := p.latestInvoice(ctx, customerID, "FINALIZED")
	switch {
	case err != nil:
		errs = append(errs, err)
	case final != nil:
		amount, unit := scaleAmount(final.Total, final.CreditType)
		out.LastInvoiceTotal = amount
		out.LastInvoiceAt = final.IssuedAt
		out.HasLastInvoice = true
		if out.Currency == "" {
			out.Currency = unit
		}
	}

	return out, errors.Join(errs...)
}

// usdCentsCreditTypeID is Metronome's built-in fiat unit, in hundredths. The id
// is fixed across accounts and documented on V1PricingUnitService.List.
const usdCentsCreditTypeID = "2714e483-4ff1-48e4-9e25-ac732e8f24f2" //nolint:gosec // public vendor id

// scaleAmount converts an amount into the unit named by the returned currency,
// so no caller has to know the vendor's. A credit type carries no precision
// field, so the id is the only stable signal; the name is Metronome's to reword.
func scaleAmount(v float64, ct shared.CreditTypeData) (float64, string) {
	if ct.ID == usdCentsCreditTypeID {
		return v / 100, "USD"
	}
	return v, ct.Name
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
