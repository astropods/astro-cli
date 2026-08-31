// Package payment defines the provider-agnostic seam for collecting and saving
// a customer's payment method. It is deliberately separate from internal/billing
// (metered usage ingest) and internal/quota (resource-count limits).
//
// The only backend today is Stripe, used as a card vault: astro-server creates
// a SetupIntent, the client confirms the card with Stripe.js, and the saved card
// is linked to the hosted billing provider (Metronome) which does the actual
// charging. astro-server never moves money.
package payment

import (
	"context"
	"time"
)

// Card is a saved payment method summarised for display. It carries no PAN —
// only the network brand, last four digits, and expiry.
type Card struct {
	ID       string `json:"id"`
	Brand    string `json:"brand"`
	Last4    string `json:"last4"`
	ExpMonth int64  `json:"exp_month"`
	ExpYear  int64  `json:"exp_year"`
}

// A method carrying no expiry is not a card, and is never expired.
func (c *Card) Expired(now time.Time) bool {
	if c == nil {
		return true
	}
	if c.ExpMonth < 1 || c.ExpMonth > 12 || c.ExpYear < 1 {
		return false
	}
	end := time.Date(int(c.ExpYear), time.Month(c.ExpMonth)+1, 1, 0, 0, 0, 0, time.UTC)
	return !now.UTC().Before(end)
}

// Provider is the card-vault contract. Implemented by the Stripe backend; nil
// when payments aren't configured (callers guard on provider == nil and report
// "not available", mirroring the billing seam).
type Provider interface {
	// CreateCustomer creates a vault customer for an Astro account and returns
	// the provider customer ID. accountID is attached as metadata for traceability.
	CreateCustomer(ctx context.Context, accountID, name, email string) (customerID string, err error)

	// CreateSetupIntent starts a card-collection flow for the customer and
	// returns the client secret the frontend confirms with Stripe.js.
	CreateSetupIntent(ctx context.Context, customerID string) (clientSecret string, err error)

	// ConfirmSetup re-reads the SetupIntent from the provider (authoritative,
	// not trusting the client), verifies it succeeded for this customer, detaches
	// any previously-saved cards, sets the new card as the customer's default,
	// and returns it.
	ConfirmSetup(ctx context.Context, customerID, setupIntentID string) (*Card, error)

	// DefaultCard returns the customer's saved card, or nil if none is on file.
	DefaultCard(ctx context.Context, customerID string) (*Card, error)

	// RemoveCard detaches all of the customer's saved cards.
	RemoveCard(ctx context.Context, customerID string) error

	// PublishableKey is the client-side key the frontend needs to load Stripe.js.
	PublishableKey() string
}
