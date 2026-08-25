// Package billing defines the provider-agnostic seam for usage metering.
// Concrete backends (Metronome hosted; a no-op OSS provider) implement
// BillingProvider so the rest of astro-server depends only on this interface.
//
// Billing is intentionally separate from per-account resource quotas: quotas
// (max agents, deployments, members, …) are DB-backed limits enforced for OSS
// and hosted alike and live in internal/quota. This package only concerns
// metered consumption (customer lifecycle + usage ingest).
package billing

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrBillingUnavailable is returned by the read methods (UsageData, Invoices,
// Balances) when the provider has no hosted billing data (e.g. the OSS noop
// backend). Callers treat it as "billing not available" rather than a failure.
var ErrBillingUnavailable = errors.New("billing: not available")

// ErrInvoiceNotAvailable is returned by InvoicePDF when the provider holds no
// PDF for the invoice. A draft invoice is the common case, but a finalized one
// can lack a PDF too, so callers report absence rather than a stage. Treated as
// a 404, not a server error.
var ErrInvoiceNotAvailable = errors.New("billing: invoice not available")

// UsageEvent is a single metered usage record. TransactionID is the idempotency
// key, and it has to identify the span the event covers rather than the moment
// it was built, or a repeat is a second charge. Properties carries the metric
// payload (e.g. cu_hours, model, component).
type UsageEvent struct {
	TransactionID string         // idempotency key; derive it from the span
	AccountID     string         // Astro account ID (ingest alias / subject)
	Type          string         // event type (deployment_compute_usage, active_agents, …)
	Time          time.Time      // event timestamp
	Properties    map[string]any // metric payload
}

// Account carries the fields a provider needs to create/link a customer.
type Account struct {
	ID         string
	Name       string
	Type       string
	OwnerEmail string
	// BifrostCustomerID, when set, is added as an ingest alias at creation.
	BifrostCustomerID string
}

// Plan is the rate treatment an account is provisioned onto. Signup credit
// belongs to a person rather than an account, so only the first account a user
// provisions takes PlanCredit. PlanUnlimited rates every metered product at
// zero, so usage still meters and the statement never totals anything.
type Plan string

const (
	PlanCredit    Plan = "credit"
	PlanNoCredit  Plan = "no_credit"
	PlanUnlimited Plan = "unlimited"
)

// Provisioner puts a customer on the rate card and grants signup credit. Kept
// off BillingProvider (interface assertion) so noop implements nothing.
type Provisioner interface {
	// ProvisionCustomer is idempotent — keyed on accountID provider-side. The
	// bool is false when the provider is unconfigured: nothing provisioned,
	// nothing failed, so callers must not record the account as done.
	ProvisionCustomer(ctx context.Context, customerID, accountID string, plan Plan) (bool, error)
}

// Coverage states. There is one package, so any contract effective now bills
// the customer at the rates provisioning would have set, and the only question
// is whether one exists.
const (
	CoverageNone    = "none"
	CoverageCovered = "covered"
)

// Contract is one contract covering a customer.
type Contract struct {
	ID           string
	Name         string
	RateCardID   string
	StartingAt   time.Time
	EndingBefore time.Time // zero when open-ended
}

// Coverage is the provisioning verdict plus the contracts it was drawn from.
type Coverage struct {
	State     string
	Contracts []Contract
}

// Spend is the at-a-glance money view for one customer. Zero values mean "no
// data", which is why every field is reported alongside its own presence flag
// rather than inferred from being non-zero.
type Spend struct {
	// Currency names the unit all three amounts share, which holds because there
	// is one package and one credit type. Amounts arrive already converted, so
	// no caller rescales money.
	Currency string

	// CreditRemaining is what is left of the granted credit. This is the number
	// credit-exhaustion gating fires on.
	CreditRemaining float64
	HasCredit       bool

	// CurrentSpend is the open billing period's draft invoice total, which is
	// net of credit drawdown. It is what the account will be charged.
	CurrentSpend     float64
	CurrentPeriodEnd time.Time
	HasCurrentSpend  bool

	// UsageSpend is the same period's usage-based spend before credit drawdown.
	// The provider's spend threshold notification measures this, not the total,
	// so an account on credit crosses its own warning while CurrentSpend still
	// reads zero. A surface that shows a threshold has to show this number.
	UsageSpend    float64
	HasUsageSpend bool

	// LastInvoice is the most recent finalized invoice.
	LastInvoiceTotal float64
	LastInvoiceAt    time.Time
	HasLastInvoice   bool
}

// SpendReporter summarises a customer's money position. Kept off
// BillingProvider (interface assertion) so noop implements nothing.
type SpendReporter interface {
	CustomerSpend(ctx context.Context, customerID string) (Spend, error)
}

// SpendThreshold is one number a customer set for itself, plus whether the
// provider currently reports it crossed.
type SpendThreshold struct {
	Amount  float64
	InAlarm bool
}

// SpendThresholds are the customer's own warning and limit. The provider is the
// only store for them, so absence is reported by the Has flags rather than a
// zero, which is a threshold a customer could legitimately set.
type SpendThresholds struct {
	Warning    SpendThreshold
	HasWarning bool
	Limit      SpendThreshold
	HasLimit   bool
}

// Alert names for the two controls a customer sets. The provider is the only
// store for them, so the name is what tells one from the other and both from an
// operator's org-wide backstop. Declared here rather than in the provider
// package because the webhook path has to recognise them too, and a rename that
// reached only one side would silently make the warning gate.
const (
	SpendWarningAlertName = "astro:spend_warning"
	SpendLimitAlertName   = "astro:spend_limit"
)

// SpendThresholdKind names which of a customer's two controls is meant. They are
// the same provider primitive at different numbers, so nothing but this
// distinguishes them.
type SpendThresholdKind string

const (
	SpendThresholdWarning SpendThresholdKind = "warning" // warns, never gates
	SpendThresholdLimit   SpendThresholdKind = "limit"   // suspends the account
)

// MaxSelfServeSpendUSD is the highest monthly spend an account can put itself on
// the hook for without a human. A limit is collectible only up to what the card
// behind it settles, so an unbounded self-serve number is our exposure and not
// the customer's. $1,000 is where both OpenAI and Anthropic end self-serve and
// require an increase request, so past it an account should be talking to us.
//
// Dollars. Callers working in minor units scale it themselves.
const MaxSelfServeSpendUSD = 1000.00

// SpendThresholdReader reads the customer's own spend controls. Kept off
// BillingProvider (interface assertion) so noop implements nothing.
type SpendThresholdReader interface {
	CustomerSpendThresholds(ctx context.Context, customerID string) (SpendThresholds, error)
}

// SpendThresholdWriter sets and clears the customer's own spend controls. Both
// are idempotent: setting the number already in force changes nothing.
type SpendThresholdWriter interface {
	SetCustomerSpendThreshold(ctx context.Context, customerID string, kind SpendThresholdKind, amount float64) error
	ClearCustomerSpendThreshold(ctx context.Context, customerID string, kind SpendThresholdKind) error
}

type UsageMetric string

const (
	UsageMetricCompute UsageMetric = "compute" // CU-hours
	UsageMetricGateway UsageMetric = "gateway" // US dollars of upstream model cost
)

var AllUsageMetrics = []UsageMetric{UsageMetricCompute, UsageMetricGateway}

type UsageThreshold struct {
	Amount  float64
	InAlarm bool
}

type UsageThresholds struct {
	Warning    UsageThreshold
	HasWarning bool
	Limit      UsageThreshold
	HasLimit   bool
}

func UsageMetricUnit(m UsageMetric) string {
	if m == UsageMetricGateway {
		return "USD of model usage"
	}
	return "CU-hours"
}

func UsageAlertName(metric UsageMetric, kind SpendThresholdKind) string {
	return "astro:usage_" + string(kind) + ":" + string(metric)
}

func UsageMetricForAlert(name string) (UsageMetric, SpendThresholdKind, bool) {
	for _, m := range AllUsageMetrics {
		for _, k := range []SpendThresholdKind{SpendThresholdWarning, SpendThresholdLimit} {
			if name == UsageAlertName(m, k) {
				return m, k, true
			}
		}
	}
	return "", "", false
}

type UsageThresholdReader interface {
	CustomerUsageThresholds(ctx context.Context, customerID string) (map[UsageMetric]UsageThresholds, error)
}

type UsageThresholdWriter interface {
	SetCustomerUsageThreshold(ctx context.Context, customerID string, metric UsageMetric, kind SpendThresholdKind, amount float64) error
	ClearCustomerUsageThreshold(ctx context.Context, customerID string, metric UsageMetric, kind SpendThresholdKind) error
}

type UsageQuantityReader interface {
	CustomerMetricUsage(ctx context.Context, customerID string, metric UsageMetric) (float64, error)
}

// PlanReporter reports the plan the customer's live contract puts them on, and
// whether any contract covers them at all. A contract on a package this build
// does not know reports covered with no plan, and only the uncovered case is a
// reason to stop an account.
type PlanReporter interface {
	CustomerPlan(ctx context.Context, customerID string) (plan Plan, covered bool, err error)
}

// ContractInspector exposes the same coverage check provisioning makes, so the
// admin view reports that verdict rather than a second opinion. Kept off
// BillingProvider (interface assertion) so noop implements nothing.
type ContractInspector interface {
	ContractCoverage(ctx context.Context, customerID string) (Coverage, error)
}

// BillingProvider is the provider-agnostic metering contract: customer
// lifecycle plus usage ingest. Implemented by noop (OSS) and metronome (hosted).
type BillingProvider interface {
	// CreateCustomer creates/links a provider customer for an Astro account and
	// returns the provider customer ID.
	CreateCustomer(ctx context.Context, a Account) (customerID string, err error)
	// DeleteCustomer removes/archives the customer.
	DeleteCustomer(ctx context.Context, customerID string) error
	// SetIngestAliases replaces the customer's ingest aliases.
	SetIngestAliases(ctx context.Context, customerID string, aliases []string) error
	// GetIngestAliases returns the customer's current ingest aliases. Returns
	// ErrBillingUnavailable when the backend keeps no customer records (OSS noop).
	GetIngestAliases(ctx context.Context, customerID string) ([]string, error)
	// IngestUsage sends a batch of metered usage events (idempotent per TransactionID).
	IngestUsage(ctx context.Context, events []UsageEvent) error

	// The read methods below return provider data as-is (JSON-serializable
	// values) for the client to render. They return ErrBillingUnavailable when
	// the backend has no hosted billing (OSS noop).

	// UsageData returns metered usage over [from, to), aggregated per window.
	UsageData(ctx context.Context, customerID string, from, to time.Time) (any, error)
	// Invoices returns the customer's invoices (including line items).
	Invoices(ctx context.Context, customerID string) (any, error)
	// InvoicePDF returns a single invoice rendered as a PDF byte stream. The
	// caller must close the returned reader.
	InvoicePDF(ctx context.Context, customerID, invoiceID string) (io.ReadCloser, error)
	// Balances returns the customer's credits and commits.
	Balances(ctx context.Context, customerID string) (any, error)
}
