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

// Decision is the gate's answer. Reason is the account's gating reason code,
// carried so callers can tell the user which fix applies; it is empty unless
// Blocked is true.
type Decision struct {
	Blocked bool
	Reason  string
}

// Check reports whether the account is billing-suspended, and why. In observe
// mode it logs the would-be block and allows. Fails open on a read error: never
// block a customer because the status lookup failed.
func (e *Entitlements) Check(ctx context.Context, accountID string) Decision {
	if e.status == nil || accountID == "" {
		return Decision{}
	}
	st, reason, err := e.status.Get(ctx, accountID)
	if err != nil {
		if e.log != nil {
			e.log.Warn("billing gate: status read failed, allowing", "account_id", accountID, "error", err)
		}
		return Decision{}
	}
	if st != billing.StatusSuspended {
		return Decision{}
	}
	if !e.enforce {
		if e.log != nil {
			e.log.Info("billing gate (observe): would block", "account_id", accountID, "reason", reason)
		}
		return Decision{}
	}
	return Decision{Blocked: true, Reason: reason}
}

// Blocked keeps the boolean form for callers that only branch on it.
func (e *Entitlements) Blocked(ctx context.Context, accountID string) bool {
	return e.Check(ctx, accountID).Blocked
}

// Wrap gates a handler: a suspended account (enforce mode) gets a 402; otherwise
// the handler runs.
func (e *Entitlements) Wrap(handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := GetAccountFromContext(c)
		if !ok {
			// The gate has nothing to check, so the route is open. That is a
			// wiring error rather than a state, and it is invisible otherwise:
			// the route looks gated at the call site and gates nobody.
			if e.status != nil && e.log != nil {
				e.log.Warn("billing gate: no account in context, allowing", "path", c.FullPath())
			}
			handler(c)
			return
		}
		if d := e.Check(c.Request.Context(), acct.ID); d.Blocked {
			c.AbortWithStatusJSON(http.StatusPaymentRequired, PaymentRequiredResponse(d.Reason))
			return
		}
		handler(c)
	}
}

// Billing actions. The gate names the fix; each surface writes its own words,
// so a terminal and a banner can phrase it differently without disagreeing on
// what the user has to do.
const (
	ActionAddCard        = "add_card"        // free tier ran dry, no card on file
	ActionUpdateCard     = "update_card"     // a card exists and collection failed
	ActionContactSupport = "contact_support" // only we can lift it
	ActionViewBilling    = "view_billing"    // reason unknown to this build
)

// BillingAction maps a gating reason to the one thing that resolves it. Telling
// an account with no card to update one is the copy bug this exists to prevent.
func BillingAction(reason string) string {
	switch reason {
	case billing.ReasonCreditsExhausted:
		return ActionAddCard
	case billing.ReasonDunning, billing.ReasonPaymentFailed, billing.ReasonUncollectible:
		return ActionUpdateCard
	case billing.ReasonBalanceAlert:
		return ActionContactSupport
	default:
		return ActionViewBilling
	}
}

// actionDetails is the fallback sentence for a client that renders the body as
// it stands, which the CLI does today. Clients that branch on action supply
// their own wording.
var actionDetails = map[string]string{
	ActionAddCard:        "This account's free credits are used up. Add a payment method to continue.",
	ActionUpdateCard:     "A payment for this account could not be collected. Update the payment method to continue.",
	ActionContactSupport: "This account reached its spend limit. Contact support to raise it.",
	ActionViewBilling:    "This account is suspended for a billing issue. Open billing settings to resolve it.",
}

// PaymentRequiredResponse is the 402 body for a billing-suspended account. It
// carries the reason code and the resolving action so a client does not have to
// re-derive either from raw billing flags.
func PaymentRequiredResponse(reason string) gin.H {
	action := BillingAction(reason)
	return gin.H{
		"error":   "Billing suspended",
		"code":    "BILLING_SUSPENDED",
		"reason":  reason,
		"action":  action,
		"details": actionDetails[action],
	}
}
