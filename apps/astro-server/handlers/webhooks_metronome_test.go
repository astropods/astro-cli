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
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/webhooks/metronome", MetronomeWebhook(logger.New("error", "json"), secret, nil))
	return r
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
