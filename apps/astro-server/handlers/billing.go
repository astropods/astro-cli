package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/config"
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
	// LimitLiftFailed reports that the controls saved but the account stayed
	// stopped by its own cap. Nothing retries it, so a client that reports a
	// plain success strands the owner.
	LimitLiftFailed bool `json:"limit_lift_failed,omitempty"`
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
	// PayLink is Stripe's hosted page for a charge waiting on authentication,
	// set only alongside the complete_payment action.
	PayLink string `json:"pay_link,omitempty"`
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
			log.Error("billing: load billing status failed", "error", err, "account_id", acct.ID)
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
				log.Warn("billing: check suspended workloads failed", "error", derr, "account_id", acct.ID)
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
			resp.Action = middleware.BillingAction(rec.Reason, rec.PayLink != "")
			if resp.Action == middleware.ActionCompletePayment {
				resp.PayLink = rec.PayLink
			}
		}
		c.JSON(http.StatusOK, resp)
	}
}

// resolveBillingCustomer resolves the account's billing customer, lazily
// creating one on first access. Returns ("", false) when billing is not
// available for this environment (OSS/noop) or the customer can't be
// created/resolved — the caller should respond with Available:false.
func resolveBillingCustomer(c *gin.Context, log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string, acct *account.Account) (string, bool) {
	if billingProvider == nil || !config.BillingBackendHasCustomers(billingBackend) {
		return "", false
	}

	// The fake derives its customer id, so it never persists one. Sharing a
	// stored column would leave a bogus id behind on every account browsed,
	// and DATABASE_URL in local development points at a shared database.
	if billingBackend == config.BillingBackendFake {
		id, err := billingProvider.CreateCustomer(c.Request.Context(), billing.Account{
			ID: acct.ID, Name: acct.Name, Type: acct.Type,
		})
		if err != nil {
			return "", false
		}
		return id, true
	}

	customerID, err := accountStore.GetBillingCustomerID(acct.ID, billingBackend)
	if err != nil {
		log.Warn("billing: load billing customer ID failed", "error", err, "account_id", acct.ID)
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
		log.Error("billing: create billing customer failed", "error", err, "account_id", acct.ID)
		return "", false
	}
	if err := accountStore.SetBillingCustomerID(acct.ID, billingBackend, customerID); err != nil {
		log.Error("billing: store billing customer ID failed", "error", err, "account_id", acct.ID)
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
	fetch func(ctx context.Context, acct *account.Account, customerID string) (any, error),
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

		data, err := fetch(c.Request.Context(), acct, customerID)
		if err != nil {
			if errors.Is(err, billing.ErrBillingUnavailable) {
				c.JSON(http.StatusOK, BillingDataResponse{Available: false})
				return
			}
			if errors.Is(err, errNoBillingContract) {
				log.Error("billing: no billing contract covers the account", "account_id", acct.ID, "customer_id", customerID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "no billing contract covers this account"})
				return
			}
			log.Error("billing: load billing  failed"+label, "error", err, "account_id", acct.ID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load billing " + label})
			return
		}

		c.JSON(http.StatusOK, BillingDataResponse{Available: true, Data: data})
	}
}

var errNoBillingContract = errors.New("no billing contract covers this account")

// utcMidnight truncates t to UTC midnight (Metronome requires window bounds to
// be UTC midnight).
func utcMidnight(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// maxBillingWindowDays bounds how wide a caller can make [from, to). The
// invoice breakdown pages 35 days at a time, so an unbounded window pages
// serially until the request's own deadline cuts it off; the caller then
// sees a bare 502 with nothing pointing at the window as the cause. A year
// covers any real report without leaving that failure mode reachable.
const maxBillingWindowDays = 366

// parseBillingWindow reads the optional from/to query params shared by every
// billing endpoint windowed on [from, to), defaulting to the current
// calendar month. Metronome requires the bounds to be UTC midnight; the end
// defaults to the start of tomorrow so today's partial data is included.
// Returns ok=false, having already written the 400 response, when the
// resulting window is wider than maxBillingWindowDays.
func parseBillingWindow(c *gin.Context) (from, to time.Time, ok bool) {
	now := time.Now().UTC()
	from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to = utcMidnight(now).AddDate(0, 0, 1)
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
	if to.Sub(from) > maxBillingWindowDays*24*time.Hour {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("window cannot exceed %d days", maxBillingWindowDays)})
		return from, to, false
	}
	return from, to, true
}

// GetBillingUsage handles GET /api/v1/accounts/:account/billing/usage. It
// returns metered usage over [from, to) (defaults to the current calendar
// month), aggregated per day, exactly as the provider reports it.
func GetBillingUsage(log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string) gin.HandlerFunc {
	return func(c *gin.Context) {
		from, to, ok := parseBillingWindow(c)
		if !ok {
			return
		}
		billingData(log, accountStore, billingProvider, billingBackend, "usage",
			func(ctx context.Context, _ *account.Account, customerID string) (any, error) {
				return billingProvider.UsageData(ctx, customerID, from, to)
			})(c)
	}
}

// GetBillingDailySpend handles
// GET /api/v1/accounts/:account/billing/usage/daily-spend. It returns the
// account's rated spend per calendar day over [from, to) (defaults to the
// current calendar month): one dollar figure per day, already summed across
// every billable metric, for a daily spend chart. UsageData's raw per-metric
// rows can't answer this on their own, since a quantity metric like Compute
// Units has no dollar figure at any window size.
func GetBillingDailySpend(log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string) gin.HandlerFunc {
	return func(c *gin.Context) {
		from, to, ok := parseBillingWindow(c)
		if !ok {
			return
		}
		billingData(log, accountStore, billingProvider, billingBackend, "daily-spend",
			func(ctx context.Context, _ *account.Account, customerID string) (any, error) {
				return billingProvider.DailySpend(ctx, customerID, from, to)
			})(c)
	}
}

// GetBillingInvoices handles GET /api/v1/accounts/:account/billing/invoices. It
// returns the customer's invoices (with line items) for the client to render.
func GetBillingInvoices(log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string) gin.HandlerFunc {
	return billingData(log, accountStore, billingProvider, billingBackend, "invoices",
		func(ctx context.Context, _ *account.Account, customerID string) (any, error) {
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
				c.JSON(http.StatusNotFound, gin.H{"error": "no PDF is available for this invoice"})
				return
			}
			log.Error("billing: load invoice PDF failed", "error", err, "account_id", acct.ID, "invoice_id", invoiceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load invoice PDF"})
			return
		}
		defer rc.Close() //nolint:errcheck

		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", "inline; filename=\"invoice.pdf\"")
		if _, err := io.Copy(c.Writer, rc); err != nil {
			log.Warn("billing: stream invoice PDF failed", "error", err, "account_id", acct.ID, "invoice_id", invoiceID)
		}
	}
}

// BillingSpendResponse is the customer-facing view of what an account is running
// up and the controls it set on itself. Spend and thresholds ship together
// because a threshold is meaningless without the number it is measured against.
type BillingSpendResponse struct {
	Currency string `json:"currency,omitempty"`

	Plan string `json:"plan,omitempty"`

	CurrentSpend       float64   `json:"current_spend"`
	HasCurrentSpend    bool      `json:"has_current_spend"`
	CurrentPeriodStart time.Time `json:"current_period_start,omitzero"`
	CurrentPeriodEnd   time.Time `json:"current_period_end,omitzero"`

	// UsageSpend is what the thresholds below are measured against: the period's
	// usage before credit drawdown. It differs from CurrentSpend whenever credit
	// covers part of the bill, and an account on signup credit reads zero for
	// CurrentSpend while UsageSpend climbs toward its own warning.
	UsageSpend    float64 `json:"usage_spend"`
	HasUsageSpend bool    `json:"has_usage_spend"`

	CreditRemaining float64 `json:"credit_remaining"`
	HasCredit       bool    `json:"has_credit"`

	// Most recently finalized invoice. Only HasLastInvoice is read today
	// (gates "View invoices"); the rest is exposed for a future client.
	LastInvoiceTotal float64   `json:"last_invoice_total,omitempty"`
	LastInvoiceAt    time.Time `json:"last_invoice_at,omitzero"`
	HasLastInvoice   bool      `json:"has_last_invoice"`

	// Absent when the customer set none. Nil rather than zero, which is a
	// threshold a customer could legitimately set.
	Warning *SpendThresholdResponse `json:"warning,omitempty"`
	Limit   *SpendThresholdResponse `json:"limit,omitempty"`

	Usage map[string]UsageThresholdsResponse `json:"usage,omitempty"`
}

type UsageThresholdsResponse struct {
	Unit    string                  `json:"unit"`
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
		func(ctx context.Context, acct *account.Account, customerID string) (any, error) {
			reporter, ok := billingProvider.(billing.SpendReporter)
			if !ok {
				return nil, billing.ErrBillingUnavailable
			}
			spend, err := reporter.CustomerSpend(ctx, customerID)
			if err != nil {
				return nil, err
			}
			resp := BillingSpendResponse{
				Currency:           spend.Currency,
				CurrentSpend:       spend.CurrentSpend,
				HasCurrentSpend:    spend.HasCurrentSpend,
				CurrentPeriodStart: spend.CurrentPeriodStart,
				CurrentPeriodEnd:   spend.CurrentPeriodEnd,
				UsageSpend:         spend.UsageSpend,
				HasUsageSpend:      spend.HasUsageSpend,
				CreditRemaining:    spend.CreditRemaining,
				HasCredit:          spend.HasCredit,
				LastInvoiceTotal:   spend.LastInvoiceTotal,
				LastInvoiceAt:      spend.LastInvoiceAt,
				HasLastInvoice:     spend.HasLastInvoice,
			}
			if planner, ok := billingProvider.(billing.PlanReporter); ok {
				plan, covered, perr := planner.CustomerPlan(ctx, customerID)
				if perr != nil {
					log.Warn("billing: load billing plan failed", "error", perr, "customer_id", customerID)
				} else {
					resp.Plan = string(plan)
					if plan == "" && covered {
						log.Warn("billing: contract sits on a package this build does not recognise",
							"account_id", acct.ID, "customer_id", customerID)
					}
					if !covered {
						return nil, errNoBillingContract
					}
				}
			}
			if reader, ok := billingProvider.(billing.UsageThresholdReader); ok {
				usage, uerr := reader.CustomerUsageThresholds(ctx, customerID)
				if uerr != nil {
					log.Warn("billing: load usage thresholds failed", "error", uerr, "customer_id", customerID)
				} else {
					resp.Usage = usageThresholdsResponse(usage)
				}
			}
			// Best-effort: a threshold read that fails must not hide the spend,
			// which is the half a customer needs to set one in the first place.
			if reader, ok := billingProvider.(billing.SpendThresholdReader); ok {
				th, terr := reader.CustomerSpendThresholds(ctx, customerID)
				if terr != nil {
					log.Warn("billing: load spend thresholds failed", "error", terr, "customer_id", customerID)
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

// A threshold too high to ever fire leaves the account uncapped while the
// settings page still shows a cap. Usage thresholds carry a different unit per
// metric, so theirs is a typo guard rather than a product ceiling.
const maxThresholdAmount = 1e9

// Cents, matching the request body. The spend limit is the one control a
// customer can use to raise our exposure, so it stops where self-serve does
// instead of at a typo guard.
const maxSpendThresholdCents = billing.MaxSelfServeSpendUSD * 100

// billingReconcileTimeout bounds a reconcile that outlives its request.
const billingReconcileTimeout = 15 * time.Second

// reconcileContext detaches from the request so a post-write reconcile survives
// a client that hangs up. By the time these run the provider write has already
// landed, so cancelling here leaves our side stale. Bounded, so a hung provider
// cannot pin a goroutine after the response is gone.
func reconcileContext(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(c.Request.Context()), billingReconcileTimeout)
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
func SetBillingSpendThresholds(log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string, status *billing.StatusStore, queue billingReconcileQueue) gin.HandlerFunc {
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
			if v == nil {
				continue
			}
			if *v < 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "a spend threshold cannot be negative"})
				return
			}
			if *v > maxSpendThresholdCents {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("a spend threshold cannot exceed $%.0f per month; contact support about an enterprise plan to raise it",
						billing.MaxSelfServeSpendUSD)})
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
		// Unconditional, and before the error branch: a failure can leave the
		// limit unset too, because replacing a threshold archives the old alert
		// before creating the new one. This only shortens the delay, so a miss
		// costs one sweep interval rather than the ceiling.
		reconcileCtx, cancelReconcile := reconcileContext(c)
		defer cancelReconcile()
		if queue != nil {
			if qerr := queue.InsertBillingGatewayBudget(reconcileCtx, acct.ID); qerr != nil {
				log.Error("billing: enqueue gateway budget re-derive failed", "error", qerr, "account_id", acct.ID)
			}
		}
		if err != nil {
			log.Error("billing: write spend threshold failed", "error", err, "account_id", acct.ID, "kind", string(failed))
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
		resp := BillingDataResponse{Available: true}
		if err := liftSelfLimit(reconcileCtx, status, queue, billingProvider, acct.ID, customerID); err != nil {
			log.Error("billing: lift the self-limit latch failed", "error", err, "account_id", acct.ID)
			resp.LimitLiftFailed = true
		}
		c.JSON(http.StatusOK, resp)
	}
}

func usageThresholdsResponse(in map[billing.UsageMetric]billing.UsageThresholds) map[string]UsageThresholdsResponse {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]UsageThresholdsResponse, len(in))
	for metric, t := range in {
		if !t.HasWarning && !t.HasLimit {
			continue
		}
		entry := UsageThresholdsResponse{Unit: billing.UsageMetricUnit(metric)}
		if t.HasWarning {
			entry.Warning = &SpendThresholdResponse{Amount: t.Warning.Amount, InAlarm: t.Warning.InAlarm}
		}
		if t.HasLimit {
			entry.Limit = &SpendThresholdResponse{Amount: t.Limit.Amount, InAlarm: t.Limit.InAlarm}
		}
		out[string(metric)] = entry
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type SetUsageThresholdsRequest struct {
	Metric  string   `json:"metric"`
	Warning *float64 `json:"warning"`
	Limit   *float64 `json:"limit"`
}

func SetBillingUsageThresholds(log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string, status *billing.StatusStore, queue billingReconcileQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		var req SetUsageThresholdsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
			return
		}
		metric, ok := parseUsageMetric(req.Metric)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown usage metric", "details": req.Metric})
			return
		}
		for _, v := range []*float64{req.Warning, req.Limit} {
			if v == nil {
				continue
			}
			if *v < 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "a usage threshold cannot be negative"})
				return
			}
			if *v > maxThresholdAmount {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("a usage threshold cannot exceed %.0f", maxThresholdAmount)})
				return
			}
		}
		if req.Warning != nil && req.Limit != nil && *req.Warning >= *req.Limit {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the warning must be below the limit"})
			return
		}

		writer, ok := billingProvider.(billing.UsageThresholdWriter)
		if !ok {
			c.JSON(http.StatusOK, BillingDataResponse{Available: false})
			return
		}
		customerID, ok := resolveBillingCustomer(c, log, accountStore, billingProvider, billingBackend, acct)
		if !ok {
			c.JSON(http.StatusOK, BillingDataResponse{Available: false})
			return
		}

		ctx := c.Request.Context()
		for _, w := range []struct {
			kind   billing.SpendThresholdKind
			amount *float64
		}{
			{billing.SpendThresholdLimit, req.Limit},
			{billing.SpendThresholdWarning, req.Warning},
		} {
			var err error
			if w.amount == nil {
				err = writer.ClearCustomerUsageThreshold(ctx, customerID, metric, w.kind)
			} else {
				err = writer.SetCustomerUsageThreshold(ctx, customerID, metric, w.kind, *w.amount)
			}
			if err != nil {
				log.Error("billing: write usage threshold failed", "error", err, "account_id", acct.ID, "kind", string(w.kind))
				c.JSON(http.StatusBadGateway, gin.H{
					"error":   "failed to save usage controls",
					"details": fmt.Sprintf("the %s may now be unset; re-save to restore it", w.kind),
				})
				return
			}
		}

		resp := BillingDataResponse{Available: true}
		reconcileCtx, cancelReconcile := reconcileContext(c)
		defer cancelReconcile()
		if err := liftSelfLimit(reconcileCtx, status, queue, billingProvider, acct.ID, customerID); err != nil {
			log.Error("billing: lift the self-limit latch failed", "error", err, "account_id", acct.ID)
			resp.LimitLiftFailed = true
		}
		c.JSON(http.StatusOK, resp)
	}
}

const centsPerUnit = 100

// selfLimitReached measures each limit the account set against what the period
// counted. It cannot read the provider's in_alarm instead: a cap written moments
// ago was archived and recreated, so that flag still answers for the number it
// replaced. Spend is measured before credit drawdown, which is what the spend
// alert itself evaluates.
func selfLimitReached(ctx context.Context, provider any, customerID string) (bool, error) {
	if reader, ok := provider.(billing.SpendThresholdReader); ok {
		th, err := reader.CustomerSpendThresholds(ctx, customerID)
		if err != nil {
			return false, err
		}
		reporter, hasSpend := provider.(billing.SpendReporter)
		if th.HasLimit && hasSpend {
			spend, err := reporter.CustomerSpend(ctx, customerID)
			if err != nil {
				return false, err
			}
			// Rounded before the comparison, as the webhook's own spend read is:
			// scaling dollars back to cents in float can land a hair under a limit
			// the account is exactly at.
			if spend.HasUsageSpend && math.Round(spend.UsageSpend*centsPerUnit) >= th.Limit.Amount {
				return true, nil
			}
		}
	}
	reader, ok := provider.(billing.UsageThresholdReader)
	quantities, hasQuantities := provider.(billing.UsageQuantityReader)
	if !ok || !hasQuantities {
		return false, nil
	}
	caps, err := reader.CustomerUsageThresholds(ctx, customerID)
	if err != nil {
		return false, err
	}
	for metric, c := range caps {
		if !c.HasLimit {
			continue
		}
		used, err := quantities.CustomerMetricUsage(ctx, customerID, metric)
		if err != nil {
			return false, err
		}
		if used >= c.Limit.Amount {
			return true, nil
		}
	}
	return false, nil
}

func liftSelfLimit(
	ctx context.Context,
	status *billing.StatusStore,
	queue billingReconcileQueue,
	provider billing.BillingProvider,
	accountID, customerID string,
) error {
	if status == nil {
		return nil
	}
	rec, err := status.Record(ctx, accountID)
	if err != nil || !rec.UsageLimitActive {
		return err
	}
	reached, err := selfLimitReached(ctx, provider, customerID)
	if err != nil || reached {
		return err
	}
	newStatus, _, err := billing.ApplySignal(ctx, status, accountID, billing.SignalUsageLimitResolved, time.Now())
	if err != nil {
		return err
	}
	if queue != nil && newStatus == billing.StatusActive {
		return queue.InsertBillingResume(ctx, accountID)
	}
	return nil
}

func parseUsageMetric(s string) (billing.UsageMetric, bool) {
	for _, m := range billing.AllUsageMetrics {
		if string(m) == s {
			return m, true
		}
	}
	return "", false
}
