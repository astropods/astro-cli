package riverqueue

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
	"github.com/astropods/astro/apps/astro-server/internal/payment"
)

type MetronomeWebhookArgs struct {
	EventID      string `json:"event_id" river:"unique"`
	EventType    string `json:"event_type"`
	CustomerID   string `json:"customer_id"`
	AlertName    string `json:"alert_name,omitempty"`
	Threshold    int64  `json:"threshold,omitempty"`
	CurrentSpend int64  `json:"current_spend,omitempty"`
	Detail       string `json:"detail,omitempty"` // provider error text, set only for metronomeAlarm events
}

func (MetronomeWebhookArgs) Kind() string { return "webhook.metronome" }

func (a MetronomeWebhookArgs) InsertOpts() river.InsertOpts {
	return webhookInsertOpts(a.EventID)
}

type StripeWebhookArgs struct {
	EventID          string `json:"event_id" river:"unique"`
	EventType        string `json:"event_type"`
	CustomerID       string `json:"customer_id"`
	HostedInvoiceURL string `json:"hosted_invoice_url,omitempty"`
}

func (StripeWebhookArgs) Kind() string { return "webhook.stripe" }

func (a StripeWebhookArgs) InsertOpts() river.InsertOpts {
	return webhookInsertOpts(a.EventID)
}

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

const spendThresholdReachedEvent = "alerts.spend_threshold_reached"

func isSpendWarning(eventType, alertName string) bool {
	return eventType == spendThresholdReachedEvent && alertName == billing.SpendWarningAlertName
}

func metronomeSignal(eventType, alertName string) (billing.Signal, bool) {
	if alertName == billing.SpendWarningAlertName {
		return "", false
	}
	switch eventType {
	case spendThresholdReachedEvent:
		return billing.SignalAlert, true
	case "alerts.low_remaining_contract_credit_balance_reached",
		"alerts.low_remaining_contract_credit_and_commit_balance_reached":
		return billing.SignalCreditsExhausted, true
	case "alerts.low_remaining_contract_credit_balance_resolved",
		"alerts.low_remaining_contract_credit_and_commit_balance_resolved":
		return billing.SignalCreditsGranted, true
	case "alerts.spend_threshold_resolved":
		return billing.SignalAlertResolved, true
	default:
		return "", false
	}
}

func metronomeAlarm(eventType string) bool {
	switch eventType {
	case "invoice.billing_provider_error", "integration.issue":
		return true
	default:
		return false
	}
}

type notifyFacts struct {
	HostedInvoiceURL string // Stripe's 3DS link, absolute
	ThresholdCents   int64
	SpentCents       int64
}

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
	case "invoice.paid":
		return billing.SignalRecovery, true
	case "payment_method.automatically_updated":
		return billing.SignalCardUpdated, true
	case "payment_method.attached":
		return billing.SignalCardAdded, true
	case "payment_method.detached":
		return billing.SignalCardRemoved, true
	default:
		return "", false
	}
}

type MetronomeWebhookWorker struct {
	river.WorkerDefaults[MetronomeWebhookArgs]
	accounts         *account.AccountStore
	status           *billing.StatusStore
	cards            cardReader           // nil when payments aren't configured
	thresholds       spendThresholdReader // nil when the backend reports no spend controls
	spend            spendReader          // nil when the backend does not report spend
	queue            *Queue               // set post-construction in New(); enqueues suspend/resume
	unlimitedDomains []string
	log              *logger.Logger
}

type spendThresholdReader interface {
	CustomerSpendThresholds(ctx context.Context, customerID string) (billing.SpendThresholds, error)
}

type spendReader interface {
	CustomerSpend(ctx context.Context, customerID string) (billing.Spend, error)
}

func spendThresholds(p billing.BillingProvider) spendThresholdReader {
	r, ok := p.(billing.SpendThresholdReader)
	if !ok {
		return nil
	}
	return r
}

func spendReports(p billing.BillingProvider) spendReader {
	r, ok := p.(billing.SpendReporter)
	if !ok {
		return nil
	}
	return r
}

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

const centsPerUnit = 100

func (w *MetronomeWebhookWorker) notifySpendWarning(ctx context.Context, args MetronomeWebhookArgs) error {
	if w.queue == nil {
		return nil
	}
	spent, err := w.spentCents(ctx, args.CustomerID, args.CurrentSpend)
	if err != nil {
		return err
	}
	return notifySpendWarning(ctx, w.log, w.accounts.GetByMetronomeCustomerID, w.queue, args, spent)
}

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
	if metronomeAlarm(job.Args.EventType) {
		w.log.Error("metronome webhook: integration failure",
			"type", job.Args.EventType, "customer_id", job.Args.CustomerID, "detail", job.Args.Detail)
		return nil
	}
	if w.accounts == nil || w.status == nil {
		return nil
	}
	if isSpendWarning(job.Args.EventType, job.Args.AlertName) {
		return w.notifySpendWarning(ctx, job.Args)
	}
	sig, ok := metronomeSignal(job.Args.EventType, job.Args.AlertName)
	if !ok {
		w.log.Info("metronome webhook: unhandled event", "type", job.Args.EventType)
		return nil
	}
	if sig == billing.SignalCreditsExhausted {
		unlimited, err := w.unlimitedAccount(job.Args.CustomerID)
		if err != nil {
			return err
		}
		if unlimited {
			w.log.Info("metronome webhook: credit exhaustion cannot gate an unlimited account",
				"customer_id", job.Args.CustomerID)
			return nil
		}
		if err := w.refreshCardFact(ctx, job.Args.CustomerID); err != nil {
			return err
		}
	}
	if sig == billing.SignalAlertResolved {
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

func (w *MetronomeWebhookWorker) unlimitedAccount(customerID string) (bool, error) {
	if len(w.unlimitedDomains) == 0 || customerID == "" {
		return false, nil
	}
	acct, err := w.accounts.GetByMetronomeCustomerID(customerID)
	if errors.Is(err, account.ErrAccountNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	email, err := w.accounts.GetCreatorVerifiedEmail(acct.ID)
	if err != nil {
		return false, err
	}
	return hasEmailDomain(email, w.unlimitedDomains), nil
}

func (w *MetronomeWebhookWorker) otherSpendAlertInAlarm(ctx context.Context, customerID, resolved string) (bool, error) {
	if w.thresholds == nil || customerID == "" {
		return false, nil
	}
	th, err := w.thresholds.CustomerSpendThresholds(ctx, customerID)
	if err != nil {
		return false, fmt.Errorf("read spend thresholds: %w", err)
	}
	if resolved == billing.SpendLimitAlertName {
		return th.OperatorSpendInAlarm, nil
	}
	return th.HasLimit && th.Limit.InAlarm, nil
}

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

type cardReader interface {
	DefaultCard(ctx context.Context, customerID string) (*payment.Card, error)
}

func paymentCards(p payment.Provider) cardReader {
	if p == nil {
		return nil
	}
	return p
}

func isCardSignal(s billing.Signal) bool {
	return s == billing.SignalCardAdded || s == billing.SignalCardRemoved
}

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
		if w.cards == nil {
			w.log.Info("stripe webhook: card event ignored, payments not configured", "type", job.Args.EventType)
			return nil
		}
		if job.Args.CustomerID == "" {
			w.log.Warn("stripe webhook: card event carries no customer", "type", job.Args.EventType, "event_id", job.Args.EventID)
			return nil
		}
		resolved, err := resolveCardSignal(ctx, w.cards, job.Args.CustomerID)
		if err != nil {
			return err
		}
		sig = resolved
	}
	switch sig {
	case billing.SignalActionRequired:
		if job.Args.HostedInvoiceURL != "" {
			// Stripe sends no email for a charge_automatically invoice, so this link
			// is the only route the customer has to authenticate.
			if err := w.storePayLink(ctx, job.Args.CustomerID, job.Args.HostedInvoiceURL); err != nil {
				return err
			}
		}
	case billing.SignalPaymentFailed:
		// Dunning is one marker for every open invoice, so a link stored for the
		// invoice that wanted authentication would still be offered when a later,
		// unrelated invoice declines. Paying it would settle a real debt and leave
		// the account stopped, which is the button this drops.
		if err := w.clearStalePayLink(ctx, job.Args.CustomerID, job.Args.HostedInvoiceURL); err != nil {
			return err
		}
	}
	return applyWebhookSignal(ctx, w.log, w.accounts.GetByStripeCustomerID, w.status, w.queue, "stripe", job.Args.CustomerID, sig, job.Args.EventID,
		notifyFacts{HostedInvoiceURL: job.Args.HostedInvoiceURL})
}

func (w *StripeWebhookWorker) clearStalePayLink(ctx context.Context, customerID, currentInvoiceURL string) error {
	acct, err := w.accounts.GetByStripeCustomerID(customerID)
	if errors.Is(err, account.ErrAccountNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return w.status.ClearStalePayLink(ctx, acct.ID, currentInvoiceURL)
}

func (w *StripeWebhookWorker) storePayLink(ctx context.Context, customerID, payLink string) error {
	// The link ends up in window.open. Stripe signs the webhook that carries it,
	// so this is a second lock rather than the first: a non-https scheme reaching
	// a browser is a script-execution vector, and no legitimate hosted page uses
	// one.
	if u, err := url.Parse(payLink); err != nil || u.Scheme != "https" || u.Host == "" {
		w.log.Warn("stripe webhook: refusing a pay link that is not an https URL", "customer_id", customerID)
		return nil
	}
	acct, err := w.accounts.GetByStripeCustomerID(customerID)
	if errors.Is(err, account.ErrAccountNotFound) {
		w.log.Warn("stripe webhook: no account for customer", "customer_id", customerID)
		return nil
	}
	if err != nil {
		return err
	}
	return w.status.SetPayLink(ctx, acct.ID, payLink)
}

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
		log.Warn(source+" webhook: no account for customer", "customer_id", customerID)
		return nil
	}
	if err != nil {
		return err
	}
	newStatus, changed, err := billing.ApplySignal(ctx, status, acct.ID, sig, time.Now())
	if err != nil {
		return err
	}
	if changed {
		log.Info("billing status changed", "source", source, "account_id", acct.ID, "status", string(newStatus), "signal", string(sig))
	}
	if sig == billing.SignalCreditsExhausted && !suspendedForCredits(ctx, status, acct.ID) {
		sig = ""
	}
	if ev, ok := billingAlert(sig, acct.ID, acct.Name, facts); ok && queue != nil {
		ev.DedupeKey = "billing:" + source + ":" + eventID
		if emitErr := queue.EmitBillingNotify(ctx, ev); emitErr != nil {
			log.Warn("billing: emit notification failed", "error", emitErr, "account_id", acct.ID, "signal", string(sig))
		}
	}
	return reconcileWorkloads(ctx, queue, acct.ID, newStatus)
}

func suspendedForCredits(ctx context.Context, status *billing.StatusStore, accountID string) bool {
	st, reason, err := status.Get(ctx, accountID)
	if err != nil {
		return false
	}
	return st == billing.StatusSuspended && reason == billing.ReasonCreditsExhausted
}

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
