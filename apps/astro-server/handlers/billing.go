package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// BillingDataResponse wraps provider billing data for the client. When the
// hosted backend or the account's customer isn't available, Available is false
// and Data is empty — the client renders a "not available" state rather than
// treating it as an error.
type BillingDataResponse struct {
	Available bool `json:"available"`
	Data      any  `json:"data,omitempty"`
}

// BillingStatusResponse is the account's gating state, for the client's banner.
// It reports the cached status plus the two facts that explain it, so the UI can
// distinguish "free credits spent, add a card" from "your card was declined"
// without interpreting the reason string.
type BillingStatusResponse struct {
	Status           string `json:"status"` // active | past_due | suspended
	Reason           string `json:"reason,omitempty"`
	CreditsExhausted bool   `json:"credits_exhausted"`
	HasPaymentMethod bool   `json:"has_payment_method"`
	// Enforced is BILLING_GATE_ENFORCE: whether this status is acted on.
	Enforced bool `json:"enforced"`
	// WorkloadsSuspended is whether billing has already stopped this account's
	// deployments. Outlives Enforced, since turning enforcement off does not
	// restart what it stopped.
	WorkloadsSuspended bool `json:"workloads_suspended"`
	// Gated is whether this status is worth surfacing: enforcement is on, or it
	// already stopped something. The server owns the rule so a client does not
	// combine Enforced and WorkloadsSuspended itself and drift from it.
	Gated bool `json:"gated"`
	// Action is the one thing that resolves the gate, matching the 402 body's
	// action so a banner and a refused request never disagree on the fix.
	Action string `json:"action,omitempty"`
}

// GetBillingStatus handles GET /api/v1/accounts/:account/billing/status. It
// reads the cached status only — no provider call — so it is cheap enough to
// poll on every page. Without a status store (OSS) every account is active.
func GetBillingStatus(log *logger.Logger, billingStatus *billing.StatusStore, deployments *deploymentstore.Store, enforced bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		if billingStatus == nil {
			c.JSON(http.StatusOK, BillingStatusResponse{Status: string(billing.StatusActive), Enforced: enforced})
			return
		}
		rec, err := billingStatus.Record(c.Request.Context(), acct.ID)
		if err != nil {
			log.Error("Failed to load billing status", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load billing status"})
			return
		}
		// Only a non-active status can raise a banner, so an active account
		// skips the read. Best-effort: a failure must not hide a status the
		// user can act on.
		stopped := false
		if rec.Status != billing.StatusActive && deployments != nil {
			var derr error
			if stopped, derr = deployments.HasBillingSuspended(c.Request.Context(), acct.ID); derr != nil {
				log.Warn("Failed to check suspended workloads", "error", derr, "account_id", acct.ID)
			}
		}
		gated := rec.Status != billing.StatusActive && (enforced || stopped)
		resp := BillingStatusResponse{
			Status:             string(rec.Status),
			Reason:             rec.Reason,
			CreditsExhausted:   rec.CreditsExhausted,
			HasPaymentMethod:   rec.HasPaymentMethod,
			Enforced:           enforced,
			WorkloadsSuspended: stopped,
			Gated:              gated,
		}
		if gated {
			resp.Action = middleware.BillingAction(rec.Reason)
		}
		c.JSON(http.StatusOK, resp)
	}
}

// resolveBillingCustomer resolves the account's billing customer, lazily
// creating one on first access. Returns ("", false) when billing is not
// available for this environment (OSS/noop) or the customer can't be
// created/resolved — the caller should respond with Available:false.
func resolveBillingCustomer(c *gin.Context, log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string, acct *account.Account) (string, bool) {
	if billingProvider == nil || billingBackend != "metronome" {
		return "", false
	}

	customerID, err := accountStore.GetBillingCustomerID(acct.ID, billingBackend)
	if err != nil {
		log.Warn("Failed to load billing customer ID", "error", err, "account_id", acct.ID)
		return "", false
	}
	if customerID != "" {
		return customerID, true
	}

	// Lazily provision the customer on first billing access.
	bifrostCustomerID, _ := accountStore.GetBifrostCustomerID(acct.ID)
	ownerEmail, _ := accountStore.GetOwnerEmail(acct.ID)
	customerID, err = billingProvider.CreateCustomer(c.Request.Context(), billing.Account{
		ID:                acct.ID,
		Name:              acct.Name,
		Type:              acct.Type,
		OwnerEmail:        ownerEmail,
		BifrostCustomerID: bifrostCustomerID,
	})
	if err != nil {
		log.Error("Failed to create billing customer", "error", err, "account_id", acct.ID)
		return "", false
	}
	if err := accountStore.SetBillingCustomerID(acct.ID, billingBackend, customerID); err != nil {
		log.Error("Failed to store billing customer ID", "error", err, "account_id", acct.ID)
	}
	return customerID, true
}

// billingData is the shared body for the read endpoints: it resolves the
// customer and returns whatever the provider hands back, verbatim.
func billingData(
	log *logger.Logger,
	accountStore *account.AccountStore,
	billingProvider billing.BillingProvider,
	billingBackend string,
	label string,
	fetch func(ctx context.Context, customerID string) (any, error),
) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		customerID, ok := resolveBillingCustomer(c, log, accountStore, billingProvider, billingBackend, acct)
		if !ok {
			c.JSON(http.StatusOK, BillingDataResponse{Available: false})
			return
		}

		data, err := fetch(c.Request.Context(), customerID)
		if err != nil {
			if errors.Is(err, billing.ErrBillingUnavailable) {
				c.JSON(http.StatusOK, BillingDataResponse{Available: false})
				return
			}
			log.Error("Failed to load billing "+label, "error", err, "account_id", acct.ID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load billing " + label})
			return
		}

		c.JSON(http.StatusOK, BillingDataResponse{Available: true, Data: data})
	}
}

// utcMidnight truncates t to UTC midnight (Metronome requires window bounds to
// be UTC midnight).
func utcMidnight(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// GetBillingUsage handles GET /api/v1/accounts/:account/billing/usage. It
// returns metered usage over [from, to) (defaults to the current calendar
// month), aggregated per day, exactly as the provider reports it.
func GetBillingUsage(log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string) gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now().UTC()
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		// Metronome requires the window bounds to be UTC midnight; default the
		// end to the start of tomorrow so today's partial data is included.
		to := utcMidnight(now).AddDate(0, 0, 1)
		if v := c.Query("from"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				from = utcMidnight(t)
			}
		}
		if v := c.Query("to"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				to = utcMidnight(t)
			}
		}
		if !to.After(from) {
			to = from.AddDate(0, 0, 1)
		}

		billingData(log, accountStore, billingProvider, billingBackend, "usage",
			func(ctx context.Context, customerID string) (any, error) {
				return billingProvider.UsageData(ctx, customerID, from, to)
			})(c)
	}
}

// GetBillingInvoices handles GET /api/v1/accounts/:account/billing/invoices. It
// returns the customer's invoices (with line items) for the client to render.
func GetBillingInvoices(log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string) gin.HandlerFunc {
	return billingData(log, accountStore, billingProvider, billingBackend, "invoices",
		func(ctx context.Context, customerID string) (any, error) {
			return billingProvider.Invoices(ctx, customerID)
		})
}

// GetBillingInvoicePDF handles
// GET /api/v1/accounts/:account/billing/invoices/:invoiceId/pdf. It streams the
// invoice PDF from the provider so the client can render it (e.g. in a modal).
func GetBillingInvoicePDF(log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string) gin.HandlerFunc {
	return func(c *gin.Context) {
		invoiceID := c.Param("invoiceId")
		if invoiceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invoice id required"})
			return
		}

		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		customerID, ok := resolveBillingCustomer(c, log, accountStore, billingProvider, billingBackend, acct)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "billing not available"})
			return
		}

		rc, err := billingProvider.InvoicePDF(c.Request.Context(), customerID, invoiceID)
		if err != nil {
			if errors.Is(err, billing.ErrBillingUnavailable) {
				c.JSON(http.StatusNotFound, gin.H{"error": "billing not available"})
				return
			}
			if errors.Is(err, billing.ErrInvoiceNotAvailable) {
				c.JSON(http.StatusNotFound, gin.H{"error": "invoice is not finalized yet"})
				return
			}
			log.Error("Failed to load invoice PDF", "error", err, "account_id", acct.ID, "invoice_id", invoiceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load invoice PDF"})
			return
		}
		defer rc.Close() //nolint:errcheck

		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", "inline; filename=\"invoice.pdf\"")
		if _, err := io.Copy(c.Writer, rc); err != nil {
			log.Warn("Failed to stream invoice PDF", "error", err, "account_id", acct.ID, "invoice_id", invoiceID)
		}
	}
}

// GetBillingBalances handles GET /api/v1/accounts/:account/billing/balances.
// BillingSpendResponse is the customer-facing view of what an account is running
// up and the controls it set on itself. Spend and thresholds ship together
// because a threshold is meaningless without the number it is measured against.
type BillingSpendResponse struct {
	Currency string `json:"currency,omitempty"`

	CurrentSpend     float64   `json:"current_spend"`
	HasCurrentSpend  bool      `json:"has_current_spend"`
	CurrentPeriodEnd time.Time `json:"current_period_end,omitempty"`

	// UsageSpend is what the thresholds below are measured against: the period's
	// usage before credit drawdown. It differs from CurrentSpend whenever credit
	// covers part of the bill, and an account on signup credit reads zero for
	// CurrentSpend while UsageSpend climbs toward its own warning.
	UsageSpend    float64 `json:"usage_spend"`
	HasUsageSpend bool    `json:"has_usage_spend"`

	CreditRemaining float64 `json:"credit_remaining"`
	HasCredit       bool    `json:"has_credit"`

	// Absent when the customer set none. Nil rather than zero, which is a
	// threshold a customer could legitimately set.
	Warning *SpendThresholdResponse `json:"warning,omitempty"`
	Limit   *SpendThresholdResponse `json:"limit,omitempty"`
}

// SpendThresholdResponse is one customer-set number and whether it is crossed.
type SpendThresholdResponse struct {
	Amount  float64 `json:"amount"`
	InAlarm bool    `json:"in_alarm"`
}

// GetBillingSpend handles GET /api/v1/accounts/:account/billing/spend. The
// thresholds live in the billing provider and nowhere else, so this reads
// through rather than from a mirror that could disagree with what fires.
func GetBillingSpend(log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string) gin.HandlerFunc {
	return billingData(log, accountStore, billingProvider, billingBackend, "spend",
		func(ctx context.Context, customerID string) (any, error) {
			reporter, ok := billingProvider.(billing.SpendReporter)
			if !ok {
				return nil, billing.ErrBillingUnavailable
			}
			spend, err := reporter.CustomerSpend(ctx, customerID)
			if err != nil {
				return nil, err
			}
			resp := BillingSpendResponse{
				Currency:         spend.Currency,
				CurrentSpend:     spend.CurrentSpend,
				HasCurrentSpend:  spend.HasCurrentSpend,
				CurrentPeriodEnd: spend.CurrentPeriodEnd,
				UsageSpend:       spend.UsageSpend,
				HasUsageSpend:    spend.HasUsageSpend,
				CreditRemaining:  spend.CreditRemaining,
				HasCredit:        spend.HasCredit,
			}
			// Best-effort: a threshold read that fails must not hide the spend,
			// which is the half a customer needs to set one in the first place.
			if reader, ok := billingProvider.(billing.SpendThresholdReader); ok {
				th, terr := reader.CustomerSpendThresholds(ctx, customerID)
				if terr != nil {
					log.Warn("Failed to load spend thresholds", "error", terr, "customer_id", customerID)
				} else {
					if th.HasWarning {
						resp.Warning = &SpendThresholdResponse{Amount: th.Warning.Amount, InAlarm: th.Warning.InAlarm}
					}
					if th.HasLimit {
						resp.Limit = &SpendThresholdResponse{Amount: th.Limit.Amount, InAlarm: th.Limit.InAlarm}
					}
				}
			}
			return resp, nil
		})
}

// writeSpendThresholds applies both controls in a fixed order and reports which
// ones took effect. The limit goes first because it is the protective one: a
// failure on the warning then leaves the cap intact rather than the reverse. A
// map would make which one survives a coin flip.
func writeSpendThresholds(
	ctx context.Context,
	writer billing.SpendThresholdWriter,
	customerID string,
	warning, limit *float64,
) (applied []string, failed billing.SpendThresholdKind, err error) {
	writes := []struct {
		kind   billing.SpendThresholdKind
		amount *float64
	}{
		{billing.SpendThresholdLimit, limit},
		{billing.SpendThresholdWarning, warning},
	}
	applied = make([]string, 0, len(writes))
	for _, w := range writes {
		if w.amount == nil {
			err = writer.ClearCustomerSpendThreshold(ctx, customerID, w.kind)
		} else {
			err = writer.SetCustomerSpendThreshold(ctx, customerID, w.kind, *w.amount)
		}
		if err != nil {
			return applied, w.kind, err
		}
		applied = append(applied, string(w.kind))
	}
	return applied, "", nil
}

// SetSpendThresholdsRequest replaces both of an account's controls. A null
// clears that one. PUT rather than PATCH: partial updates would make an omitted
// field ambiguous between "leave it" and "remove it", and removing a spend limit
// by accident is the expensive direction.
type SetSpendThresholdsRequest struct {
	Warning *float64 `json:"warning"`
	Limit   *float64 `json:"limit"`
}

// SetBillingSpendThresholds handles
// PUT /api/v1/accounts/:account/billing/spend/thresholds.
func SetBillingSpendThresholds(log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		var req SetSpendThresholdsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
			return
		}
		// A negative threshold fires the moment it exists, which reads as an
		// outage rather than a control the owner chose.
		for _, v := range []*float64{req.Warning, req.Limit} {
			if v != nil && *v < 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "a spend threshold cannot be negative"})
				return
			}
		}
		// A warning at or above the limit never fires on its own: the limit
		// suspends the account first, so the warning is silently useless.
		if req.Warning != nil && req.Limit != nil && *req.Warning >= *req.Limit {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the warning must be below the limit"})
			return
		}

		writer, ok := billingProvider.(billing.SpendThresholdWriter)
		if !ok {
			c.JSON(http.StatusOK, BillingDataResponse{Available: false})
			return
		}
		customerID, ok := resolveBillingCustomer(c, log, accountStore, billingProvider, billingBackend, acct)
		if !ok {
			c.JSON(http.StatusOK, BillingDataResponse{Available: false})
			return
		}

		applied, failed, err := writeSpendThresholds(c.Request.Context(), writer, customerID, req.Warning, req.Limit)
		if err != nil {
			log.Error("Failed to write spend threshold", "error", err, "account_id", acct.ID, "kind", string(failed))
			// Changing a threshold archives the old alert before creating its
			// replacement, so a failure can leave that control unset. Name it rather
			// than reporting a generic failure over an account that may now be
			// uncapped.
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "failed to save spend controls",
				"details": fmt.Sprintf("the %s may now be unset; re-save to restore it", failed),
				"applied": applied,
			})
			return
		}
		c.JSON(http.StatusOK, BillingDataResponse{Available: true})
	}
}

func GetBillingBalances(log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string) gin.HandlerFunc {
	return billingData(log, accountStore, billingProvider, billingBackend, "balances",
		func(ctx context.Context, customerID string) (any, error) {
			return billingProvider.Balances(ctx, customerID)
		})
}
