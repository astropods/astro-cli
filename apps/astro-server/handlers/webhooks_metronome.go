package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// WebhookQueue enqueues verified billing webhooks for off-request-path
// processing. The handlers only verify signatures and enqueue; account mapping
// and status recompute run in the River worker so every event is tracked and
// retried. Satisfied by *riverqueue.Queue; nil disables enqueue (endpoint 404s
// when its secret is also unset).
type WebhookQueue interface {
	InsertMetronomeWebhook(ctx context.Context, eventID, eventType, customerID string) error
	InsertStripeWebhook(ctx context.Context, eventID, eventType, customerID, hostedInvoiceURL string) error
}

// metronomeWebhookEnvelope is the minimal shape we read to route a webhook and
// map it back to an account. id is used as the River idempotency key.
type metronomeWebhookEnvelope struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Properties struct {
		CustomerID string `json:"customer_id"`
	} `json:"properties"`
}

// MetronomeWebhook handles POST /webhooks/metronome. It verifies the
// HMAC-SHA256 signature over `Metronome-Webhook-Date + "\n" + rawBody` (keyed by
// METRONOME_WEBHOOK_SECRET, hex-encoded, compared to the
// Metronome-Webhook-Signature header), then enqueues the event as a River job
// for tracked, retryable processing (MetronomeWebhookWorker maps the customer to
// an account and drives the cached billing status).
//
// The raw body must be read before any JSON middleware; this handler reads it
// directly. When no secret is configured the endpoint is disabled (404).
func MetronomeWebhook(log *logger.Logger, secret string, queue WebhookQueue) gin.HandlerFunc {
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

		if queue != nil {
			if err := queue.InsertMetronomeWebhook(c.Request.Context(), env.ID, env.Type, env.Properties.CustomerID); err != nil {
				// Return 500 so Metronome redelivers — the event is not yet tracked.
				log.Error("Metronome webhook: enqueue failed", "type", env.Type, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "enqueue failed"})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
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
