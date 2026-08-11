package metronome

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Metronome-Industries/metronome-go/v3"
	"github.com/Metronome-Industries/metronome-go/v3/option"
)

// spendProvider points a Provider at a stub Metronome. No retries, so a 500
// fails once instead of stalling the test.
func spendProvider(t *testing.T, h http.HandlerFunc) *Provider {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Provider{
		mc: metronome.NewClient(
			option.WithBearerToken("test"),
			option.WithBaseURL(srv.URL),
			option.WithMaxRetries(0),
		),
	}
}

const oneCreditBalance = `{"data":[{"id":"cred_1","balance":250}],"next_page":null}`

// Credit remaining is the number gating turns on, so a failing invoice
// endpoint must not take it away.
func TestCustomerSpend_KeepsCreditWhenInvoicesFail(t *testing.T) {
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "customerCredits") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(oneCreditBalance))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})

	spend, err := p.CustomerSpend(context.Background(), "cust_1")
	if err == nil {
		t.Fatal("err is nil; the invoice failure must still be reported")
	}
	if !spend.HasCredit || spend.CreditRemaining != 250 {
		t.Errorf("credit = %v (present %v), want 250 present", spend.CreditRemaining, spend.HasCredit)
	}
	if spend.HasCurrentSpend || spend.HasLastInvoice {
		t.Error("invoice figures are marked present after the lookup failed")
	}
}

// Failures are independent, so the error names all of them.
func TestCustomerSpend_ReportsEveryFailure(t *testing.T) {
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	spend, err := p.CustomerSpend(context.Background(), "cust_1")
	if err == nil {
		t.Fatal("err is nil")
	}
	for _, want := range []string{"credits", "DRAFT", "FINALIZED"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
	if spend.HasCredit || spend.HasCurrentSpend || spend.HasLastInvoice {
		t.Errorf("spend = %+v, want nothing marked present", spend)
	}
}

// A part-read page understates the balance, reading as closer to exhaustion
// than the account is.
func TestCustomerSpend_DropsPartialCreditPage(t *testing.T) {
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "customerCredits") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.URL.Query().Get("next_page") != "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"cred_1","balance":250}],"next_page":"more"}`))
	})

	spend, err := p.CustomerSpend(context.Background(), "cust_1")
	if err == nil {
		t.Fatal("err is nil")
	}
	if spend.HasCredit || spend.CreditRemaining != 0 {
		t.Errorf("credit = %v (present %v), want it dropped", spend.CreditRemaining, spend.HasCredit)
	}
}
