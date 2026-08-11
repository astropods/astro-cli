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

// ErrInvoiceNotAvailable is returned by InvoicePDF when the invoice has no PDF
// yet (e.g. a draft invoice that isn't finalized). Callers treat it as a 404,
// not a server error.
var ErrInvoiceNotAvailable = errors.New("billing: invoice not available")

// ErrProvisionBlocked marks a provisioning failure only an operator can clear
// (a customer already covered by someone else's contract). Retrying cannot fix
// it, so callers should stop rather than back off.
var ErrProvisionBlocked = errors.New("billing: provisioning blocked")

// UsageEvent is a single metered usage record. TransactionID is the idempotency
// key (the metering UUID) so retries and backfills dedupe. Properties carries
// the metric payload (e.g. cu_hours, model, component).
type UsageEvent struct {
	TransactionID string         // idempotency key; reuse the event UUID
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

// Provisioner puts a customer on the rate card and grants signup credit. Kept
// off BillingProvider (interface assertion) so noop implements nothing.
type Provisioner interface {
	// ProvisionCustomer is idempotent — keyed on accountID provider-side. The
	// bool is false when the provider is unconfigured: nothing provisioned,
	// nothing failed, so callers must not record the account as done.
	ProvisionCustomer(ctx context.Context, customerID, accountID string) (bool, error)
}

// Coverage states, mirroring the decision ProvisionCustomer acts on.
const (
	CoverageNone    = "none"    // nothing covers the customer
	CoverageOurs    = "ours"    // covered by a contract provisioning created
	CoverageForeign = "foreign" // covered by a contract created elsewhere
)

// Contract is one contract covering a customer.
type Contract struct {
	ID            string
	Name          string
	UniquenessKey string
	RateCardID    string
	StartingAt    time.Time
	EndingBefore  time.Time // zero when open-ended
	Ours          bool
}

// Coverage is the provisioning verdict plus the contracts it was drawn from.
type Coverage struct {
	State     string
	Contracts []Contract
}

// Spend is the at-a-glance money view for one customer. Zero means "no data",
// so every amount carries its own presence flag.
type Spend struct {
	Currency string

	// CreditRemaining is what is left of the granted credit. This is the number
	// credit-exhaustion gating fires on.
	CreditRemaining float64
	HasCredit       bool

	// CurrentSpend is the open billing period's draft invoice total.
	CurrentSpend     float64
	CurrentPeriodEnd time.Time
	HasCurrentSpend  bool

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

// ContractInspector exposes the same coverage check provisioning makes, so the
// admin view reports the verdict rather than a second opinion. Kept off
// BillingProvider (interface assertion) so noop implements nothing.
type ContractInspector interface {
	ContractCoverage(ctx context.Context, customerID, accountID string) (Coverage, error)
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
