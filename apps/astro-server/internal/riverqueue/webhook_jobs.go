package riverqueue

import (
	"context"
	"errors"
	"fmt"
	"math"
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
	// AlertName tells an account's own spend warning from its limit, which are
	// the same alert type at different numbers.
	AlertName string `json:"alert_name,omitempty"`
	// Threshold and CurrentSpend are the alert's own numbers, in the credit
	// type's minor units. They ride along so the notification can state them:
	// a spend message without them tells the reader nothing actionable.
	Threshold    int64  `json:"threshold,omitempty"`
	CurrentSpend int64  `json:"current_spend,omitempty"`
	Detail       string `json:"detail,omitempty"` // provider error text, set only for metronomeAlarm events
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
// spendThresholdReachedEvent is the alert_type both spend controls fire under,
// which is why the alert name is what tells them apart.
const spendThresholdReachedEvent = "alerts.spend_threshold_reached"

// isSpendWarning reports the account's own warning crossing its threshold, which
// notifies without gating. The limit and the warning share one alert type, so the
// name is the only thing separating a heads-up from a suspension.
func isSpendWarning(eventType, alertName string) bool {
	return eventType == spendThresholdReachedEvent && alertName == billing.SpendWarningAlertName
}

func metronomeSignal(eventType, alertName string) (billing.Signal, bool) {
	// An account's own warning is the same alert type at a lower number, and it
	// exists to warn rather than stop. Gating on it would suspend an account for
	// crossing the line it asked to be told about. Its resolved edge is skipped
	// for the mirror-image reason: clearing the latch would un-gate an account
	// the limit had stopped.
	if alertName == billing.SpendWarningAlertName {
		return "", false
	}
	switch eventType {
	case spendThresholdReachedEvent:
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

// notifyFacts carries the event-specific values a notification states. Only the
// fields the signal in hand uses are set, which is why one struct beats adding a
// parameter per signal to a path Stripe and Metronome share.
type notifyFacts struct {
	HostedInvoiceURL string // Stripe's 3DS link, absolute
	ThresholdCents   int64
	SpentCents       int64
}

// billingAlert maps a billing signal to the owner-facing notification event, if
// one exists for it. Uncollectible/voided/card-updated have no user alert.
func billingAlert(sig billing.Signal, accountID, accountName string, facts notifyFacts) (notify.Event, bool) {
	switch sig {
	case billing.SignalPaymentFailed:
		return notify.BillingPaymentFailed(accountID, accountName), true
	case billing.SignalActionRequired:
		return notify.BillingActionRequired(accountID, accountName, facts.HostedInvoiceURL), true
	case billing.SignalAlert:
		return notify.BillingSpendThreshold(accountID, accountName, facts.ThresholdCents, facts.SpentCents), true
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
	accounts   *account.AccountStore
	status     *billing.StatusStore
	cards      cardReader           // nil when payments aren't configured
	thresholds spendThresholdReader // nil when the backend reports no spend controls
	spend      spendReader          // nil when the backend does not report spend
	queue      *Queue               // set post-construction in New(); enqueues suspend/resume
	log        *logger.Logger
}

// spendThresholdReader reports whether any of a customer's spend alerts is still
// in alarm. Satisfied by the metronome provider.
type spendThresholdReader interface {
	CustomerSpendThresholds(ctx context.Context, customerID string) (billing.SpendThresholds, error)
}

// spendReader summarises a customer's money position, which is where a spend
// message gets the amount it states. Satisfied by the metronome provider.
type spendReader interface {
	CustomerSpend(ctx context.Context, customerID string) (billing.Spend, error)
}

// spendThresholds narrows an optional provider, keeping a nil interface nil
// rather than wrapping it in a non-nil spendThresholdReader.
func spendThresholds(p billing.BillingProvider) spendThresholdReader {
	r, ok := p.(billing.SpendThresholdReader)
	if !ok {
		return nil
	}
	return r
}

// spendReports narrows an optional provider, keeping a nil interface nil rather
// than wrapping it in a non-nil spendReader.
func spendReports(p billing.BillingProvider) spendReader {
	r, ok := p.(billing.SpendReporter)
	if !ok {
		return nil
	}
	return r
}

// spentCents is the amount a spend message states. The provider measures a
// threshold against usage before credit drawdown, so the invoice total it sends
// alongside the alert reads zero for any account still on credit: an owner who
// set a $1 warning would be told they had spent $0.00 of it. Reading the usage
// figure ourselves also keeps the message and the billing page on one number.
//
// A read failure returns the error so the job retries, because a threshold fires
// once and a wrong amount is worse than a late one. A backend that reports no
// spend keeps the provider's figure, which is all it has.
func (w *MetronomeWebhookWorker) spentCents(ctx context.Context, customerID string, reported int64) (int64, error) {
	if w.spend == nil || customerID == "" {
		return reported, nil
	}
	s, err := w.spend.CustomerSpend(ctx, customerID)
	if err != nil {
		return 0, fmt.Errorf("read customer spend: %w", err)
	}
	if !s.HasUsageSpend {
		w.log.Warn("metronome webhook: no usage spend to state in a spend message",
			"customer_id", customerID)
		return reported, nil
	}
	return int64(math.Round(s.UsageSpend * centsPerUnit)), nil
}

// centsPerUnit converts the reporter's currency units back to the minor units a
// notification renders. Spend arrives scaled to the currency it names, while a
// threshold arrives in the provider's minor units, and the two share one message.
const centsPerUnit = 100

// notifySpendWarning tells the account's managers they crossed their own
// warning. It changes no status: the warning exists to warn, and gating on it
// would suspend an account for reaching the line it set as a heads-up.
func (w *MetronomeWebhookWorker) notifySpendWarning(ctx context.Context, args MetronomeWebhookArgs) error {
	// Guarded on the concrete type: a nil *Queue in an interface parameter is a
	// non-nil interface, so the check has to happen before the handoff.
	if w.queue == nil {
		return nil
	}
	spent, err := w.spentCents(ctx, args.CustomerID, args.CurrentSpend)
	if err != nil {
		return err
	}
	return notifySpendWarning(ctx, w.log, w.accounts.GetByMetronomeCustomerID, w.queue, args, spent)
}

// billingNotifier emits an owner-facing billing notification. Narrowed from
// *Queue so the warning path can be tested without River.
type billingNotifier interface {
	EmitBillingNotify(ctx context.Context, ev notify.Event) error
}

func notifySpendWarning(
	ctx context.Context,
	log *logger.Logger,
	lookup func(customerID string) (*account.Account, error),
	queue billingNotifier,
	args MetronomeWebhookArgs,
	spentCents int64,
) error {
	if args.CustomerID == "" {
		return nil
	}
	acct, err := lookup(args.CustomerID)
	if errors.Is(err, account.ErrAccountNotFound) {
		log.Warn("metronome webhook: no account for customer", "customer_id", args.CustomerID)
		return nil
	}
	if err != nil {
		return err
	}
	ev := notify.BillingSpendWarning(acct.ID, acct.Name, args.Threshold, spentCents)
	ev.DedupeKey = "billing:metronome:" + args.EventID
	if emitErr := queue.EmitBillingNotify(ctx, ev); emitErr != nil {
		log.Warn("billing: emit notification failed", "error", emitErr, "account_id", acct.ID, "signal", "spend_warning")
	}
	return nil
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
	// The warning yields no signal by design, and the notification path hangs off
	// the signal, so without this the account is never told it crossed the number
	// it asked to be told about.
	if isSpendWarning(job.Args.EventType, job.Args.AlertName) {
		return w.notifySpendWarning(ctx, job.Args)
	}
	sig, ok := metronomeSignal(job.Args.EventType, job.Args.AlertName)
	if !ok {
		w.log.Info("metronome webhook: unhandled event", "type", job.Args.EventType)
		return nil
	}
	if sig == billing.SignalCreditsExhausted {
		if err := w.refreshCardFact(ctx, job.Args.CustomerID); err != nil {
			return err
		}
	}
	if sig == billing.SignalAlertResolved {
		// An account can hold two spend alerts at once: its own limit and an
		// operator's org-wide backstop. They share one latch, so clearing on the
		// first to resolve would resume an account the other still stops. That is
		// reachable by design: raising your own limit above current spend resets
		// and resolves it while the backstop is untouched.
		stillOver, err := w.otherSpendAlertInAlarm(ctx, job.Args.CustomerID, job.Args.AlertName)
		if err != nil {
			return err
		}
		if stillOver {
			w.log.Info("metronome webhook: spend alert resolved but another is still in alarm",
				"customer_id", job.Args.CustomerID, "resolved", job.Args.AlertName)
			return nil
		}
	}
	facts := notifyFacts{ThresholdCents: job.Args.Threshold, SpentCents: job.Args.CurrentSpend}
	if sig == billing.SignalAlert {
		spent, err := w.spentCents(ctx, job.Args.CustomerID, job.Args.CurrentSpend)
		if err != nil {
			return err
		}
		facts.SpentCents = spent
	}
	return applyWebhookSignal(ctx, w.log, w.accounts.GetByMetronomeCustomerID, w.status, w.queue, "metronome", job.Args.CustomerID, sig, job.Args.EventID, facts)
}

// otherSpendAlertInAlarm reports whether a spend alert other than the one that
// just resolved is still over. Without a reader it answers false, which keeps a
// backend with no spend controls behaving as it did: one alert, one latch.
func (w *MetronomeWebhookWorker) otherSpendAlertInAlarm(ctx context.Context, customerID, resolved string) (bool, error) {
	if w.thresholds == nil || customerID == "" {
		return false, nil
	}
	th, err := w.thresholds.CustomerSpendThresholds(ctx, customerID)
	if err != nil {
		return false, fmt.Errorf("read spend thresholds: %w", err)
	}
	// Answer for the other side only. The reader collapses every alert that is
	// not the customer's own into one bool, so asking it about an operator alert
	// that just resolved counts that alert against itself: a list read still
	// reporting in_alarm holds the latch, the event is acked, and nothing else
	// resumes the account.
	if resolved == billing.SpendLimitAlertName {
		return th.OperatorSpendInAlarm, nil
	}
	return th.HasLimit && th.Limit.InAlarm, nil
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
	return applyWebhookSignal(ctx, w.log, w.accounts.GetByStripeCustomerID, w.status, w.queue, "stripe", job.Args.CustomerID, sig, job.Args.EventID,
		notifyFacts{HostedInvoiceURL: job.Args.HostedInvoiceURL})
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
	eventID string,
	facts notifyFacts,
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
	if ev, ok := billingAlert(sig, acct.ID, acct.Name, facts); ok && queue != nil {
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
