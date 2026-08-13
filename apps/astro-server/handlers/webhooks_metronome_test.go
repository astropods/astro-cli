package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func sign(secret, date, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(date))
	mac.Write([]byte("\n"))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func metronomeWebhookRouter(secret string) *gin.Engine {
	return metronomeWebhookRouterWithQueue(secret, nil)
}

func metronomeWebhookRouterWithQueue(secret string, q WebhookQueue) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/webhooks/metronome", MetronomeWebhook(logger.New("error", "json"), secret, q))
	return r
}

// A delivery failure to Stripe reports nothing but this webhook: the account's
// billing status is unaffected and Metronome keeps finalizing invoices. Losing
// the payload's error text leaves the log with no diagnosis.
func TestMetronomeWebhook_ForwardsTheBillingProviderError(t *testing.T) {
	const secret = "whsec-test"
	const date = "2026-08-12T00:00:00Z"
	body := `{"id":"evt_1","type":"invoice.billing_provider_error","properties":{` +
		`"invoice_id":"inv_1","customer_id":"cust_1","billing_provider":"STRIPE",` +
		`"billing_provider_error":"No token found for environment type SANDBOX and billing provider STRIPE"}}`

	q := &fakeWebhookQueue{}
	req := httptest.NewRequest(http.MethodPost, "/webhooks/metronome", strings.NewReader(body))
	req.Header.Set("X-Metronome-Date", date)
	req.Header.Set("Metronome-Webhook-Signature", sign(secret, date, body))
	rec := httptest.NewRecorder()
	metronomeWebhookRouterWithQueue(secret, q).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if q.metronomeCalls != 1 {
		t.Fatalf("enqueued %d events, want 1", q.metronomeCalls)
	}
	if !strings.Contains(q.lastDetail, "No token found") {
		t.Errorf("detail = %q, want the provider error text", q.lastDetail)
	}
	if !strings.Contains(q.lastDetail, "inv_1") {
		t.Errorf("detail = %q, want the invoice id", q.lastDetail)
	}
}

// An integration issue names the integration and an error code instead of an
// invoice, so it renders from different fields.
func TestMetronomeWebhook_ForwardsTheIntegrationIssue(t *testing.T) {
	const secret = "whsec-test"
	const date = "2026-08-12T00:00:00Z"
	body := `{"id":"evt_2","type":"integration.issue","properties":{` +
		`"integration":"STRIPE","error":"Authentication failed","error_code":"INVALID_CREDENTIALS"}}`

	q := &fakeWebhookQueue{}
	req := httptest.NewRequest(http.MethodPost, "/webhooks/metronome", strings.NewReader(body))
	req.Header.Set("X-Metronome-Date", date)
	req.Header.Set("Metronome-Webhook-Signature", sign(secret, date, body))
	rec := httptest.NewRecorder()
	metronomeWebhookRouterWithQueue(secret, q).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(q.lastDetail, "INVALID_CREDENTIALS") {
		t.Errorf("detail = %q, want the error code", q.lastDetail)
	}
}

// An alert carries no error fields, and a detail invented for it would read as
// a failure in the log.
func TestMetronomeWebhook_LeavesDetailEmptyForAlerts(t *testing.T) {
	const secret = "whsec-test"
	const date = "2026-08-12T00:00:00Z"
	body := `{"id":"evt_3","type":"alerts.spend_threshold_reached","properties":{"customer_id":"cust_1"}}`

	q := &fakeWebhookQueue{}
	req := httptest.NewRequest(http.MethodPost, "/webhooks/metronome", strings.NewReader(body))
	req.Header.Set("X-Metronome-Date", date)
	req.Header.Set("Metronome-Webhook-Signature", sign(secret, date, body))
	rec := httptest.NewRecorder()
	metronomeWebhookRouterWithQueue(secret, q).ServeHTTP(rec, req)

	if q.lastDetail != "" {
		t.Errorf("detail = %q, want empty", q.lastDetail)
	}
}

// The date header name has to match what Metronome actually sends, so each
// accepted name is asserted separately rather than through a shared helper.
func TestMetronomeWebhook_ValidSignature(t *testing.T) {
	const secret = "whsec-test"
	body := `{"type":"invoice.finalized"}`
	date := "2026-07-15T00:00:00Z"

	for _, header := range []string{"X-Metronome-Date", "Date"} {
		t.Run(header, func(t *testing.T) {
			r := metronomeWebhookRouter(secret)
			req := httptest.NewRequest(http.MethodPost, "/webhooks/metronome", strings.NewReader(body))
			req.Header.Set(header, date)
			req.Header.Set("Metronome-Webhook-Signature", sign(secret, date, body))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// A real delivery carries both headers. X-Metronome-Date is the signed one, so
// signing over it must be accepted even when Date disagrees.
func TestMetronomeWebhook_PrefersXMetronomeDate(t *testing.T) {
	const secret = "whsec-test"
	body := `{"id":"evt_1","type":"alerts.low_remaining_contract_credit_balance_reached"}`
	date := "2026-08-11T16:36:54Z"

	r := metronomeWebhookRouter(secret)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/metronome", strings.NewReader(body))
	req.Header.Set("X-Metronome-Date", date)
	req.Header.Set("Date", "Mon, 11 Aug 2026 16:36:54 GMT")
	req.Header.Set("Metronome-Webhook-Signature", sign(secret, date, body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMetronomeWebhook_BadSignature(t *testing.T) {
	r := metronomeWebhookRouter("whsec-test")
	body := `{"type":"invoice.finalized"}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/metronome", strings.NewReader(body))
	req.Header.Set("X-Metronome-Date", "2026-07-15T00:00:00Z")
	req.Header.Set("Metronome-Webhook-Signature", "deadbeef")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMetronomeWebhook_Disabled(t *testing.T) {
	r := metronomeWebhookRouter("") // no secret configured
	req := httptest.NewRequest(http.MethodPost, "/webhooks/metronome", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when disabled, got %d", rec.Code)
	}
}
