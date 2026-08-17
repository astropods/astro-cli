package riverqueue

import (
	"context"
	"errors"
	"testing"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

type fakeCollector struct {
	calls []string
	paid  int
	err   error
}

func (f *fakeCollector) CollectOpenInvoices(_ context.Context, customerID string) (int, error) {
	f.calls = append(f.calls, customerID)
	return f.paid, f.err
}

func collectJob(args BillingCollectArgs) *river.Job[BillingCollectArgs] {
	return &river.Job[BillingCollectArgs]{Args: args}
}

func TestBillingCollectChargesTheCustomer(t *testing.T) {
	c := &fakeCollector{paid: 1}
	w := &BillingCollectWorker{invoices: c, log: logger.New("error", "json")}

	err := w.Work(context.Background(), collectJob(BillingCollectArgs{
		AccountID: "acct_1", StripeCustomerID: "cus_1",
	}))
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(c.calls) != 1 || c.calls[0] != "cus_1" {
		t.Errorf("collected %v, want one call for cus_1", c.calls)
	}
}

// An account with no vaulted customer has no invoices to charge, and passing an
// empty id to the provider lists every open invoice on the platform.
func TestBillingCollectSkipsAnAccountWithNoCustomer(t *testing.T) {
	c := &fakeCollector{}
	w := &BillingCollectWorker{invoices: c, log: logger.New("error", "json")}

	if err := w.Work(context.Background(), collectJob(BillingCollectArgs{AccountID: "acct_1"})); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(c.calls) != 0 {
		t.Errorf("collected %v for an account with no customer", c.calls)
	}
}

// Payments are unconfigured in OSS and local builds, where the worker is still
// registered and still receives jobs.
func TestBillingCollectWithoutAProvider(t *testing.T) {
	w := &BillingCollectWorker{log: logger.New("error", "json")}
	if err := w.Work(context.Background(), collectJob(BillingCollectArgs{
		AccountID: "acct_1", StripeCustomerID: "cus_1",
	})); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

// A failure to reach the provider is transient. Swallowing it would leave the
// account suspended with its debt unattempted, which is the state this job
// exists to end.
func TestBillingCollectRetriesAProviderError(t *testing.T) {
	boom := errors.New("stripe unreachable")
	w := &BillingCollectWorker{invoices: &fakeCollector{err: boom}, log: logger.New("error", "json")}

	err := w.Work(context.Background(), collectJob(BillingCollectArgs{
		AccountID: "acct_1", StripeCustomerID: "cus_1",
	}))
	if !errors.Is(err, boom) {
		t.Fatalf("Work error = %v, want the provider error", err)
	}
}

// A double-submit from the card form must not charge the same invoice twice.
func TestBillingCollectDedupesByAccount(t *testing.T) {
	opts := BillingCollectArgs{}.InsertOpts()
	if !opts.UniqueOpts.ByArgs {
		t.Error("collection is not unique by args, so a repeated save charges twice")
	}
	if opts.UniqueOpts.ByPeriod == 0 {
		t.Error("collection has no uniqueness window")
	}
}

// paymentInvoices must keep a nil provider nil. Wrapping it would produce a
// non-nil interface holding a nil pointer, and the worker's guard would pass.
func TestPaymentInvoicesKeepsNilNil(t *testing.T) {
	if got := paymentInvoices(nil); got != nil {
		t.Errorf("paymentInvoices(nil) = %v, want nil", got)
	}
}
