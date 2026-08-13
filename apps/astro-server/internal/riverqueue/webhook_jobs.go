package riverqueue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
	"github.com/astropods/astro/apps/astro-server/internal/payment"
)

// MetronomeWebhookArgs carries a verified Metronome webhook for off-request-path
// processing. The receiving handler only verifies the signature and enqueues;
// account mapping + status recompute happen here so every event is tracked and
// retried as a River job.
type MetronomeWebhookArgs struct {
	EventID    string `json:"event_id" river:"unique"`
	EventType  string `json:"event_type"`
	CustomerID string `json:"customer_id"`
	Detail     string `json:"detail,omitempty"` // provider error text, set only for metronomeAlarm events
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
// entirely Stripe's domain (see the Stripe webhook). Two alerts gate: a hard
// spend threshold (suspend outright) and a spent credit balance (suspend only
// while no card is on file — see billing.computeStatus). The remaining alerts
// (usage/commit/invoice-total) are UI banners, not gating signals, so they stay
// unhandled here. Event names are the alert_type enum prefixed with "alerts.":
// https://docs.metronome.com/api-reference/alerts/create-a-threshold-notification
func metronomeSignal(eventType string) (billing.Signal, bool) {
	switch eventType {
	case "alerts.spend_threshold_reached":
		return billing.SignalAlert, true
	// Both contract-credit alerts mean the same thing here: the package's credit
	// balance hit its threshold. We issue no commits, so the and-commit variant
	// is equivalent — accept either so reconfiguring the alert to Metronome's
	// recommended type doesn't silently stop gating.
	case "alerts.low_remaining_contract_credit_balance_reached",
		"alerts.low_remaining_contract_credit_and_commit_balance_reached":
		return billing.SignalCreditsExhausted, true
	// The IN_ALARM -> OK edge. Without these both gating latches are one-way:
	// exhaustion exits only via a card or an operator, and the spend alert only
	// via a void. Resolved notifications are an account-level Metronome setting
	// covering every threshold type at once. Only the and-commit credit variant
	// is documented by name; the others follow the enum's "<alert_type>_resolved"
	// form, and an unmatched name falls through to the unhandled-event log.
	case "alerts.low_remaining_contract_credit_balance_resolved",
		"alerts.low_remaining_contract_credit_and_commit_balance_resolved":
		return billing.SignalCreditsGranted, true
	case "alerts.spend_threshold_resolved":
		return billing.SignalAlertResolved, true
	default:
		return "", false
	}
}

// metronomeAlarm reports whether the event is an integration failure rather than
// a billing signal. It is the only notice that invoices stopped reaching the
// billing provider. Account status is unaffected, so Metronome keeps finalizing
// invoices that are never delivered or paid.
func metronomeAlarm(eventType string) bool {
	switch eventType {
	case "invoice.billing_provider_error", "integration.issue":
		return true
	default:
		return false
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
	case billing.SignalCreditsExhausted:
		return notify.BillingCreditsExhausted(accountID, accountName), true
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
	// The card handlers write has_payment_method inline and best-effort, so
	// these are the backstop: Stripe redelivers, the inline write doesn't.
	case "payment_method.attached":
		return billing.SignalCardAdded, true
	// Refined in Work — replacing a card detaches the old one while the new one
	// is already attached, so the remaining cards decide.
	case "payment_method.detached":
		return billing.SignalCardRemoved, true
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
	cards    cardReader // nil when payments aren't configured
	queue    *Queue     // set post-construction in New(); enqueues suspend/resume
	log      *logger.Logger
}

func (w *MetronomeWebhookWorker) Work(ctx context.Context, job *river.Job[MetronomeWebhookArgs]) error {
	// Ahead of the store guard, which would otherwise drop the alarm on a backend
	// that has no billing status.
	if metronomeAlarm(job.Args.EventType) {
		w.log.Error("metronome webhook: integration failure",
			"type", job.Args.EventType, "customer_id", job.Args.CustomerID, "detail", job.Args.Detail)
		return nil
	}
	if w.accounts == nil || w.status == nil {
		return nil
	}
	sig, ok := metronomeSignal(job.Args.EventType)
	if !ok {
		w.log.Info("metronome webhook: unhandled event", "type", job.Args.EventType)
		return nil
	}
	if sig == billing.SignalCreditsExhausted {
		if err := w.refreshCardFact(ctx, job.Args.CustomerID); err != nil {
			return err
		}
	}
	return applyWebhookSignal(ctx, w.log, w.accounts.GetByMetronomeCustomerID, w.status, w.queue, "metronome", job.Args.CustomerID, sig, job.Args.EventID, "")
}

// refreshCardFact re-reads whether the account has a card before exhaustion is
// applied, since that is the one signal whose outcome depends on it. Without
// this, an account that vaulted a card before has_payment_method existed still
// reads false and gets suspended for a balance it is entitled to spend. An
// error stops the job rather than falling through to a stale false.
func (w *MetronomeWebhookWorker) refreshCardFact(ctx context.Context, metronomeCustomerID string) error {
	if w.cards == nil {
		return nil
	}
	acct, err := w.accounts.GetByMetronomeCustomerID(metronomeCustomerID)
	if errors.Is(err, account.ErrAccountNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	stripeID, err := w.accounts.GetStripeCustomerID(acct.ID)
	if err != nil {
		return err
	}
	if stripeID == "" {
		return nil
	}
	sig, err := resolveCardSignal(ctx, w.cards, stripeID)
	if err != nil {
		return err
	}
	_, _, err = billing.ApplySignal(ctx, w.status, acct.ID, sig, time.Now())
	return err
}

// cardReader reports whether a Stripe customer still has a card on file.
// Satisfied by payment.Provider.
type cardReader interface {
	DefaultCard(ctx context.Context, customerID string) (*payment.Card, error)
}

// paymentCards narrows an optional payment provider, keeping a nil interface
// nil rather than wrapping it in a non-nil cardReader.
func paymentCards(p payment.Provider) cardReader {
	if p == nil {
		return nil
	}
	return p
}

func isCardSignal(s billing.Signal) bool {
	return s == billing.SignalCardAdded || s == billing.SignalCardRemoved
}

// resolveCardSignal settles a provisional card signal against Stripe. The
// attach/detach pair for a replacement can arrive in either order, so the cards
// actually left on the customer decide, not the event name. An error is
// returned rather than guessed around: assuming "removed" would suspend a
// paying account.
func resolveCardSignal(ctx context.Context, cards cardReader, customerID string) (billing.Signal, error) {
	card, err := cards.DefaultCard(ctx, customerID)
	if err != nil {
		return "", fmt.Errorf("stripe webhook: read default card: %w", err)
	}
	if card != nil {
		return billing.SignalCardAdded, nil
	}
	return billing.SignalCardRemoved, nil
}

// StripeWebhookWorker applies a Stripe payment-collection webhook's billing
// signal to the cached status and reconciles workload suspend/resume.
type StripeWebhookWorker struct {
	river.WorkerDefaults[StripeWebhookArgs]
	accounts *account.AccountStore
	status   *billing.StatusStore
	cards    cardReader // nil when payments aren't configured
	queue    *Queue     // set post-construction in New(); enqueues suspend/resume
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
	if isCardSignal(sig) {
		// Without a Stripe client we can't tell a removal from a replacement,
		// and guessing wrong suspends a paying account.
		if w.cards == nil {
			w.log.Info("stripe webhook: card event ignored, payments not configured", "type", job.Args.EventType)
			return nil
		}
		resolved, err := resolveCardSignal(ctx, w.cards, job.Args.CustomerID)
		if err != nil {
			return err
		}
		sig = resolved
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
	if sig == billing.SignalCreditsExhausted && !suspendedForCredits(ctx, status, acct.ID) {
		// The alert fires for every account crossing zero, including
		// pay-as-you-go ones it does not gate. Telling those owners to add a
		// card they already have is worse than saying nothing.
		sig = ""
	}
	if ev, ok := billingAlert(sig, acct.ID, acct.Name, hostedInvoiceURL); ok && queue != nil {
		ev.DedupeKey = "billing:" + source + ":" + eventID
		if emitErr := queue.EmitBillingNotify(ctx, ev); emitErr != nil {
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

// suspendedForCredits reports whether the account is stopped specifically
// because its credits ran out. Reads the row rather than inferring from status,
// so an account suspended for a write-off is not told to add a card.
func suspendedForCredits(ctx context.Context, status *billing.StatusStore, accountID string) bool {
	st, reason, err := status.Get(ctx, accountID)
	if err != nil {
		return false
	}
	return st == billing.StatusSuspended && reason == billing.ReasonCreditsExhausted
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
