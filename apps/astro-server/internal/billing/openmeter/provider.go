package openmeter

import (
	"context"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// Provider adapts *Client to the billing.BillingProvider (and HostedBilling)
// interface. It maps the provider-agnostic billing.UsageEvent to the OpenMeter
// CloudEvent wire format, preserving the event UUID as the CloudEvent ID so
// idempotency/backfill dedupe behave exactly as before the seam.
//
// This is transitional: OpenMeter is deleted in Phase 4. New code depends on
// billing.BillingProvider, not on *Client.
//
// Client is a named (not embedded) field: several interface methods intentionally
// have different signatures than the underlying client (e.g. CreateCustomer), so
// explicit selectors avoid shadowing ambiguity.
type Provider struct {
	client *Client
}

// Compile-time assertions that the adapter satisfies both interfaces.
var (
	_ billing.BillingProvider = (*Provider)(nil)
	_ billing.HostedBilling   = (*Provider)(nil)
)

// NewProvider wraps an OpenMeter client as a billing.BillingProvider. Returns a
// true nil interface when c is nil (no OPENMETER_URL configured) so callers'
// `provider == nil` no-op guards work.
func NewProvider(c *Client) billing.BillingProvider {
	if c == nil {
		return nil
	}
	return &Provider{client: c}
}

// IngestUsage maps usage events to CloudEvents and forwards them to OpenMeter.
func (p *Provider) IngestUsage(ctx context.Context, events []billing.UsageEvent) error {
	if len(events) == 0 {
		return nil
	}
	ce := make([]CloudEvent, len(events))
	for i, e := range events {
		ts := e.Time
		if ts.IsZero() {
			ts = time.Now()
		}
		ce[i] = CloudEvent{
			ID:          e.TransactionID,
			Source:      "astro-server",
			SpecVersion: "1.0",
			Type:        e.Type,
			Subject:     e.AccountID,
			Time:        ts.UTC().Format(time.RFC3339),
			Data:        e.Properties,
		}
	}
	return p.client.IngestEvents(ctx, ce)
}

// CreateCustomer creates an OpenMeter customer for an Astro account.
func (p *Provider) CreateCustomer(ctx context.Context, a billing.Account) (string, error) {
	return p.client.CreateCustomer(ctx, a.ID, a.Name, a.Type, a.OwnerEmail)
}

// UpdateCustomer updates the OpenMeter customer's display name.
func (p *Provider) UpdateCustomer(ctx context.Context, customerID, name string) error {
	return p.client.UpdateCustomerName(ctx, customerID, name)
}

// DeleteCustomer removes the OpenMeter customer.
func (p *Provider) DeleteCustomer(ctx context.Context, customerID string) error {
	return p.client.DeleteCustomer(ctx, customerID)
}

// CheckBalance maps the OpenMeter customer-access view to a coarse balance gate.
// Transitional: the real balance/spend gate is provided by Metronome. OpenMeter
// entitlement enforcement continues to run through the middleware, so this is
// unused on the metering path in the current phase.
func (p *Provider) CheckBalance(ctx context.Context, customerID string) (billing.Balance, error) {
	access, err := p.client.GetCustomerAccess(ctx, customerID)
	if err != nil {
		return billing.Balance{}, err
	}
	// Gate on the compute entitlement when present; otherwise allow.
	bal := billing.Balance{Allow: true}
	if ent, ok := access.Entitlements["compute"]; ok {
		bal.Allow = ent.HasAccess
		if ent.Balance != nil {
			bal.RemainingUSD = *ent.Balance
		}
	}
	return bal, nil
}

// GetUsage is not modeled by the OpenMeter adapter (usage rendering reads the
// meter query API directly via the concrete client). Returns an empty report.
func (p *Provider) GetUsage(ctx context.Context, customerID string, from, to time.Time) (billing.UsageReport, error) {
	return billing.UsageReport{From: from, To: to}, nil
}

// GrantCredits is Metronome-only; OpenMeter does not support ad-hoc grants here.
func (p *Provider) GrantCredits(ctx context.Context, customerID string, usd float64, expiry time.Time, reason string) error {
	return billing.ErrUnsupported
}

// ProvisionPackaging subscribes the customer to a plan (OpenMeter subscription).
func (p *Provider) ProvisionPackaging(ctx context.Context, customerID string, plan billing.PackagingPlan) error {
	return p.client.CreateSubscription(ctx, customerID, plan.Key)
}
