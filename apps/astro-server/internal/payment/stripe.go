package payment

import (
	"context"
	"fmt"

	"github.com/stripe/stripe-go/v86"
)

// StripeConfig holds the settings the Stripe card-vault needs.
type StripeConfig struct {
	SecretKey      string // STRIPE_SECRET_KEY
	PublishableKey string // STRIPE_PUBLISHABLE_KEY (surfaced to the client)
}

// Stripe is the Stripe-backed card vault. It implements Provider.
type Stripe struct {
	sc         *stripe.Client
	publishKey string
}

// Compile-time assertion.
var _ Provider = (*Stripe)(nil)

// NewStripe constructs a Stripe provider. Returns nil when no secret key is set
// so callers' `provider == nil` guards behave like the billing backends.
func NewStripe(cfg StripeConfig) *Stripe {
	if cfg.SecretKey == "" {
		return nil
	}
	return &Stripe{sc: stripe.NewClient(cfg.SecretKey), publishKey: cfg.PublishableKey}
}

func (s *Stripe) PublishableKey() string { return s.publishKey }

// CreateCustomer creates a Stripe customer keyed to the Astro account ID (stored
// as metadata) so the customer is traceable back to the account.
func (s *Stripe) CreateCustomer(ctx context.Context, accountID, name, email string) (string, error) {
	params := &stripe.CustomerCreateParams{
		Name:  stripe.String(name),
		Email: stripe.String(email),
	}
	params.AddMetadata("astro_account_id", accountID)
	cust, err := s.sc.V1Customers.Create(ctx, params)
	if err != nil {
		return "", fmt.Errorf("stripe create customer: %w", err)
	}
	return cust.ID, nil
}

// CreateSetupIntent creates a card-only, off-session SetupIntent for the
// customer and returns its client secret. off-session usage runs SCA up front so
// the saved card can be charged later without the customer present.
func (s *Stripe) CreateSetupIntent(ctx context.Context, customerID string) (string, error) {
	si, err := s.sc.V1SetupIntents.Create(ctx, &stripe.SetupIntentCreateParams{
		Customer:           stripe.String(customerID),
		Usage:              stripe.String("off_session"),
		PaymentMethodTypes: []*string{stripe.String("card")},
	})
	if err != nil {
		return "", fmt.Errorf("stripe create setup intent: %w", err)
	}
	return si.ClientSecret, nil
}

// ConfirmSetup verifies the SetupIntent succeeded for this customer, then makes
// its card the sole saved card and the customer's default.
func (s *Stripe) ConfirmSetup(ctx context.Context, customerID, setupIntentID string) (*Card, error) {
	si, err := s.sc.V1SetupIntents.Retrieve(ctx, setupIntentID, nil)
	if err != nil {
		return nil, fmt.Errorf("stripe get setup intent: %w", err)
	}
	if si.Customer == nil || si.Customer.ID != customerID {
		return nil, fmt.Errorf("setup intent %s does not belong to customer %s", setupIntentID, customerID)
	}
	if si.Status != stripe.SetupIntentStatusSucceeded {
		return nil, fmt.Errorf("setup intent %s not succeeded (status=%s)", setupIntentID, si.Status)
	}
	if si.PaymentMethod == nil {
		return nil, fmt.Errorf("setup intent %s has no payment method", setupIntentID)
	}
	pmID := si.PaymentMethod.ID

	// Detach any previously-saved cards so exactly one card is on file.
	if err := s.detachCardsExcept(ctx, customerID, pmID); err != nil {
		return nil, err
	}

	// Make the new card the customer's default for invoices.
	if _, err := s.sc.V1Customers.Update(ctx, customerID, &stripe.CustomerUpdateParams{
		InvoiceSettings: &stripe.CustomerUpdateInvoiceSettingsParams{
			DefaultPaymentMethod: stripe.String(pmID),
		},
	}); err != nil {
		return nil, fmt.Errorf("stripe set default payment method: %w", err)
	}

	return s.cardByID(ctx, pmID)
}

// DefaultCard returns the customer's first saved card, or nil if none.
func (s *Stripe) DefaultCard(ctx context.Context, customerID string) (*Card, error) {
	for pm, err := range s.sc.V1PaymentMethods.List(ctx, &stripe.PaymentMethodListParams{
		Customer: stripe.String(customerID),
		Type:     stripe.String("card"),
	}).All(ctx) {
		if err != nil {
			return nil, fmt.Errorf("stripe list payment methods: %w", err)
		}
		return cardFromPM(pm), nil
	}
	return nil, nil
}

// RemoveCard detaches every saved card for the customer.
func (s *Stripe) RemoveCard(ctx context.Context, customerID string) error {
	return s.detachCardsExcept(ctx, customerID, "")
}

// CollectOpenInvoices charges the customer's default card for every open
// invoice and reports how many were paid. Only `open` invoices are eligible:
// a draft has not been finalized, and paid, void, and uncollectible are all
// settled. Stripe refuses a pay call on any of those.
//
// A declined card is the expected outcome, not a fault, so a failed attempt is
// counted and skipped rather than returned. Stripe emits
// `invoice.payment_failed` for it, and the webhook path records the state.
func (s *Stripe) CollectOpenInvoices(ctx context.Context, customerID string) (paid int, err error) {
	var ids []string
	for inv, err := range s.sc.V1Invoices.List(ctx, &stripe.InvoiceListParams{
		Customer: stripe.String(customerID),
		Status:   stripe.String("open"),
	}).All(ctx) {
		if err != nil {
			return 0, fmt.Errorf("stripe list invoices: %w", err)
		}
		ids = append(ids, inv.ID)
	}
	for _, id := range ids {
		inv, err := s.sc.V1Invoices.Pay(ctx, id, &stripe.InvoicePayParams{})
		if err != nil {
			continue
		}
		if inv.Status == stripe.InvoiceStatusPaid {
			paid++
		}
	}
	return paid, nil
}

// detachCardsExcept detaches all of the customer's card payment methods except
// keepID (pass "" to detach all).
func (s *Stripe) detachCardsExcept(ctx context.Context, customerID, keepID string) error {
	var ids []string
	for pm, err := range s.sc.V1PaymentMethods.List(ctx, &stripe.PaymentMethodListParams{
		Customer: stripe.String(customerID),
		Type:     stripe.String("card"),
	}).All(ctx) {
		if err != nil {
			return fmt.Errorf("stripe list payment methods: %w", err)
		}
		if pm.ID != keepID {
			ids = append(ids, pm.ID)
		}
	}
	for _, id := range ids {
		if _, err := s.sc.V1PaymentMethods.Detach(ctx, id, nil); err != nil {
			return fmt.Errorf("stripe detach payment method %s: %w", id, err)
		}
	}
	return nil
}

func (s *Stripe) cardByID(ctx context.Context, pmID string) (*Card, error) {
	pm, err := s.sc.V1PaymentMethods.Retrieve(ctx, pmID, nil)
	if err != nil {
		return nil, fmt.Errorf("stripe get payment method: %w", err)
	}
	return cardFromPM(pm), nil
}

func cardFromPM(pm *stripe.PaymentMethod) *Card {
	c := &Card{ID: pm.ID}
	if pm.Card != nil {
		c.Brand = string(pm.Card.Brand)
		c.Last4 = pm.Card.Last4
		c.ExpMonth = pm.Card.ExpMonth
		c.ExpYear = pm.Card.ExpYear
	}
	return c
}
