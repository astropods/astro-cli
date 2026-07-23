// Package noop is the OSS billing provider: unmetered, no customer records. It
// satisfies billing.BillingProvider so the rest of astro-server depends only on
// the interface. Quotas still apply (they are DB-backed and independent of
// billing); consumption is simply never metered.
package noop

import (
	"context"
	"io"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// Provider is the no-op billing provider.
type Provider struct{}

// Compile-time assertion that Provider satisfies BillingProvider.
var _ billing.BillingProvider = (*Provider)(nil)

// New returns a no-op billing provider.
func New() *Provider { return &Provider{} }

// CreateCustomer is a no-op; OSS keeps no customer records. Returns an empty ID.
func (p *Provider) CreateCustomer(ctx context.Context, a billing.Account) (string, error) {
	return "", nil
}

// DeleteCustomer is a no-op.
func (p *Provider) DeleteCustomer(ctx context.Context, customerID string) error { return nil }

// SetIngestAliases is a no-op; OSS keeps no customer records.
func (p *Provider) SetIngestAliases(ctx context.Context, customerID string, aliases []string) error {
	return nil
}

// IngestUsage discards usage; OSS is unmetered.
func (p *Provider) IngestUsage(ctx context.Context, events []billing.UsageEvent) error { return nil }

// UsageData reports no billing data; OSS keeps none.
func (p *Provider) UsageData(ctx context.Context, customerID string, from, to time.Time) (any, error) {
	return nil, billing.ErrBillingUnavailable
}

// Invoices reports no billing data; OSS keeps none.
func (p *Provider) Invoices(ctx context.Context, customerID string) (any, error) {
	return nil, billing.ErrBillingUnavailable
}

// InvoicePDF reports no billing data; OSS keeps none.
func (p *Provider) InvoicePDF(ctx context.Context, customerID, invoiceID string) (io.ReadCloser, error) {
	return nil, billing.ErrBillingUnavailable
}

// Balances reports no billing data; OSS keeps none.
func (p *Provider) Balances(ctx context.Context, customerID string) (any, error) {
	return nil, billing.ErrBillingUnavailable
}
