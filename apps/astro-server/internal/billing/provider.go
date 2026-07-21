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
}

// BillingProvider is the provider-agnostic metering contract: customer
// lifecycle plus usage ingest. Implemented by noop (OSS) and metronome (hosted).
type BillingProvider interface {
	// CreateCustomer creates/links a provider customer for an Astro account and
	// returns the provider customer ID.
	CreateCustomer(ctx context.Context, a Account) (customerID string, err error)
	// DeleteCustomer removes/archives the customer.
	DeleteCustomer(ctx context.Context, customerID string) error
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
