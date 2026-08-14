package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// fakeWebhookQueue records the last enqueue so tests can assert the handler
// verified and forwarded the event without touching River.
type fakeWebhookQueue struct {
	stripeCalls    int
	metronomeCalls int
	lastEventID    string
	lastEventType  string
	lastCustomer   string
	lastURL        string
	lastAlertName  string
	lastDetail     string
	err            error
}

func (f *fakeWebhookQueue) InsertMetronomeWebhook(_ context.Context, eventID, eventType, customerID, alertName, detail string) error {
	f.metronomeCalls++
	f.lastEventID, f.lastEventType, f.lastCustomer, f.lastAlertName, f.lastDetail = eventID, eventType, customerID, alertName, detail
	return f.err
}

func (f *fakeWebhookQueue) InsertStripeWebhook(_ context.Context, eventID, eventType, customerID, hostedInvoiceURL string) error {
	f.stripeCalls++
	f.lastEventID, f.lastEventType, f.lastCustomer, f.lastURL = eventID, eventType, customerID, hostedInvoiceURL
	return f.err
}

func stripeWebhookRouter(secret string, q WebhookQueue) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/webhooks/stripe", StripeWebhook(logger.New("error", "json"), secret, q))
	return r
}

// signedStripeBody wraps a raw event body in a valid Stripe-Signature header.
func signedStripeBody(secret, body string) string {
	sp := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: []byte(body), Secret: secret})
	return sp.Header
}

func TestStripeWebhook_ValidSignatureEnqueues(t *testing.T) {
	const secret = "whsec-test"
	body := fmt.Sprintf(
		`{"id":"evt_123","object":"event","api_version":%q,"type":"invoice.payment_action_required","data":{"object":{"customer":"cus_abc","hosted_invoice_url":"https://pay.stripe.com/i/1"}}}`,
		stripe.APIVersion,
	)

	q := &fakeWebhookQueue{}
	r := stripeWebhookRouter(secret, q)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	req.Header.Set("Stripe-Signature", signedStripeBody(secret, body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if q.stripeCalls != 1 {
		t.Fatalf("expected 1 enqueue, got %d", q.stripeCalls)
	}
	if q.lastEventID != "evt_123" || q.lastEventType != "invoice.payment_action_required" ||
		q.lastCustomer != "cus_abc" || q.lastURL != "https://pay.stripe.com/i/1" {
		t.Fatalf("unexpected enqueue args: id=%q type=%q cust=%q url=%q",
			q.lastEventID, q.lastEventType, q.lastCustomer, q.lastURL)
	}
}

func TestStripeWebhook_BadSignature(t *testing.T) {
	q := &fakeWebhookQueue{}
	r := stripeWebhookRouter("whsec-test", q)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(`{"id":"evt_1"}`))
	req.Header.Set("Stripe-Signature", "t=1,v1=deadbeef")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if q.stripeCalls != 0 {
		t.Fatalf("expected no enqueue on bad signature, got %d", q.stripeCalls)
	}
}

func TestStripeWebhook_Disabled(t *testing.T) {
	r := stripeWebhookRouter("", &fakeWebhookQueue{}) // no secret configured
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when disabled, got %d", rec.Code)
	}
}
