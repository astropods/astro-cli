package handlers

import (
	"context"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/payment"
	"github.com/gin-gonic/gin"
)

// stripeLinker is the optional capability, satisfied by the metronome billing
// provider, to link a saved Stripe customer so the hosted provider charges the
// vaulted card. Detected via interface assertion so the core billing seam stays
// metering-only.
type stripeLinker interface {
	LinkStripeCustomer(ctx context.Context, metronomeCustomerID, stripeCustomerID string) error
}

// SetupIntentResponse carries the client secret + publishable key the frontend
// needs to confirm a card with Stripe.js.
type SetupIntentResponse struct {
	ClientSecret   string `json:"client_secret"`
	PublishableKey string `json:"publishable_key"`
}

// PaymentMethodResponse wraps the saved card for the client. Available is false
// when payments aren't configured (no Stripe); Card is nil when none is on file.
type PaymentMethodResponse struct {
	Available bool          `json:"available"`
	Card      *payment.Card `json:"card,omitempty"`
}

// resolveStripeCustomer resolves the account's Stripe customer, lazily creating
// one on first access. Returns ("", false) when payments aren't configured.
func resolveStripeCustomer(c *gin.Context, log *logger.Logger, accountStore *account.AccountStore, paymentProvider payment.Provider, acct *account.Account) (string, bool) {
	if paymentProvider == nil {
		return "", false
	}
	customerID, err := accountStore.GetStripeCustomerID(acct.ID)
	if err != nil {
		log.Warn("Failed to load Stripe customer ID", "error", err, "account_id", acct.ID)
		return "", false
	}
	if customerID != "" {
		return customerID, true
	}
	customerID, err = paymentProvider.CreateCustomer(c.Request.Context(), acct.ID, acct.Name, acct.Email)
	if err != nil {
		log.Error("Failed to create Stripe customer", "error", err, "account_id", acct.ID)
		return "", false
	}
	if err := accountStore.SetStripeCustomerID(acct.ID, customerID); err != nil {
		log.Error("Failed to store Stripe customer ID", "error", err, "account_id", acct.ID)
	}
	return customerID, true
}

// CreateSetupIntent handles POST /api/v1/accounts/:account/billing/setup-intent.
// It ensures a Stripe customer exists and returns a SetupIntent client secret
// for the frontend to confirm a card against.
func CreateSetupIntent(log *logger.Logger, accountStore *account.AccountStore, paymentProvider payment.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		customerID, ok := resolveStripeCustomer(c, log, accountStore, paymentProvider, acct)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "payments not available"})
			return
		}
		clientSecret, err := paymentProvider.CreateSetupIntent(c.Request.Context(), customerID)
		if err != nil {
			log.Error("Failed to create setup intent", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to create setup intent"})
			return
		}
		c.JSON(http.StatusOK, SetupIntentResponse{
			ClientSecret:   clientSecret,
			PublishableKey: paymentProvider.PublishableKey(),
		})
	}
}

// ConfirmPaymentMethod handles POST /api/v1/accounts/:account/billing/payment-method.
// The frontend calls it after Stripe.js confirms the SetupIntent; the server
// re-reads the intent from Stripe (authoritative), saves the card as default,
// and links the Stripe customer to the hosted billing provider so it can charge.
func ConfirmPaymentMethod(log *logger.Logger, accountStore *account.AccountStore, paymentProvider payment.Provider, billingProvider billing.BillingProvider, billingBackend string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			SetupIntentID string `json:"setup_intent_id"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.SetupIntentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "setup_intent_id required"})
			return
		}
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		customerID, ok := resolveStripeCustomer(c, log, accountStore, paymentProvider, acct)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "payments not available"})
			return
		}

		card, err := paymentProvider.ConfirmSetup(c.Request.Context(), customerID, body.SetupIntentID)
		if err != nil {
			log.Error("Failed to confirm setup intent", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to save payment method"})
			return
		}

		// Best-effort: link the Stripe customer to the hosted billing provider so
		// finalized invoices charge the saved card. A failure here doesn't fail the
		// save — the card is already vaulted and the link can be retried.
		linkStripeToBilling(c, log, accountStore, billingProvider, billingBackend, acct, customerID)

		c.JSON(http.StatusOK, PaymentMethodResponse{Available: true, Card: card})
	}
}

// GetPaymentMethod handles GET /api/v1/accounts/:account/billing/payment-method.
func GetPaymentMethod(log *logger.Logger, accountStore *account.AccountStore, paymentProvider payment.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		if paymentProvider == nil {
			c.JSON(http.StatusOK, PaymentMethodResponse{Available: false})
			return
		}
		customerID, err := accountStore.GetStripeCustomerID(acct.ID)
		if err != nil {
			log.Warn("Failed to load Stripe customer ID", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusOK, PaymentMethodResponse{Available: false})
			return
		}
		if customerID == "" {
			// No customer yet — payments available, but no card on file.
			c.JSON(http.StatusOK, PaymentMethodResponse{Available: true})
			return
		}
		card, err := paymentProvider.DefaultCard(c.Request.Context(), customerID)
		if err != nil {
			log.Error("Failed to load payment method", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load payment method"})
			return
		}
		c.JSON(http.StatusOK, PaymentMethodResponse{Available: true, Card: card})
	}
}

// DeletePaymentMethod handles DELETE /api/v1/accounts/:account/billing/payment-method.
func DeletePaymentMethod(log *logger.Logger, accountStore *account.AccountStore, paymentProvider payment.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		if paymentProvider == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "payments not available"})
			return
		}
		customerID, err := accountStore.GetStripeCustomerID(acct.ID)
		if err != nil || customerID == "" {
			// Nothing to remove.
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
			return
		}
		if err := paymentProvider.RemoveCard(c.Request.Context(), customerID); err != nil {
			log.Error("Failed to remove payment method", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to remove payment method"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// linkStripeToBilling links the Stripe customer to the hosted billing provider
// (Metronome) so it charges the vaulted card. No-op when the backend isn't
// metronome or doesn't support linking. Errors are logged, not returned.
func linkStripeToBilling(c *gin.Context, log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string, acct *account.Account, stripeCustomerID string) {
	linker, ok := billingProvider.(stripeLinker)
	if !ok {
		return
	}
	// Resolve (lazily creating) the hosted billing customer to link against.
	metronomeCustomerID, ok := resolveBillingCustomer(c, log, accountStore, billingProvider, billingBackend, acct)
	if !ok || metronomeCustomerID == "" {
		return
	}
	if err := linker.LinkStripeCustomer(c.Request.Context(), metronomeCustomerID, stripeCustomerID); err != nil {
		log.Error("Failed to link Stripe customer to billing provider", "error", err, "account_id", acct.ID)
	}
}
