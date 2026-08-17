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

// The alert name is the only thing separating an account's own warning from its
// limit. Dropping it at the edge would leave the worker unable to tell them
// apart, and the warning would gate.
func TestMetronomeWebhook_ForwardsTheAlertName(t *testing.T) {
	const secret = "whsec-test"
	const date = "2026-08-12T00:00:00Z"
	body := `{"id":"evt_9","type":"alerts.spend_threshold_reached","properties":{` +
		`"customer_id":"cust_1","alert_id":"a1","alert_name":"astro:spend_warning",` +
		`"threshold":2500,"current_spend":2600}}`

	q := &fakeWebhookQueue{}
	req := httptest.NewRequest(http.MethodPost, "/webhooks/metronome", strings.NewReader(body))
	req.Header.Set("X-Metronome-Date", date)
	req.Header.Set("Metronome-Webhook-Signature", sign(secret, date, body))
	rec := httptest.NewRecorder()
	metronomeWebhookRouterWithQueue(secret, q).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if q.lastAlertName != "astro:spend_warning" {
		t.Errorf("alert name = %q, want astro:spend_warning", q.lastAlertName)
	}
}

// Metered spend accrues fractional cents, and this envelope decodes every
// Metronome webhook. An amount an int64 field rejects would 400 the event before
// any signal is read, so the alert that suspends an account over its limit would
// be dropped for the sake of a number only the message text uses.
func TestMetronomeWebhook_AcceptsFractionalAmounts(t *testing.T) {
	const secret = "whsec-test"
	const date = "2026-08-12T00:00:00Z"
	cases := []struct {
		name             string
		threshold, spend string
		wantThreshold    int64
		wantCurrentSpend int64
	}{
		{"whole numbers", "2500", "2600", 2500, 2600},
		{"fractional spend rounds down", "2500", "2600.4", 2500, 2600},
		{"fractional spend rounds up", "2500", "2600.5", 2500, 2601},
		{"a whole number written with a decimal point", "2500.0", "2600", 2500, 2600},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"id":"evt_f","type":"alerts.spend_threshold_reached","properties":{` +
				`"customer_id":"cust_1","alert_name":"astro:spend_limit",` +
				`"threshold":` + tc.threshold + `,"current_spend":` + tc.spend + `}}`

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
				t.Fatalf("enqueued %d jobs, want 1: the gating event was dropped", q.metronomeCalls)
			}
			if q.lastMetronome.Threshold != tc.wantThreshold {
				t.Errorf("threshold = %d, want %d", q.lastMetronome.Threshold, tc.wantThreshold)
			}
			if q.lastMetronome.CurrentSpend != tc.wantCurrentSpend {
				t.Errorf("current_spend = %d, want %d", q.lastMetronome.CurrentSpend, tc.wantCurrentSpend)
			}
		})
	}
}
