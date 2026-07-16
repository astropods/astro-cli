// Package noop is the OSS billing provider: unmetered, no gating, no customer
// records. It satisfies billing.BillingProvider so the rest of astro-server
// depends only on the interface. Quotas still apply (they are DB-backed and
// independent of billing); consumption is simply never metered or balance-gated.
package noop

import (
	"context"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// Provider is the no-op billing provider.
type Provider struct{}

// Compile-time assertion that Provider satisfies BillingProvider. It
// deliberately does NOT implement HostedBilling — callers type-assert for that
// and skip hosted-only operations on OSS.
var _ billing.BillingProvider = (*Provider)(nil)

// New returns a no-op billing provider.
func New() *Provider { return &Provider{} }

// CreateCustomer is a no-op; OSS keeps no customer records. Returns an empty ID.
func (p *Provider) CreateCustomer(ctx context.Context, a billing.Account) (string, error) {
	return "", nil
}

// UpdateCustomer is a no-op.
func (p *Provider) UpdateCustomer(ctx context.Context, customerID, name string) error { return nil }

// DeleteCustomer is a no-op.
func (p *Provider) DeleteCustomer(ctx context.Context, customerID string) error { return nil }

// IngestUsage discards usage; OSS is unmetered.
func (p *Provider) IngestUsage(ctx context.Context, events []billing.UsageEvent) error { return nil }

// CheckBalance always allows: consumption is never balance-gated on OSS.
func (p *Provider) CheckBalance(ctx context.Context, customerID string) (billing.Balance, error) {
	return billing.Balance{Allow: true}, nil
}

// GetUsage returns an empty report; OSS does not track metered usage.
func (p *Provider) GetUsage(ctx context.Context, customerID string, from, to time.Time) (billing.UsageReport, error) {
	return billing.UsageReport{From: from, To: to}, nil
}
