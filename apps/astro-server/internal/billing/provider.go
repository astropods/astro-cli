// Package billing defines the provider-agnostic seam for usage metering and
// balance/spend gating. Concrete backends (OpenMeter today; Metronome and a
// no-op OSS provider in later phases) implement BillingProvider so the rest of
// astro-server depends only on this interface.
//
// Billing is intentionally separate from per-account resource quotas: quotas
// (max agents, deployments, members, …) are DB-backed limits enforced for OSS
// and hosted alike and live in internal/quota. This package only concerns
// metered consumption and the balance/spend gate.
package billing

import (
	"context"
	"errors"
	"time"
)

// ErrUnsupported is returned by provider methods that a given backend does not
// implement (e.g. HostedBilling methods on the no-op OSS provider).
var ErrUnsupported = errors.New("billing: operation not supported by provider")

// UsageEvent is a single metered usage record. TransactionID is the idempotency
// key (the metering UUID) so retries and backfills dedupe. Properties carries
// the metric payload (e.g. compute_unit_hours, model, component).
type UsageEvent struct {
	TransactionID string         // idempotency key; reuse the event UUID
	AccountID     string         // Astro account ID (ingest alias / subject)
	Type          string         // event type (compute_usage, active_agents, …)
	Time          time.Time      // event timestamp
	Properties    map[string]any // metric payload
}

// Balance is the result of a balance/spend gate check. It is NOT a resource
// count — resource limits are quota's job. Allow is false only when the
// provider actively blocks further consumption (e.g. prepaid-overages-off and
// balance ≤ 0, or over a spend cap).
type Balance struct {
	Allow        bool    // false when consumption should be blocked
	RemainingUSD float64 // remaining balance in USD (v1 is USD-denominated)
}

// Account carries the fields a provider needs to create/link a customer.
type Account struct {
	ID         string
	Name       string
	Type       string
	OwnerEmail string
}

// UsageItem is one aggregated usage row for a customer over a window.
type UsageItem struct {
	Type    string            // event/meter type
	Value   float64           // aggregated value
	GroupBy map[string]string // dimension values (e.g. component, model)
}

// UsageReport is the aggregated usage for a customer over a window.
type UsageReport struct {
	From  time.Time
	To    time.Time
	Items []UsageItem
}

// PackagingPlan identifies a packaging/plan to provision for a customer.
type PackagingPlan struct {
	Key string // provider plan key
}

// BillingProvider is the provider-agnostic metering + balance-gate contract.
// Implemented by openmeter (transitional), noop (OSS), and metronome (hosted).
type BillingProvider interface {
	// CreateCustomer creates/links a provider customer for an Astro account and
	// returns the provider customer ID.
	CreateCustomer(ctx context.Context, a Account) (customerID string, err error)
	// UpdateCustomer updates the customer's display name.
	UpdateCustomer(ctx context.Context, customerID, name string) error
	// DeleteCustomer removes/archives the customer.
	DeleteCustomer(ctx context.Context, customerID string) error
	// IngestUsage sends a batch of metered usage events (idempotent per TransactionID).
	IngestUsage(ctx context.Context, events []UsageEvent) error
	// CheckBalance reports whether consumption is allowed and the remaining balance.
	CheckBalance(ctx context.Context, customerID string) (Balance, error)
	// GetUsage returns aggregated usage for a customer over [from, to).
	GetUsage(ctx context.Context, customerID string, from, to time.Time) (UsageReport, error)
}

// HostedBilling is the hosted-only surface (credit grants, packaging). The
// no-op OSS provider returns ErrUnsupported; callers type-assert for it.
type HostedBilling interface {
	GrantCredits(ctx context.Context, customerID string, usd float64, expiry time.Time, reason string) error
	ProvisionPackaging(ctx context.Context, customerID string, plan PackagingPlan) error
}
