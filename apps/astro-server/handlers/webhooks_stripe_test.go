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

	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
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
	lastMetronome  riverqueue.MetronomeWebhookArgs
	err            error
}

func (f *fakeWebhookQueue) InsertMetronomeWebhook(_ context.Context, args riverqueue.MetronomeWebhookArgs) error {
	f.metronomeCalls++
	f.lastMetronome = args
	f.lastEventID, f.lastEventType, f.lastCustomer = args.EventID, args.EventType, args.CustomerID
	f.lastAlertName, f.lastDetail = args.AlertName, args.Detail
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

// stripeEventBody wraps an event body with the fields ConstructEvent validates,
// so a test asserts the handler's behaviour rather than its signature check.
func stripeEventBody(id, eventType, object, previous string) string {
	prev := ""
	if previous != "" {
		prev = fmt.Sprintf(`,"previous_attributes":%s`, previous)
	}
	return fmt.Sprintf(
		`{"id":%q,"object":"event","api_version":%q,"type":%q,"data":{"object":%s%s}}`,
		id, stripe.APIVersion, eventType, object, prev)
}

func postStripeEvent(t *testing.T, secret, body string, q WebhookQueue) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	req.Header.Set("Stripe-Signature", signedStripeBody(secret, body))
	rec := httptest.NewRecorder()
	stripeWebhookRouter(secret, q).ServeHTTP(rec, req)
	return rec
}

// A detach clears `customer` on the object, so the id survives only in
// previous_attributes. Enqueueing an empty customer leaves the event mapped to no
// account, and a card removed outside the app is then never recorded: the account
// keeps the exemption a card grants it while holding none.
func TestStripeWebhook_DetachFallsBackToThePreviousCustomer(t *testing.T) {
	const secret = "whsec-detach"
	q := &fakeWebhookQueue{}
	body := stripeEventBody("evt_detach_1", "payment_method.detached",
		`{"id":"pm_1","object":"payment_method","customer":null}`, `{"customer":"cus_prev_1"}`)

	if rec := postStripeEvent(t, secret, body, q); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if q.lastCustomer != "cus_prev_1" {
		t.Errorf("customer = %q, want cus_prev_1: the detach names no account without it", q.lastCustomer)
	}
}

// An attach carries the customer on the object, and previous_attributes must not
// override it. A replacement would otherwise be attributed to whatever the field
// held before.
func TestStripeWebhook_ObjectCustomerWinsOverPrevious(t *testing.T) {
	const secret = "whsec-attach"
	q := &fakeWebhookQueue{}
	body := stripeEventBody("evt_attach_1", "payment_method.attached",
		`{"id":"pm_2","object":"payment_method","customer":"cus_now"}`, `{"customer":"cus_before"}`)

	if rec := postStripeEvent(t, secret, body, q); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if q.lastCustomer != "cus_now" {
		t.Errorf("customer = %q, want cus_now", q.lastCustomer)
	}
}

// Some events carry no id in either place. Enqueueing the empty customer is still
// right: the worker acks it, and refusing at the edge would make Stripe redeliver
// an event nothing can ever resolve.
func TestStripeWebhook_NoCustomerAnywhereStillEnqueues(t *testing.T) {
	const secret = "whsec-none"
	q := &fakeWebhookQueue{}
	body := stripeEventBody("evt_none_1", "payment_method.detached",
		`{"id":"pm_3","object":"payment_method"}`, "")

	if rec := postStripeEvent(t, secret, body, q); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if q.stripeCalls != 1 {
		t.Fatalf("enqueued %d events, want 1", q.stripeCalls)
	}
	if q.lastCustomer != "" {
		t.Errorf("customer = %q, want empty", q.lastCustomer)
	}
}
