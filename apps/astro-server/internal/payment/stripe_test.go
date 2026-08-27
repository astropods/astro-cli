package payment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stripe/stripe-go/v86"
)

// No retries, so a non-2xx response fails once instead of stalling the test.
func stripeProvider(t *testing.T, h http.HandlerFunc) *Stripe {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	backends := stripe.NewBackendsWithConfig(&stripe.BackendConfig{
		URL:               stripe.String(srv.URL),
		HTTPClient:        srv.Client(),
		MaxNetworkRetries: stripe.Int64(0),
	})
	return &Stripe{sc: stripe.NewClient("test_key", stripe.WithBackends(backends)), publishKey: "pk_test"}
}

func writeJSON(w http.ResponseWriter, v map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func pmList(pms ...map[string]any) map[string]any {
	return map[string]any{"object": "list", "data": pms, "has_more": false, "url": "/v1/payment_methods"}
}

func cardPM(id, brand, last4 string) map[string]any {
	return map[string]any{
		"id":     id,
		"object": "payment_method",
		"type":   "card",
		"card":   map[string]any{"brand": brand, "last4": last4, "exp_month": 4, "exp_year": 2030},
	}
}

func TestNewStripe(t *testing.T) {
	if p := NewStripe(StripeConfig{SecretKey: ""}); p != nil {
		t.Errorf("NewStripe with no secret key = %v, want nil so provider == nil guards work", p)
	}

	p := NewStripe(StripeConfig{SecretKey: "sk_test", PublishableKey: "pk_test"})
	if p == nil {
		t.Fatal("NewStripe with a secret key returned nil")
	}
	if got := p.PublishableKey(); got != "pk_test" {
		t.Errorf("PublishableKey() = %q, want pk_test", got)
	}
}

func TestCardFromPM(t *testing.T) {
	pm := &stripe.PaymentMethod{
		ID:   "pm_1",
		Card: &stripe.PaymentMethodCard{Brand: "visa", Last4: "4242", ExpMonth: 4, ExpYear: 2030},
	}
	card := cardFromPM(pm)
	if card.ID != "pm_1" || card.Brand != "visa" || card.Last4 != "4242" || card.ExpMonth != 4 || card.ExpYear != 2030 {
		t.Errorf("cardFromPM(%+v) = %+v, want the Card fields to match", pm, card)
	}

	noCard := &stripe.PaymentMethod{ID: "pm_2"}
	got := cardFromPM(noCard)
	if got.ID != "pm_2" || got.Brand != "" {
		t.Errorf("cardFromPM with nil Card = %+v, want only ID set", got)
	}
}

func TestConfirmSetup_RejectsWrongCustomer(t *testing.T) {
	p := stripeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id":             "seti_1",
			"object":         "setup_intent",
			"status":         "succeeded",
			"customer":       map[string]any{"id": "cus_other", "object": "customer"},
			"payment_method": map[string]any{"id": "pm_new", "object": "payment_method"},
		})
	})

	card, err := p.ConfirmSetup(context.Background(), "cus_mine", "seti_1")
	if err == nil {
		t.Fatal("err is nil; a setup intent for a different customer must not be confirmed")
	}
	if card != nil {
		t.Errorf("card = %+v, want nil on a customer mismatch", card)
	}
}

func TestConfirmSetup_RejectsUnsucceededStatus(t *testing.T) {
	p := stripeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id":             "seti_1",
			"object":         "setup_intent",
			"status":         "requires_action",
			"customer":       map[string]any{"id": "cus_mine", "object": "customer"},
			"payment_method": map[string]any{"id": "pm_new", "object": "payment_method"},
		})
	})

	if _, err := p.ConfirmSetup(context.Background(), "cus_mine", "seti_1"); err == nil {
		t.Fatal("err is nil; a setup intent that has not succeeded must not be confirmed")
	}
}

func TestConfirmSetup_RejectsMissingPaymentMethod(t *testing.T) {
	p := stripeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id":       "seti_1",
			"object":   "setup_intent",
			"status":   "succeeded",
			"customer": map[string]any{"id": "cus_mine", "object": "customer"},
		})
	})

	if _, err := p.ConfirmSetup(context.Background(), "cus_mine", "seti_1"); err == nil {
		t.Fatal("err is nil; a succeeded setup intent with no payment method must not be confirmed")
	}
}

// detachCardsExcept is unexported and only reachable through this call, so
// it's covered here rather than directly.
func TestConfirmSetup_DetachesOldCardsAndReturnsTheNewOne(t *testing.T) {
	var detached []string
	p := stripeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/setup_intents/seti_1":
			writeJSON(w, map[string]any{
				"id":             "seti_1",
				"object":         "setup_intent",
				"status":         "succeeded",
				"customer":       map[string]any{"id": "cus_mine", "object": "customer"},
				"payment_method": map[string]any{"id": "pm_new", "object": "payment_method"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/payment_methods":
			writeJSON(w, pmList(cardPM("pm_old_1", "visa", "1111"), cardPM("pm_new", "mastercard", "4242")))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/payment_methods/pm_old_1/detach":
			detached = append(detached, "pm_old_1")
			writeJSON(w, cardPM("pm_old_1", "visa", "1111"))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/customers/cus_mine":
			writeJSON(w, map[string]any{"id": "cus_mine", "object": "customer"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/payment_methods/pm_new":
			writeJSON(w, cardPM("pm_new", "mastercard", "4242"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	card, err := p.ConfirmSetup(context.Background(), "cus_mine", "seti_1")
	if err != nil {
		t.Fatalf("ConfirmSetup() error = %v", err)
	}
	if card == nil || card.ID != "pm_new" || card.Brand != "mastercard" {
		t.Errorf("card = %+v, want the new card (pm_new/mastercard)", card)
	}
	if len(detached) != 1 || detached[0] != "pm_old_1" {
		t.Errorf("detached = %v, want exactly [pm_old_1]: the new card must be kept, not detached", detached)
	}
}

func TestCollectOpenInvoices_CountsDeclinesAsSkippedNotFailed(t *testing.T) {
	p := stripeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/invoices":
			writeJSON(w, map[string]any{
				"object": "list",
				"data": []map[string]any{
					{"id": "in_ok", "object": "invoice", "status": "open"},
					{"id": "in_declined", "object": "invoice", "status": "open"},
				},
				"has_more": false,
				"url":      "/v1/invoices",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/invoices/in_ok/pay":
			writeJSON(w, map[string]any{"id": "in_ok", "object": "invoice", "status": "paid"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/invoices/in_declined/pay":
			w.WriteHeader(http.StatusPaymentRequired)
			writeJSON(w, map[string]any{"error": map[string]any{"message": "card declined", "type": "card_error"}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	paid, err := p.CollectOpenInvoices(context.Background(), "cus_mine")
	if err != nil {
		t.Fatalf("CollectOpenInvoices() error = %v, want a declined invoice to be skipped, not to fail the batch", err)
	}
	if paid != 1 {
		t.Errorf("paid = %d, want 1 (only the non-declined invoice)", paid)
	}
}

func TestDefaultCard_NoCardOnFile(t *testing.T) {
	p := stripeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, pmList())
	})

	card, err := p.DefaultCard(context.Background(), "cus_mine")
	if err != nil {
		t.Fatalf("DefaultCard() error = %v", err)
	}
	if card != nil {
		t.Errorf("card = %+v, want nil when the customer has no saved card", card)
	}
}

func TestRemoveCard_DetachesEveryCard(t *testing.T) {
	var detached []string
	p := stripeProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/payment_methods":
			writeJSON(w, pmList(cardPM("pm_1", "visa", "1111"), cardPM("pm_2", "mastercard", "2222")))
		case r.Method == http.MethodPost:
			detached = append(detached, r.URL.Path)
			writeJSON(w, map[string]any{"id": "detached"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	if err := p.RemoveCard(context.Background(), "cus_mine"); err != nil {
		t.Fatalf("RemoveCard() error = %v", err)
	}
	want := []string{"/v1/payment_methods/pm_1/detach", "/v1/payment_methods/pm_2/detach"}
	if len(detached) != len(want) || detached[0] != want[0] || detached[1] != want[1] {
		t.Errorf("detached = %v, want %v (every saved card, none kept)", detached, want)
	}
}
