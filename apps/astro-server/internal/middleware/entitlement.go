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
	// PayLink is set when the block clears by authenticating a charge rather
	// than by changing a card.
	PayLink string
}

// Check reports whether the account is billing-suspended, and why. In observe
// mode it logs the would-be block and allows. Fails open on a read error: never
// block a customer because the status lookup failed.
func (e *Entitlements) Check(ctx context.Context, accountID string) Decision {
	if e.status == nil || accountID == "" {
		return Decision{}
	}
	// Record rather than Get: the block has to name the fix, and for an
	// unauthenticated charge the fix is a link this row holds.
	rec, err := e.status.Record(ctx, accountID)
	if err != nil {
		if e.log != nil {
			e.log.Warn("billing gate: status read failed, allowing", "account_id", accountID, "error", err)
		}
		return Decision{}
	}
	if rec.Status != billing.StatusSuspended {
		return Decision{}
	}
	if !e.enforce {
		if e.log != nil {
			e.log.Info("billing gate (observe): would block", "account_id", accountID, "reason", rec.Reason)
		}
		return Decision{}
	}
	return Decision{Blocked: true, Reason: rec.Reason, PayLink: rec.PayLink}
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
			c.AbortWithStatusJSON(http.StatusPaymentRequired, PaymentRequiredResponse(d.Reason, d.PayLink))
			return
		}
		handler(c)
	}
}

// Billing actions. The gate names the fix; each surface writes its own words,
// so a terminal and a banner can phrase it differently without disagreeing on
// what the user has to do.
const (
	ActionAddCard         = "add_card"          // free tier ran dry, no card on file
	ActionUpdateCard      = "update_card"       // a card exists and collection failed
	ActionContactSupport  = "contact_support"   // nothing the account holder can do
	ActionCompletePayment = "complete_payment"  // the bank wants the customer to authenticate
	ActionRaiseUsageLimit = "raise_usage_limit" // the account's own cap stopped it
)

// BillingAction maps a gating reason to the one thing that resolves it. Only two
// outcomes are self-serve, and they differ in what the client has to render: an
// empty card form, or replacing a card already on file. Telling an account with
// a card to add one is the copy bug this exists to prevent.
//
// Everything else is contact_support, which is the absence of a fix rather than
// a third one. An unrecognised reason lands there too: a build that cannot name
// the problem must not send the owner to change a card that may be fine.
// A pay link outranks update_card on the collection reasons. When the bank asked
// for authentication the card is fine, and telling the customer to replace it
// sends them to fix something that is not broken while the charge waits.
func BillingAction(reason string, hasPayLink bool) string {
	switch reason {
	case billing.ReasonCreditsExhausted:
		return ActionAddCard
	case billing.ReasonDunning, billing.ReasonPaymentFailed:
		if hasPayLink {
			return ActionCompletePayment
		}
		return ActionUpdateCard
	case billing.ReasonUsageLimit:
		return ActionRaiseUsageLimit
	case billing.ReasonUncollectible:
		// A write-off is terminal: only a void or an operator lifts it, and
		// paying the old link would leave the account suspended anyway. Offering
		// the link here is a button that takes money and changes nothing.
		return ActionUpdateCard
	default:
		return ActionContactSupport
	}
}

// actionDetails is the fallback sentence for a client that renders the body as
// it stands, which the CLI does today. Clients that branch on action supply
// their own wording.
var actionDetails = map[string]string{
	ActionAddCard:         "This account's free credits are used up. Add a payment method to continue.",
	ActionUpdateCard:      "A payment for this account could not be collected. Update the payment method to continue.",
	ActionContactSupport:  "This account is suspended for a billing issue only support can resolve. Contact support to continue.",
	ActionCompletePayment: "A payment for this account needs to be confirmed with your bank. Complete it to continue.",
	ActionRaiseUsageLimit: "This account reached the usage limit it set. Raise or remove the limit to continue.",
}

// PaymentRequiredResponse is the 402 body for a billing-suspended account. It
// carries the reason code, the resolving action, and for the one action the
// customer cannot complete in the app, the hosted page to finish it on. Pass an
// empty payLink when there is none.
func PaymentRequiredResponse(reason, payLink string) gin.H {
	action := BillingAction(reason, payLink != "")
	body := gin.H{
		"error":   "Billing suspended",
		"code":    "BILLING_SUSPENDED",
		"reason":  reason,
		"action":  action,
		"details": actionDetails[action],
	}
	if action == ActionCompletePayment {
		body["pay_link"] = payLink
	}
	return body
}
