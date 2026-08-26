package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
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

// billingReconcileQueue enqueues workload suspend/resume after a card change
// flips an account's gating status. Satisfied by *riverqueue.Queue.
type billingReconcileQueue interface {
	InsertBillingSuspend(ctx context.Context, accountID string) error
	InsertBillingResume(ctx context.Context, accountID string) error
	InsertBillingCollect(ctx context.Context, accountID, stripeCustomerID string) error
	InsertBillingGatewayBudget(ctx context.Context, accountID string) error
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
		log.Warn("payment methods: load Stripe customer ID failed", "error", err, "account_id", acct.ID)
		return "", false
	}
	if customerID != "" {
		return customerID, true
	}
	// Use the account owner's WorkOS-verified email (persisted in our DB), not the
	// requesting user or the editable profile email. Best-effort — an empty email
	// is acceptable if none is mirrored yet.
	ownerEmail, emailErr := accountStore.GetOwnerEmail(acct.ID)
	if emailErr != nil {
		log.Warn("payment methods: load account owner email for Stripe customer failed", "error", emailErr, "account_id", acct.ID)
	}
	customerID, err = paymentProvider.CreateCustomer(c.Request.Context(), acct.ID, acct.Name, ownerEmail)
	if err != nil {
		log.Error("payment methods: create Stripe customer failed", "error", err, "account_id", acct.ID)
		return "", false
	}
	if err := accountStore.SetStripeCustomerID(acct.ID, customerID); err != nil {
		log.Error("payment methods: store Stripe customer ID failed", "error", err, "account_id", acct.ID)
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
			log.Error("payment methods: create setup intent failed", "error", err, "account_id", acct.ID)
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
func ConfirmPaymentMethod(log *logger.Logger, accountStore *account.AccountStore, paymentProvider payment.Provider, billingProvider billing.BillingProvider, billingBackend string, billingStatus *billing.StatusStore, queue billingReconcileQueue) gin.HandlerFunc {
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
			log.Error("payment methods: confirm setup intent failed", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to save payment method"})
			return
		}

		// Best-effort: link the Stripe customer to the hosted billing provider so
		// finalized invoices charge the saved card. A failure here doesn't fail the
		// save — the card is already vaulted and the link can be retried.
		linkStripeToBilling(c, log, accountStore, billingProvider, billingBackend, acct, customerID)

		applyCardSignal(c, log, billingStatus, queue, acct.ID, customerID, billing.SignalCardAdded)

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
			log.Warn("payment methods: load Stripe customer ID failed", "error", err, "account_id", acct.ID)
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
			log.Error("payment methods: load payment method failed", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load payment method"})
			return
		}
		c.JSON(http.StatusOK, PaymentMethodResponse{Available: true, Card: card})
	}
}

// DeletePaymentMethod handles DELETE /api/v1/accounts/:account/billing/payment-method.
func DeletePaymentMethod(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, paymentProvider payment.Provider, billingProvider billing.BillingProvider, billingBackend string, billingStatus *billing.StatusStore, queue billingReconcileQueue) gin.HandlerFunc {
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
		// Compute is metered on closed five-minute windows, so the balance below
		// cannot yet see what a running agent has already spent.
		if deployStore != nil {
			running, err := deployStore.CountRunningByAccount(c.Request.Context(), acct.ID)
			if err != nil {
				log.Error("payment methods: count running deployments failed", "error", err, "account_id", acct.ID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove payment method"})
				return
			}
			if running > 0 {
				log.Info("payment methods: removal refused, running deployments", "account_id", acct.ID, "running", running)
				c.JSON(http.StatusConflict, gin.H{"error": "Pause or delete this account's running agents before removing your payment method. They are still accruing charges."})
				return
			}
		}
		// Removing the card is the other way out of a bill. Deleting the account
		// is refused for the same reason, and this door is cheaper to walk
		// through: the spend stops but the accrued draft is never charged.
		if billingProvider != nil {
			billingCustomerID, err := accountStore.GetBillingCustomerID(acct.ID, billingBackend)
			if err != nil {
				log.Error("payment methods: load billing customer id for removal failed", "error", err, "account_id", acct.ID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove payment method"})
				return
			}
			if billingCustomerID != "" {
				owed, err := outstandingBalance(c.Request.Context(), billingProvider, billingStatus, acct.ID, billingCustomerID)
				if err != nil {
					log.Error("payment methods: read balance for removal failed", "error", err, "account_id", acct.ID, "billing_customer_id", billingCustomerID)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove payment method"})
					return
				}
				if owed {
					log.Info("payment methods: removal refused, outstanding balance", "account_id", acct.ID, "billing_customer_id", billingCustomerID)
					c.JSON(http.StatusConflict, gin.H{"error": "this account has an outstanding balance; settle it before removing your payment method. To change cards, save the new one instead: it replaces the old card without leaving the account unpayable"})
					return
				}
			}
		}

		if err := paymentProvider.RemoveCard(c.Request.Context(), customerID); err != nil {
			log.Error("payment methods: remove payment method failed", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to remove payment method"})
			return
		}

		applyCardSignal(c, log, billingStatus, queue, acct.ID, customerID, billing.SignalCardRemoved)

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// applyCardSignal records the card fact and reconciles workloads to the
// resulting status. Adding a card lifts a credits-exhausted suspension (the
// account becomes pay-as-you-go); removing the last one puts it back. No-op
// without a status store (OSS / unconfigured). Best-effort: the card is already
// saved with Stripe, so a failure here must not fail the request — the next
// webhook or the dunning sweep recomputes.
func applyCardSignal(c *gin.Context, log *logger.Logger, status *billing.StatusStore, queue billingReconcileQueue, accountID, stripeCustomerID string, sig billing.Signal) {
	if status == nil {
		return
	}
	// The card is already saved, so everything from here has to outlive the
	// request: a client that hangs up must not leave the status unwritten and
	// the gateway ceiling stale.
	ctx, cancel := reconcileContext(c)
	defer cancel()
	newStatus, changed, err := billing.ApplySignal(ctx, status, accountID, sig, time.Now())
	if err != nil {
		log.Error("payment methods: apply card billing signal failed", "error", err, "account_id", accountID, "signal", string(sig))
		return
	}
	if changed {
		log.Info("payment methods: billing status changed", "source", "card", "account_id", accountID, "status", string(newStatus), "signal", string(sig))
	}
	if queue == nil {
		return
	}
	// A card is the fact the gateway ceiling is derived from, so it moves on the
	// same signal that moves the status.
	if err := queue.InsertBillingGatewayBudget(ctx, accountID); err != nil {
		log.Error("payment methods: enqueue gateway budget after card change failed", "error", err, "account_id", accountID)
	}
	// Reconcile on every card change, not only on a transition, so a dropped
	// enqueue is re-attempted by the next one. Suspend/resume are idempotent.
	var enqueueErr error
	switch newStatus {
	case billing.StatusSuspended:
		enqueueErr = queue.InsertBillingSuspend(ctx, accountID)
	case billing.StatusActive:
		enqueueErr = queue.InsertBillingResume(ctx, accountID)
	}
	if enqueueErr != nil {
		log.Error("payment methods: enqueue workload reconcile after card change failed", "error", enqueueErr, "account_id", accountID)
	}
	if !collectAfterCard(newStatus, sig) || stripeCustomerID == "" {
		return
	}
	if err := queue.InsertBillingCollect(ctx, accountID, stripeCustomerID); err != nil {
		log.Error("payment methods: enqueue invoice collection after card change failed", "error", err, "account_id", accountID)
	}
}

// collectAfterCard reports whether a new card should be charged for what the
// account already owes.
//
// A saved card does not clear the debt that suspended the account: dunning
// clears on payment, and only a payment clears it. The account stays gated
// until the provider's own retry schedule comes around, which can be days out
// or already exhausted, so the card that fixes the problem appears to do
// nothing. Charging now closes that gap.
//
// Active means no collection flag is raised and there is nothing owed to chase.
func collectAfterCard(status billing.Status, sig billing.Signal) bool {
	return sig == billing.SignalCardAdded && status != billing.StatusActive
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
		log.Error("payment methods: link Stripe customer to billing provider failed", "error", err, "account_id", acct.ID)
	}
}
