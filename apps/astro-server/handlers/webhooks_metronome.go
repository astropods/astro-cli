package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// BillingGateQueue enqueues workload suspend/resume on billing-status
// transitions. Satisfied by *riverqueue.Queue; nil disables the enqueue.
type BillingGateQueue interface {
	InsertBillingSuspend(ctx context.Context, accountID string) error
	InsertBillingResume(ctx context.Context, accountID string) error
}

// metronomeWebhookEnvelope is the minimal shape we read to route a webhook and
// map it back to an account.
type metronomeWebhookEnvelope struct {
	Type       string `json:"type"`
	Properties struct {
		CustomerID string `json:"customer_id"`
	} `json:"properties"`
}

// MetronomeWebhook handles POST /webhooks/metronome. It verifies the
// HMAC-SHA256 signature over `Metronome-Webhook-Date + "\n" + rawBody` (keyed by
// METRONOME_WEBHOOK_SECRET, hex-encoded, compared to the
// Metronome-Webhook-Signature header) before dispatching on event type.
//
// Payment/alert events drive the cached billing-gating status
// (account_billing_status): the server sets/clears dunning + alert flags and
// recomputes. It never reads a balance — Metronome owns that; we react to its
// signals. statusStore is nil for backends without gating (noop), in which case
// status writes are skipped.
//
// The raw body must be read before any JSON middleware; this handler reads it
// directly. When no secret is configured the endpoint is disabled (404).
func MetronomeWebhook(log *logger.Logger, secret string, accountStore *account.AccountStore, statusStore *billing.StatusStore, gate BillingGateQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "webhook not configured"})
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
			return
		}

		date := c.GetHeader("Metronome-Webhook-Date")
		sig := c.GetHeader("Metronome-Webhook-Signature")
		if !verifyMetronomeSignature(secret, date, body, sig) {
			log.Warn("Metronome webhook signature verification failed")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}

		var env metronomeWebhookEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}

		switch env.Type {
		case "invoice.finalized":
			// TODO(metronome): fetch line items via V1.Customers.Invoices.Get and reconcile.
			log.Info("Metronome webhook: invoice finalized")
		case "payment.failed", "invoice.payment_failed":
			log.Info("Metronome webhook: payment failed", "customer_id", env.Properties.CustomerID)
			applyBillingSignal(c, log, accountStore, statusStore, gate, env.Properties.CustomerID, signalPaymentFailed)
		case "alert.threshold_reached", "threshold.reached":
			log.Info("Metronome webhook: threshold reached", "customer_id", env.Properties.CustomerID)
			applyBillingSignal(c, log, accountStore, statusStore, gate, env.Properties.CustomerID, signalAlert)
		case "invoice.paid", "payment.succeeded":
			log.Info("Metronome webhook: payment recovered", "customer_id", env.Properties.CustomerID)
			applyBillingSignal(c, log, accountStore, statusStore, gate, env.Properties.CustomerID, signalRecovery)
		default:
			log.Info("Metronome webhook: unhandled event", "type", env.Type)
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

type billingSignal int

const (
	signalPaymentFailed billingSignal = iota
	signalAlert
	signalRecovery
)

// applyBillingSignal maps a Metronome customer to an account, writes the
// dunning/alert flags for the signal, and recomputes the cached status.
// Best-effort: logs and returns on any miss (no gating backend, unknown
// customer, DB error) — a webhook is never failed over gating bookkeeping.
func applyBillingSignal(c *gin.Context, log *logger.Logger, accountStore *account.AccountStore, statusStore *billing.StatusStore, gate BillingGateQueue, customerID string, sig billingSignal) {
	if statusStore == nil || accountStore == nil || customerID == "" {
		return
	}
	ctx := c.Request.Context()
	acct, err := accountStore.GetByMetronomeCustomerID(customerID)
	if err != nil {
		log.Warn("Metronome webhook: no account for customer", "customer_id", customerID, "error", err)
		return
	}
	now := time.Now()
	switch sig {
	case signalPaymentFailed:
		err = statusStore.SetDunningSince(ctx, acct.ID, now)
	case signalAlert:
		err = statusStore.SetAlert(ctx, acct.ID)
	case signalRecovery:
		if e := statusStore.ClearDunning(ctx, acct.ID); e != nil {
			err = e
		} else {
			err = statusStore.ClearAlert(ctx, acct.ID)
		}
	}
	if err != nil {
		log.Error("Metronome webhook: failed to write billing signal", "account_id", acct.ID, "error", err)
		return
	}
	status, changed, err := statusStore.Recompute(ctx, acct.ID, now)
	if err != nil {
		log.Error("Metronome webhook: failed to recompute billing status", "account_id", acct.ID, "error", err)
		return
	}
	if !changed {
		return
	}
	log.Info("billing status changed", "account_id", acct.ID, "status", string(status))
	if gate == nil {
		return
	}
	switch status {
	case billing.StatusSuspended:
		if err := gate.InsertBillingSuspend(ctx, acct.ID); err != nil {
			log.Error("failed to enqueue billing suspend", "account_id", acct.ID, "error", err)
		}
	case billing.StatusActive:
		if err := gate.InsertBillingResume(ctx, acct.ID); err != nil {
			log.Error("failed to enqueue billing resume", "account_id", acct.ID, "error", err)
		}
	}
}

// verifyMetronomeSignature computes HMAC-SHA256 over date + "\n" + body and
// constant-time compares the hex digest to the provided signature.
func verifyMetronomeSignature(secret, date string, body []byte, sig string) bool {
	if sig == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(date))
	mac.Write([]byte("\n"))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig))
}
