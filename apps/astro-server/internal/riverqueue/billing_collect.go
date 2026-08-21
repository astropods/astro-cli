package riverqueue

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/payment"
)

// BillingCollectArgs charges an account's open invoices against the card now on
// file. A card save is the only thing that enqueues it.
type BillingCollectArgs struct {
	AccountID        string `json:"account_id"`
	StripeCustomerID string `json:"stripe_customer_id"`
}

func (BillingCollectArgs) Kind() string { return "billing.collect" }

func (BillingCollectArgs) InsertOpts() river.InsertOpts {
	// One in-flight attempt per account. A double-submit from the card form must
	// not charge the same invoice twice.
	return river.InsertOpts{
		Queue: queueBilling,
		UniqueOpts: river.UniqueOpts{
			ByArgs:   true,
			ByPeriod: time.Minute,
		},
	}
}

func init() {
	registerJobKind[BillingCollectArgs]()
}

// invoiceCollector charges a customer's open invoices. Satisfied by
// payment.Provider.
type invoiceCollector interface {
	CollectOpenInvoices(ctx context.Context, customerID string) (int, error)
}

// paymentInvoices narrows an optional payment provider, keeping a nil interface
// nil rather than wrapping it in a non-nil invoiceCollector.
func paymentInvoices(p payment.Provider) invoiceCollector {
	c, ok := p.(invoiceCollector)
	if !ok {
		return nil
	}
	return c
}

// BillingCollectWorker asks the payment provider to charge the open invoices an
// account accrued while its card was failing.
//
// Nothing else closes that loop. A suspension for a failed payment clears only
// on `invoice.paid`, and the provider retries a failed invoice on its own
// schedule, which can be days away or already exhausted. Without this the owner
// fixes their card and stays suspended.
type BillingCollectWorker struct {
	river.WorkerDefaults[BillingCollectArgs]
	invoices invoiceCollector
	log      *logger.Logger
}

func (w *BillingCollectWorker) Work(ctx context.Context, job *river.Job[BillingCollectArgs]) error {
	if w.invoices == nil || job.Args.StripeCustomerID == "" {
		return nil
	}
	paid, err := w.invoices.CollectOpenInvoices(ctx, job.Args.StripeCustomerID)
	if err != nil {
		return err
	}
	// The status change rides the resulting provider webhook rather than being
	// written here. A payment that succeeds outside our window then reaches the
	// same code, so one path clears dunning instead of two.
	w.log.Info("billing collect: completed", "account_id", job.Args.AccountID, "paid", paid)
	return nil
}
