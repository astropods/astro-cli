package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v86/webhook"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// stripeEventObject is the minimal slice of the event's data.object we need:
// the customer (a plain id string in webhook payloads, not expanded) and the
// hosted invoice URL (present on invoice events, used to surface a 3DS pay-link).
type stripeEventObject struct {
	Customer         string `json:"customer"`
	HostedInvoiceURL string `json:"hosted_invoice_url"`
}

// previousCustomer reads the customer the object was attached to before the
// event. A detach clears the field on the object, so the id survives only here,
// and it is the only thing that maps the event to an account. Without it a
// removed card is never recorded and the account keeps the exemption a card
// grants it.
func previousCustomer(previous map[string]any) string {
	id, _ := previous["customer"].(string)
	return id
}

// StripeWebhook handles POST /webhooks/stripe. It is the only source of
// payment-collection state (Metronome does not relay Stripe payment failures).
// The signature is verified with the stripe-go SDK against the Stripe-Signature
// header (keyed by STRIPE_WEBHOOK_SECRET); the verified event is then enqueued
// as a River job for tracked, retryable processing (StripeWebhookWorker maps the
// Stripe customer to an account and drives the cached billing status).
//
// astro-server never charges — collection and retry stay Stripe/Metronome's; we
// only mirror state. The raw body must be read before any JSON middleware. When
// no secret is configured the endpoint is disabled (404).
func StripeWebhook(log *logger.Logger, secret string, queue WebhookQueue) gin.HandlerFunc {
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

		event, err := webhook.ConstructEvent(body, c.GetHeader("Stripe-Signature"), secret)
		if err != nil {
			log.Warn("webhooks stripe: Stripe webhook signature verification failed", "error", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}

		var obj stripeEventObject
		if event.Data != nil && len(event.Data.Raw) > 0 {
			if err := json.Unmarshal(event.Data.Raw, &obj); err != nil {
				// A verified event we can't parse is an internal problem, not a
				// client error — 500 so Stripe redelivers rather than us acking
				// and silently dropping a possible payment-failure signal.
				log.Error("webhooks stripe: parse event object failed", "type", string(event.Type), "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse event"})
				return
			}
		}

		customer := obj.Customer
		if customer == "" && event.Data != nil {
			customer = previousCustomer(event.Data.PreviousAttributes)
		}

		if queue != nil {
			if err := queue.InsertStripeWebhook(c.Request.Context(), event.ID, string(event.Type), customer, obj.HostedInvoiceURL); err != nil {
				// Return 500 so Stripe redelivers — the event is not yet tracked.
				log.Error("webhooks stripe: enqueue failed", "type", string(event.Type), "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "enqueue failed"})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
