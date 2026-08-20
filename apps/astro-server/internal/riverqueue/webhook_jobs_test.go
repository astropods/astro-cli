package riverqueue

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func TestStripeSignalMapping(t *testing.T) {
	cases := []struct {
		event   string
		want    billing.Signal
		handled bool
	}{
		{"invoice.payment_failed", billing.SignalPaymentFailed, true},
		{"invoice.payment_action_required", billing.SignalActionRequired, true},
		{"invoice.marked_uncollectible", billing.SignalUncollectible, true},
		{"invoice.voided", billing.SignalVoided, true},
		{"invoice.paid", billing.SignalRecovery, true},
		{"payment_method.automatically_updated", billing.SignalCardUpdated, true},
		// Backstop for the inline, best-effort has_payment_method write.
		// detached is provisional — Work re-reads Stripe to settle it.
		{"payment_method.attached", billing.SignalCardAdded, true},
		{"payment_method.detached", billing.SignalCardRemoved, true},
		// Lifecycle events Metronome owns — no signal.
		{"invoice.payment_succeeded", "", false},
		{"invoice.created", "", false},
		{"charge.succeeded", "", false},
	}
	for _, tc := range cases {
		got, ok := stripeSignal(tc.event)
		if ok != tc.handled || got != tc.want {
			t.Errorf("stripeSignal(%q) = (%q, %v), want (%q, %v)", tc.event, got, ok, tc.want, tc.handled)
		}
	}
}

func TestMetronomeSignalMapping(t *testing.T) {
	cases := []struct {
		event   string
		want    billing.Signal
		handled bool
	}{
		{"alerts.spend_threshold_reached", billing.SignalAlert, true},
		// Gates only while no card is on file (billing.computeStatus). Both
		// contract-credit variants map, since we issue no commits.
		{"alerts.low_remaining_contract_credit_balance_reached", billing.SignalCreditsExhausted, true},
		{"alerts.low_remaining_contract_credit_and_commit_balance_reached", billing.SignalCreditsExhausted, true},
		// IN_ALARM -> OK clears the exhaustion latch, whichever variant fires.
		{"alerts.low_remaining_contract_credit_balance_resolved", billing.SignalCreditsGranted, true},
		{"alerts.low_remaining_contract_credit_and_commit_balance_resolved", billing.SignalCreditsGranted, true},
		// Clears the alert latch, which a payment deliberately does not.
		{"alerts.spend_threshold_resolved", billing.SignalAlertResolved, true},
		// Prepaid credit balance, not contract credit: a UI banner, not a gate.
		{"alerts.low_remaining_credit_balance_reached", "", false},
		// Resolved fires for every threshold type once enabled, so the non-gating
		// alerts get one too. They stay unhandled, same as their _reached twin.
		{"alerts.usage_threshold_resolved", "", false},
		// Non-suspend alerts are UI banners, not gating signals.
		{"alerts.usage_threshold_reached", "", false},
		{"alerts.low_remaining_commit_balance_reached", "", false},
		{"alerts.low_remaining_contract_credit_percentage_reached", "", false},
		{"invoice.finalized", "", false},
		{"contract.create", "", false},
		// Payment/recovery do not exist on the Metronome side (Stripe owns them).
		{"invoice.payment_failed", "", false},
		{"invoice.paid", "", false},
	}
	for _, tc := range cases {
		got, ok := metronomeSignal(tc.event, "")
		if ok != tc.handled || got != tc.want {
			t.Errorf("metronomeSignal(%q) = (%q, %v), want (%q, %v)", tc.event, got, ok, tc.want, tc.handled)
		}
	}
}

// The two spend controls are the same alert type at different numbers, so only
// the name separates them. Gating on the warning would suspend an account for
// crossing the line it asked to be warned about, and clearing the latch on the
// warning's resolved edge would un-gate one the limit had stopped.
func TestMetronomeSignal_TheAccountsOwnWarningNeverGates(t *testing.T) {
	for _, ev := range []string{"alerts.spend_threshold_reached", "alerts.spend_threshold_resolved"} {
		if sig, ok := metronomeSignal(ev, "astro:spend_warning"); ok {
			t.Errorf("%s on the warning produced %q: a warning must not move billing status", ev, sig)
		}
		// The limit and the org-wide backstop are the same event, and both gate.
		if _, ok := metronomeSignal(ev, "astro:spend_limit"); !ok {
			t.Errorf("%s on the limit produced no signal", ev)
		}
		if _, ok := metronomeSignal(ev, "Hard spend threshold"); !ok {
			t.Errorf("%s on the org-wide backstop produced no signal", ev)
		}
	}
}

// An integration failure is an operator alarm, not a billing signal. Routing one
// into metronomeSignal would move an account's status on a delivery problem, and
// leaving it out of metronomeAlarm files it under unhandled at info level, where
// the invoices stop reaching Stripe with nothing to show for it.
func TestMetronomeAlarmMapping(t *testing.T) {
	cases := []struct {
		event string
		alarm bool
	}{
		{"invoice.billing_provider_error", true},
		{"integration.issue", true},
		{"alerts.spend_threshold_reached", false},
		{"invoice.finalized", false},
	}
	for _, tc := range cases {
		if got := metronomeAlarm(tc.event); got != tc.alarm {
			t.Errorf("metronomeAlarm(%q) = %v, want %v", tc.event, got, tc.alarm)
		}
		if _, handled := metronomeSignal(tc.event, ""); tc.alarm && handled {
			t.Errorf("metronomeSignal(%q) returned a signal: an alarm must not move billing status", tc.event)
		}
	}
}

// The worker returns early without stores, so an alarm logged after that guard
// is lost on any backend that has no billing status.
func TestMetronomeWebhookWorker_LogsAlarmWithoutStores(t *testing.T) {
	var buf bytes.Buffer
	w := &MetronomeWebhookWorker{log: &logger.Logger{
		Logger: slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})),
	}}
	job := &river.Job[MetronomeWebhookArgs]{Args: MetronomeWebhookArgs{
		EventID:   "evt_1",
		EventType: "invoice.billing_provider_error",
		Detail:    "STRIPE invoice inv_1: No token found",
	}}

	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("Work: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `"level":"ERROR"`) {
		t.Errorf("logged %q, want an error-level line", out)
	}
	if !strings.Contains(out, "No token found") {
		t.Errorf("logged %q, want the provider error text", out)
	}
}

// A gate-clearing signal that no provider event produces is only reachable by an
// operator, which is how a topped-up account stayed suspended until someone
// forced a provisioning re-run. Assert each clear is reachable from a real event.
// The set mirrors billing's signalWrites spec (see signal_matrix_test.go).
func TestWebhookEvents_ReachEveryGateClearingSignal(t *testing.T) {
	corpus := []string{
		// Stripe payment-collection events.
		"invoice.payment_failed", "invoice.payment_action_required", "invoice.paid",
		"invoice.voided", "invoice.marked_uncollectible",
		"payment_method.automatically_updated", "payment_method.attached", "payment_method.detached",
		// Metronome alert events, both edges.
		"alerts.spend_threshold_reached", "alerts.spend_threshold_resolved",
		"alerts.low_remaining_contract_credit_balance_reached",
		"alerts.low_remaining_contract_credit_balance_resolved",
		"alerts.low_remaining_contract_credit_and_commit_balance_reached",
		"alerts.low_remaining_contract_credit_and_commit_balance_resolved",
	}
	reachable := map[billing.Signal]string{}
	for _, ev := range corpus {
		if sig, ok := stripeSignal(ev); ok {
			reachable[sig] = ev
		}
		if sig, ok := metronomeSignal(ev, ""); ok {
			reachable[sig] = ev
		}
	}
	for _, sig := range []billing.Signal{
		billing.SignalRecovery, billing.SignalVoided, billing.SignalCardUpdated,
		billing.SignalCreditsGranted, billing.SignalAlertResolved,
	} {
		if _, ok := reachable[sig]; !ok {
			t.Errorf("no provider event maps to %q: the gate it clears can only be lifted by an operator", sig)
		}
	}
}

// Redelivery is normal: providers retry, and River collapses repeats by event id.
// An id-less event must skip dedupe instead of hashing to a shared key, or two
// unrelated events collapse into one job and the second signal is silently lost.
// ApplySignal is idempotent, so double-processing is the safe side of this trade.
func TestWebhookInsertOpts_DedupesOnlyWithAnEventID(t *testing.T) {
	if got := (MetronomeWebhookArgs{EventID: "evt_1"}).InsertOpts(); !got.UniqueOpts.ByArgs {
		t.Error("event with an id must dedupe: a provider retry would run the job twice")
	}
	if got := (MetronomeWebhookArgs{}).InsertOpts(); got.UniqueOpts.ByArgs {
		t.Error("id-less event must not dedupe: distinct events would collapse into one job")
	}
	if got := (StripeWebhookArgs{EventID: "evt_1"}).InsertOpts(); !got.UniqueOpts.ByArgs {
		t.Error("stripe event with an id must dedupe")
	}
	if got := (StripeWebhookArgs{}).InsertOpts(); got.UniqueOpts.ByArgs {
		t.Error("id-less stripe event must not dedupe")
	}
}

// fakeThresholds reports one customer's spend controls.
type fakeThresholds struct {
	limitInAlarm bool
	err          error
}

type fakeUsageCaps struct {
	inAlarm map[billing.UsageMetric]bool
	err     error
}

func (f fakeUsageCaps) CustomerUsageThresholds(context.Context, string) (map[billing.UsageMetric]billing.UsageThresholds, error) {
	out := make(map[billing.UsageMetric]billing.UsageThresholds, len(billing.AllUsageMetrics))
	for _, m := range billing.AllUsageMetrics {
		out[m] = billing.UsageThresholds{
			HasLimit: true,
			Limit:    billing.UsageThreshold{Amount: 10, InAlarm: f.inAlarm[m]},
		}
	}
	return out, f.err
}

func (f fakeThresholds) CustomerSpendThresholds(context.Context, string) (billing.SpendThresholds, error) {
	return billing.SpendThresholds{
		HasLimit: true,
		Limit:    billing.SpendThreshold{Amount: 5000, InAlarm: f.limitInAlarm},
	}, f.err
}

// An account can hold several limits of its own at once, on spend and on each
// metered quantity, and they share one latch. Clearing on the first to resolve
// restarts an account another limit still stops.
func TestMetronomeWebhook_ResolvedDoesNotClearALatchAnotherLimitHolds(t *testing.T) {
	cases := []struct {
		name       string
		resolved   string
		thresholds fakeThresholds
		caps       fakeUsageCaps
		wantHeld   bool
	}{
		{
			name:     "the spend limit resolves while a quantity cap is still over",
			resolved: billing.SpendLimitAlertName,
			caps:     fakeUsageCaps{inAlarm: map[billing.UsageMetric]bool{billing.UsageMetricCompute: true}},
			wantHeld: true,
		},
		{
			name:       "a quantity cap resolves while the spend limit is still over",
			resolved:   billing.UsageAlertName(billing.UsageMetricCompute, billing.SpendThresholdLimit),
			thresholds: fakeThresholds{limitInAlarm: true},
			wantHeld:   true,
		},
		{
			name:     "one quantity cap resolves while the other metric is still over",
			resolved: billing.UsageAlertName(billing.UsageMetricCompute, billing.SpendThresholdLimit),
			caps:     fakeUsageCaps{inAlarm: map[billing.UsageMetric]bool{billing.UsageMetricGateway: true}},
			wantHeld: true,
		},
		{
			name:     "nothing else is over, so the latch clears",
			resolved: billing.SpendLimitAlertName,
			wantHeld: false,
		},
		{
			name:       "a resolved spend limit does not hold the latch on its own account",
			resolved:   billing.SpendLimitAlertName,
			thresholds: fakeThresholds{limitInAlarm: true},
			wantHeld:   false,
		},
		{
			name:     "a resolved quantity cap does not hold the latch on its own account",
			resolved: billing.UsageAlertName(billing.UsageMetricCompute, billing.SpendThresholdLimit),
			caps:     fakeUsageCaps{inAlarm: map[billing.UsageMetric]bool{billing.UsageMetricCompute: true}},
			wantHeld: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &MetronomeWebhookWorker{thresholds: tc.thresholds, usage: tc.caps, log: logger.New("error", "json")}
			held, err := w.otherSelfLimitInAlarm(context.Background(), "cust_1", tc.resolved)
			if err != nil {
				t.Fatalf("otherSelfLimitInAlarm: %v", err)
			}
			if held != tc.wantHeld {
				t.Errorf("held = %v, want %v: clearing here restarts an account another limit still stops", held, tc.wantHeld)
			}
		})
	}
}

func TestMetronomeWebhook_NoThresholdReaderStillClears(t *testing.T) {
	w := &MetronomeWebhookWorker{log: logger.New("error", "json")}
	held, err := w.otherSelfLimitInAlarm(context.Background(), "cust_1", billing.SpendLimitAlertName)
	if err != nil || held {
		t.Fatalf("held = %v, err = %v: a backend without controls must still clear", held, err)
	}
}

func TestMetronomeWebhook_CreditAlertCannotSuspendAnUnlimitedAccount(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(metronomeCustomerRe).
		WithArgs("cus_1").
		WillReturnRows(accountByCustomerRow("acct_1"))
	mock.ExpectQuery(creatorEmailRe).
		WithArgs("acct_1").
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("employee@postman.com"))

	w := &MetronomeWebhookWorker{
		accounts:         account.NewAccountStore(db),
		status:           billing.NewStatusStore(db, 7),
		unlimitedDomains: []string{"postman.com"},
		log:              logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), creditAlertJob()); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the alert was allowed to gate an unlimited account: %v", err)
	}
}

func TestMetronomeWebhook_CreditAlertStillGatesAnOutsideAccount(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	mock.MatchExpectationsInOrder(false)

	for range 2 {
		mock.ExpectQuery(metronomeCustomerRe).
			WithArgs("cus_1").
			WillReturnRows(accountByCustomerRow("acct_1"))
	}
	mock.ExpectQuery(creatorEmailRe).
		WithArgs("acct_1").
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("someone@gmail.com"))
	mock.ExpectExec(`INSERT INTO account_billing_status`).
		WithArgs("acct_1").
		WillReturnError(errors.New("latch write failed"))

	w := &MetronomeWebhookWorker{
		accounts:         account.NewAccountStore(db),
		status:           billing.NewStatusStore(db, 7),
		unlimitedDomains: []string{"postman.com"},
		log:              logger.New("error", "json"),
	}
	err = w.Work(context.Background(), creditAlertJob())
	if err == nil || !strings.Contains(err.Error(), "latch write failed") {
		t.Fatalf("Work error = %v, want the exhaustion latch to have been written", err)
	}
}

const metronomeCustomerRe = `FROM accounts WHERE metronome_customer_id`

func accountByCustomerRow(id string) *sqlmock.Rows {
	now := time.Unix(0, 0)
	return sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
		AddRow(id, "acme", "org", now, now)
}

func creditAlertJob() *river.Job[MetronomeWebhookArgs] {
	return &river.Job[MetronomeWebhookArgs]{Args: MetronomeWebhookArgs{
		EventID:    "evt_1",
		EventType:  "alerts.low_remaining_contract_credit_balance_reached",
		CustomerID: "cus_1",
	}}
}

func TestStripeWebhook_ActionRequiredStoresTheHostedPage(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	mock.MatchExpectationsInOrder(false)

	const link = "https://invoice.stripe.com/i/acct_1/test"
	for range 2 {
		mock.ExpectQuery(`FROM accounts WHERE stripe_customer_id`).
			WithArgs("cus_1").
			WillReturnRows(accountByCustomerRow("acct_1"))
	}
	mock.ExpectExec(`INSERT INTO account_billing_status .*pay_link`).
		WithArgs("acct_1", link).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO account_billing_status`).
		WithArgs("acct_1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs("acct_1").WillReturnError(errors.New("stop here"))
	mock.ExpectRollback()

	w := &StripeWebhookWorker{
		accounts: account.NewAccountStore(db),
		status:   billing.NewStatusStore(db, 7),
		log:      logger.New("error", "json"),
	}
	_ = w.Work(context.Background(), &river.Job[StripeWebhookArgs]{Args: StripeWebhookArgs{
		EventID:          "evt_1",
		EventType:        "invoice.payment_action_required",
		CustomerID:       "cus_1",
		HostedInvoiceURL: link,
	}})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the hosted page was not stored: %v", err)
	}
}

// The pay link is handed to window.open. A javascript: URL would execute there,
// so the writer refuses anything that is not an https page rather than trusting
// the upstream field.
func TestStripeWebhook_RefusesAPayLinkThatIsNotHTTPS(t *testing.T) {
	for _, link := range []string{"javascript:alert(1)", "http://invoice.stripe.com/x", "not a url"} {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		mock.MatchExpectationsInOrder(false)
		// No expectations at all: the worker must not reach the account lookup.
		w := &StripeWebhookWorker{
			accounts: account.NewAccountStore(db),
			status:   billing.NewStatusStore(db, 7),
			log:      logger.New("error", "json"),
		}
		if err := w.storePayLink(context.Background(), "cus_1", link); err != nil {
			t.Errorf("%q: storePayLink returned %v, want a quiet refusal", link, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("%q: %v", link, err)
		}
		db.Close() //nolint:errcheck
	}
}

// Dunning is one marker for every open invoice, so the link stored for the
// invoice that wanted authentication outlives that invoice's relevance. A later,
// unrelated decline must not offer it: paying it settles a real debt and leaves
// the account stopped. The clear is conditional on the URL so a retry on the
// same invoice keeps the link, which is the case a blanket clear would destroy.
func TestStripeWebhook_ADifferentInvoiceFailingDropsTheStaleLink(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	mock.MatchExpectationsInOrder(false)

	const invoiceB = "https://invoice.stripe.com/i/acct_1/second"
	for range 2 {
		mock.ExpectQuery(`FROM accounts WHERE stripe_customer_id`).
			WithArgs("cus_1").
			WillReturnRows(accountByCustomerRow("acct_1"))
	}
	mock.ExpectExec(`UPDATE account_billing_status SET pay_link = NULL.*pay_link <> \$2`).
		WithArgs("acct_1", invoiceB).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO account_billing_status`).
		WithArgs("acct_1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs("acct_1").WillReturnError(errors.New("stop here"))
	mock.ExpectRollback()

	w := &StripeWebhookWorker{
		accounts: account.NewAccountStore(db),
		status:   billing.NewStatusStore(db, 7),
		log:      logger.New("error", "json"),
	}
	_ = w.Work(context.Background(), &river.Job[StripeWebhookArgs]{Args: StripeWebhookArgs{
		EventID:          "evt_2",
		EventType:        "invoice.payment_failed",
		CustomerID:       "cus_1",
		HostedInvoiceURL: invoiceB,
	}})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the stale link survived a different invoice failing: %v", err)
	}
}

// A failure that cannot name its invoice cannot prove the stored link is the one
// holding the account, so the link goes. Keeping it risks a payment that settles
// a different invoice and leaves the account stopped; dropping it falls back to
// replacing the card, which still resolves a decline.
func TestStripeWebhook_AFailureWithNoInvoiceURLClearsTheLink(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`FROM accounts WHERE stripe_customer_id`).
		WithArgs("cus_1").
		WillReturnRows(accountByCustomerRow("acct_1"))
	mock.ExpectExec(`UPDATE account_billing_status SET pay_link = NULL.*pay_link <> \$2`).
		WithArgs("acct_1", "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := &StripeWebhookWorker{
		accounts: account.NewAccountStore(db),
		status:   billing.NewStatusStore(db, 7),
		log:      logger.New("error", "json"),
	}
	if err := w.clearStalePayLink(context.Background(), "cus_1", ""); err != nil {
		t.Fatalf("clearStalePayLink returned %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("a link that no event vouches for survived: %v", err)
	}
}

func TestMetronomeWebhook_UsageLimitGatesAndWarningDoesNot(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		alertName string
		wantSig   billing.Signal
		wantSet   bool
	}{
		{
			name:      "a usage limit gates",
			eventType: "alerts.usage_threshold_reached",
			alertName: billing.UsageAlertName(billing.UsageMetricCompute, billing.SpendThresholdLimit),
			wantSig:   billing.SignalUsageLimit, wantSet: true,
		},
		{
			name:      "a usage warning never gates",
			eventType: "alerts.usage_threshold_reached",
			alertName: billing.UsageAlertName(billing.UsageMetricCompute, billing.SpendThresholdWarning),
			wantSet:   false,
		},
		{
			name:      "the gateway metric gates too",
			eventType: "alerts.usage_threshold_reached",
			alertName: billing.UsageAlertName(billing.UsageMetricGateway, billing.SpendThresholdLimit),
			wantSig:   billing.SignalUsageLimit, wantSet: true,
		},
		{
			name:      "the resolved edge lifts it",
			eventType: "alerts.usage_threshold_resolved",
			alertName: billing.UsageAlertName(billing.UsageMetricGateway, billing.SpendThresholdLimit),
			wantSig:   billing.SignalUsageLimitResolved, wantSet: true,
		},
		{
			name:      "an alert this build does not own is ignored",
			eventType: "alerts.usage_threshold_reached",
			alertName: "someone-elses-alert",
			wantSet:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig, ok := metronomeSignal(tc.eventType, tc.alertName)
			if ok != tc.wantSet {
				t.Fatalf("metronomeSignal(%s, %s) ok = %v, want %v", tc.eventType, tc.alertName, ok, tc.wantSet)
			}
			if ok && sig != tc.wantSig {
				t.Errorf("signal = %q, want %q", sig, tc.wantSig)
			}
		})
	}
}

func TestMetronomeWebhook_UsageWarningIsNotifyOnly(t *testing.T) {
	warning := billing.UsageAlertName(billing.UsageMetricCompute, billing.SpendThresholdWarning)
	limit := billing.UsageAlertName(billing.UsageMetricCompute, billing.SpendThresholdLimit)

	if !isUsageWarning("alerts.usage_threshold_reached", warning) {
		t.Error("the usage warning does not reach its notify-only branch")
	}
	if isUsageWarning("alerts.usage_threshold_reached", limit) {
		t.Error("the usage limit reached the notify-only branch, so it would never gate")
	}
	if isUsageWarning("alerts.usage_threshold_resolved", warning) {
		t.Error("the warning's resolved edge must not re-notify")
	}
}
