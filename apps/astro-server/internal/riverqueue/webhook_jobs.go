package riverqueue

import (
	"context"
	"errors"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
)

// MetronomeWebhookArgs carries a verified Metronome webhook for off-request-path
// processing. The receiving handler only verifies the signature and enqueues;
// account mapping + status recompute happen here so every event is tracked and
// retried as a River job.
type MetronomeWebhookArgs struct {
	EventID    string `json:"event_id" river:"unique"`
	EventType  string `json:"event_type"`
	CustomerID string `json:"customer_id"`
}

func (MetronomeWebhookArgs) Kind() string { return "webhook.metronome" }

// InsertOpts routes to the billing queue and dedupes by event ID (ByArgs hashes
// only the river:"unique"-tagged EventID) so provider redeliveries collapse to
// one job. An empty event ID skips dedupe — better to double-process (ApplySignal
// is idempotent) than to collapse distinct id-less events into a single job.
func (a MetronomeWebhookArgs) InsertOpts() river.InsertOpts {
	return webhookInsertOpts(a.EventID)
}

// StripeWebhookArgs carries a verified Stripe payment-collection webhook for
// off-request-path processing. HostedInvoiceURL is present on
// invoice.payment_action_required so we can surface a 3DS link (Stripe does not
// email on charge_automatically invoices).
type StripeWebhookArgs struct {
	EventID          string `json:"event_id" river:"unique"`
	EventType        string `json:"event_type"`
	CustomerID       string `json:"customer_id"`
	HostedInvoiceURL string `json:"hosted_invoice_url,omitempty"`
}

func (StripeWebhookArgs) Kind() string { return "webhook.stripe" }

// InsertOpts mirrors MetronomeWebhookArgs.InsertOpts (billing queue + event-ID dedupe).
func (a StripeWebhookArgs) InsertOpts() river.InsertOpts {
	return webhookInsertOpts(a.EventID)
}

// webhookInsertOpts builds the shared routing/dedupe opts for webhook jobs.
func webhookInsertOpts(eventID string) river.InsertOpts {
	opts := river.InsertOpts{Queue: queueBilling}
	if eventID != "" {
		opts.UniqueOpts = river.UniqueOpts{ByArgs: true}
	}
	return opts
}

func init() {
	registerJobKind[MetronomeWebhookArgs]()
	registerJobKind[StripeWebhookArgs]()
}

// metronomeSignal maps a Metronome event type to a billing signal. The second
// return is false for events with no status effect (informational/lifecycle).
//
// Metronome has no payment-failure or recovery event — payment collection is
// entirely Stripe's domain (see the Stripe webhook). The only status-changing
// Metronome signal is a hard spend threshold → suspend. Other alerts
// (usage/credit/commit/invoice-total) are UI banners, not gating signals, so
// they stay unhandled here. Event names per the Metronome webhook catalog:
// https://docs.metronome.com/guides/platform-configuration/setup-webhooks
func metronomeSignal(eventType string) (billing.Signal, bool) {
	switch eventType {
	case "alerts.spend_threshold_reached":
		return billing.SignalAlert, true
	default:
		return "", false
	}
}

// billingAlert maps a billing signal to the owner-facing notification event, if
// one exists for it. Uncollectible/voided/card-updated have no user alert.
func billingAlert(sig billing.Signal, accountID, accountName, hostedInvoiceURL string) (notify.Event, bool) {
	switch sig {
	case billing.SignalPaymentFailed:
		return notify.BillingPaymentFailed(accountID, accountName), true
	case billing.SignalActionRequired:
		return notify.BillingActionRequired(accountID, accountName, hostedInvoiceURL), true
	case billing.SignalAlert:
		return notify.BillingSpendThreshold(accountID, accountName), true
	case billing.SignalRecovery:
		return notify.BillingRecovered(accountID, accountName), true
	default:
		return notify.Event{}, false
	}
}

// stripeSignal maps a Stripe event type to a billing signal. Only
// payment-collection events map; invoice-lifecycle and PM-CRUD events (owned by
// Metronome / the synchronous card vault) return false.
func stripeSignal(eventType string) (billing.Signal, bool) {
	switch eventType {
	case "invoice.payment_failed":
		return billing.SignalPaymentFailed, true
	case "invoice.payment_action_required":
		return billing.SignalActionRequired, true
	case "invoice.marked_uncollectible":
		return billing.SignalUncollectible, true
	case "invoice.voided":
		return billing.SignalVoided, true
	// invoice.paid is the single recovery trigger (invoice.payment_succeeded
	// overlaps — consume one, not both).
	case "invoice.paid":
		return billing.SignalRecovery, true
	case "payment_method.automatically_updated":
		return billing.SignalCardUpdated, true
	default:
		return "", false
	}
}

// MetronomeWebhookWorker applies a Metronome webhook's billing signal to the
// cached status and reconciles workload suspend/resume on a transition.
type MetronomeWebhookWorker struct {
	river.WorkerDefaults[MetronomeWebhookArgs]
	accounts *account.AccountStore
	status   *billing.StatusStore
	queue    *Queue // set post-construction in New(); enqueues suspend/resume
	log      *logger.Logger
}

func (w *MetronomeWebhookWorker) Work(ctx context.Context, job *river.Job[MetronomeWebhookArgs]) error {
	if w.accounts == nil || w.status == nil {
		return nil
	}
	sig, ok := metronomeSignal(job.Args.EventType)
	if !ok {
		w.log.Info("metronome webhook: unhandled event", "type", job.Args.EventType)
		return nil
	}
	return applyWebhookSignal(ctx, w.log, w.accounts.GetByMetronomeCustomerID, w.status, w.queue, "metronome", job.Args.CustomerID, sig, job.Args.EventID, "")
}

// StripeWebhookWorker applies a Stripe payment-collection webhook's billing
// signal to the cached status and reconciles workload suspend/resume.
type StripeWebhookWorker struct {
	river.WorkerDefaults[StripeWebhookArgs]
	accounts *account.AccountStore
	status   *billing.StatusStore
	queue    *Queue // set post-construction in New(); enqueues suspend/resume
	log      *logger.Logger
}

func (w *StripeWebhookWorker) Work(ctx context.Context, job *river.Job[StripeWebhookArgs]) error {
	if w.accounts == nil || w.status == nil {
		return nil
	}
	sig, ok := stripeSignal(job.Args.EventType)
	if !ok {
		w.log.Info("stripe webhook: unhandled event", "type", job.Args.EventType)
		return nil
	}
	if sig == billing.SignalActionRequired && job.Args.HostedInvoiceURL != "" {
		// Stripe does not email the customer for 3DS on charge_automatically
		// invoices; the app must surface this link. Log it for now (the client
		// reads pay-link state from the invoices endpoint).
		w.log.Info("stripe webhook: payment action required", "customer_id", job.Args.CustomerID, "hosted_invoice_url", job.Args.HostedInvoiceURL)
	}
	return applyWebhookSignal(ctx, w.log, w.accounts.GetByStripeCustomerID, w.status, w.queue, "stripe", job.Args.CustomerID, sig, job.Args.EventID, job.Args.HostedInvoiceURL)
}

// applyWebhookSignal is the shared body of both webhook workers: map the
// provider customer to an account, apply the signal, and reconcile workloads on
// a transition. An unknown customer or empty payload is a permanent no-op (nil,
// no retry); a DB error is returned so River retries.
func applyWebhookSignal(
	ctx context.Context,
	log *logger.Logger,
	lookup func(customerID string) (*account.Account, error),
	status *billing.StatusStore,
	queue *Queue,
	source, customerID string,
	sig billing.Signal,
	eventID, hostedInvoiceURL string,
) error {
	if status == nil || customerID == "" {
		return nil
	}
	acct, err := lookup(customerID)
	if errors.Is(err, account.ErrAccountNotFound) {
		// Unknown customer is permanent — acking (nil) is correct, don't retry.
		log.Warn(source+" webhook: no account for customer", "customer_id", customerID)
		return nil
	}
	if err != nil {
		// Transient (DB) error — return it so River retries rather than dropping
		// the signal (a lost payment_failed/uncollectible would bypass gating).
		return err
	}
	newStatus, changed, err := billing.ApplySignal(ctx, status, acct.ID, sig, time.Now())
	if err != nil {
		return err
	}
	if changed {
		log.Info("billing status changed", "source", source, "account_id", acct.ID, "status", string(newStatus), "signal", string(sig))
	}
	// Notify the owner of the payment event (deduped per provider event id so a
	// webhook retry doesn't re-send). Best-effort — a notification failure must
	// not fail the webhook and bypass gating.
	if ev, ok := billingAlert(sig, acct.ID, acct.Name, hostedInvoiceURL); ok && queue != nil {
		ev.DedupeKey = "billing:" + source + ":" + eventID
		if emitErr := queue.EmitNotify(ctx, ev); emitErr != nil {
			log.Warn("billing: emit notification failed", "error", emitErr, "account_id", acct.ID, "signal", string(sig))
		}
	}
	// Reconcile workloads to the current status on every handled event, not only
	// on a transition: if a prior enqueue was dropped, a later event re-attempts
	// it. A failed enqueue is returned so River retries the job — there is no
	// other backstop for resumes (the dunning sweep only re-enqueues suspends).
	// Suspend/resume are idempotent, so a no-op reconcile is cheap.
	return reconcileWorkloads(ctx, queue, acct.ID, newStatus)
}

// reconcileWorkloads enqueues workload suspend/resume to match the account's
// current billing status. Returns the enqueue error so the caller can retry.
func reconcileWorkloads(ctx context.Context, queue *Queue, accountID string, status billing.Status) error {
	if queue == nil {
		return nil
	}
	switch status {
	case billing.StatusSuspended:
		return queue.InsertBillingSuspend(ctx, accountID)
	case billing.StatusActive:
		return queue.InsertBillingResume(ctx, accountID)
	}
	return nil
}
