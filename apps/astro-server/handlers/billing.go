package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
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
func GetBillingBalances(log *logger.Logger, accountStore *account.AccountStore, billingProvider billing.BillingProvider, billingBackend string) gin.HandlerFunc {
	return billingData(log, accountStore, billingProvider, billingBackend, "balances",
		func(ctx context.Context, customerID string) (any, error) {
			return billingProvider.Balances(ctx, customerID)
		})
}
