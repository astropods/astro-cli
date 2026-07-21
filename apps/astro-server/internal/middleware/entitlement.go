package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// Entitlements is the consumption gate: a binary paid/not-paid check. It reads
// the cached account_billing_status (written off the request path by the
// Metronome webhook + dunning sweep) and blocks suspended accounts. There is no
// per-feature notion — an account is either in good standing or suspended.
//
// It never calls the billing provider on the request path and never reads a
// balance (Metronome owns that). Pass-through when no status store is wired
// (OSS/noop) or in observe mode.
type Entitlements struct {
	status  *billing.StatusStore
	enforce bool
	log     *logger.Logger
}

// NewEntitlements builds the gate. status is nil for backends without gating
// (OSS/noop) → pass-through. enforce=false is observe mode: decisions are logged
// but never blocked. log may be nil.
func NewEntitlements(status *billing.StatusStore, enforce bool, log *logger.Logger) *Entitlements {
	return &Entitlements{status: status, enforce: enforce, log: log}
}

// Blocked reports whether the account is billing-suspended and should be
// blocked. In observe mode it logs the would-be block and returns false. Fails
// open on any read error (never block because status lookup failed).
func (e *Entitlements) Blocked(ctx context.Context, accountID string) bool {
	if e.status == nil || accountID == "" {
		return false
	}
	st, _, err := e.status.Get(ctx, accountID)
	if err != nil {
		if e.log != nil {
			e.log.Warn("billing gate: status read failed, allowing", "account_id", accountID, "error", err)
		}
		return false
	}
	if st != billing.StatusSuspended {
		return false
	}
	if !e.enforce {
		if e.log != nil {
			e.log.Info("billing gate (observe): would block", "account_id", accountID)
		}
		return false
	}
	return true
}

// Wrap gates a handler: a suspended account (enforce mode) gets a 402; otherwise
// the handler runs.
func (e *Entitlements) Wrap(handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if acct, ok := GetAccountFromContext(c); ok && e.Blocked(c.Request.Context(), acct.ID) {
			c.AbortWithStatusJSON(http.StatusPaymentRequired, PaymentRequiredResponse())
			return
		}
		handler(c)
	}
}

// PaymentRequiredResponse is the 402 body for a billing-suspended account.
func PaymentRequiredResponse() gin.H {
	return gin.H{
		"error":   "Billing suspended",
		"code":    "BILLING_SUSPENDED",
		"details": "Your account is suspended due to a billing issue. Update your payment method to continue.",
	}
}
