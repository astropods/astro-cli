package metronome

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Metronome-Industries/metronome-go/v3"
	"github.com/Metronome-Industries/metronome-go/v3/option"
	"github.com/Metronome-Industries/metronome-go/v3/shared"
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

// A contract credit of 250 in Metronome's cents credit type is $2.50.
const oneCredit = `{"data":[{"id":"cred_1","type":"CREDIT","name":"Signup credit",` +
	`"balance":250,"access_schedule":{"credit_type":` +
	`{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"}}}],"next_page":null}`

// Credit remaining is the number gating turns on, so a failing invoice
// endpoint must not take it away.
func TestCustomerSpend_KeepsCreditWhenInvoicesFail(t *testing.T) {
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "customerCredits") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(oneCredit))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})

	spend, err := p.CustomerSpend(context.Background(), "cust_1")
	if err == nil {
		t.Fatal("err is nil; the invoice failure must still be reported")
	}
	if !spend.HasCredit || spend.CreditRemaining != 2.5 {
		t.Errorf("credit = %v (present %v), want 2.5 present", spend.CreditRemaining, spend.HasCredit)
	}
	// The cents unit is converted here, not in the UI, so what ships is already
	// the unit Currency names.
	if spend.Currency != "USD" {
		t.Errorf("currency = %q, want USD", spend.Currency)
	}
	if spend.HasCurrentSpend || spend.HasLastInvoice {
		t.Error("invoice figures are marked present after the lookup failed")
	}
}

// A credit type this system does not convert must pass through untouched
// rather than be silently divided.
func TestScaleAmount(t *testing.T) {
	for _, tc := range []struct {
		name       string
		creditType shared.CreditTypeData
		want       float64
		wantUnit   string
	}{
		{"cents", shared.CreditTypeData{ID: usdCentsCreditTypeID, Name: "USD (cents)"}, 2.5, "USD"},
		// Metronome is free to reword a display name, so the id decides.
		{"cents renamed", shared.CreditTypeData{ID: usdCentsCreditTypeID, Name: "US Dollars (cents)"}, 2.5, "USD"},
		{"custom unit", shared.CreditTypeData{ID: "other", Name: "AI Credits"}, 250, "AI Credits"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, unit := scaleAmount(250, tc.creditType)
			if got != tc.want || unit != tc.wantUnit {
				t.Errorf("scaleAmount(250, %q) = (%v, %q), want (%v, %q)", tc.creditType.ID, got, unit, tc.want, tc.wantUnit)
			}
		})
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
		_, _ = w.Write([]byte(strings.Replace(oneCredit, `"next_page":null`, `"next_page":"more"`, 1)))
	})

	spend, err := p.CustomerSpend(context.Background(), "cust_1")
	if err == nil {
		t.Fatal("err is nil")
	}
	if spend.HasCredit || spend.CreditRemaining != 0 {
		t.Errorf("credit = %v (present %v), want it dropped", spend.CreditRemaining, spend.HasCredit)
	}
}

// The package carries the signup credit, so provisioning must not write one
// itself: a second credit would double what a new account is given.
func TestProvisionCustomer_CreatesOnlyTheContract(t *testing.T) {
	var paths []string
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "v2/contracts/list"):
			_, _ = w.Write([]byte(`{"data":[],"next_page":null}`))
		default:
			_, _ = w.Write([]byte(`{"data":{"contract":{"id":"con_1"}}}`))
		}
	})
	p.cfg = Config{PackageID: "pkg_1"}

	provisioned, err := p.ProvisionCustomer(context.Background(), "cust_1", "acct_1", true)
	if err != nil || !provisioned {
		t.Fatalf("ProvisionCustomer = (%v, %v), want (true, nil)", provisioned, err)
	}
	for _, path := range paths {
		if strings.Contains(path, "credits") {
			t.Errorf("provisioning called %s; the package already carries the credit", path)
		}
	}
	if len(paths) != 2 {
		t.Errorf("calls = %v, want the contract list and the contract create", paths)
	}
}

// A customer already covered is left alone, so no contract and no second credit.
func TestProvisionCustomer_SkipsWhenCovered(t *testing.T) {
	var paths []string
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"con_existing"}],"next_page":null}`))
	})
	p.cfg = Config{PackageID: "pkg_1"}

	provisioned, err := p.ProvisionCustomer(context.Background(), "cust_1", "acct_1", true)
	if err != nil || !provisioned {
		t.Fatalf("ProvisionCustomer = (%v, %v), want (true, nil)", provisioned, err)
	}
	if len(paths) != 1 {
		t.Errorf("calls = %v, want only the contract list", paths)
	}
}

// The provider's spend threshold notification measures usage before credit
// drawdown. Reporting the invoice total instead reads zero for any account whose
// credit still covers it, and that account can still cross its own warning: the
// number a customer sets a threshold against would not be the number shown.
func TestUsageSpendExcludesCreditDrawdown(t *testing.T) {
	usd := shared.CreditTypeData{ID: usdCentsCreditTypeID, Name: "USD (cents)"}
	inv := &metronome.Invoice{
		Total: 0,
		LineItems: []metronome.InvoiceLineItem{
			{Name: "Compute Units", Type: "usage", Total: 262.32, CreditType: usd},
			{Name: "Signup credit applied", Type: "applied_commit_or_credit", Total: -262.32, CreditType: usd},
			{Name: "AI Gateway", Type: "usage", Total: 13.56, CreditType: usd},
			{Name: "Signup credit applied", Type: "applied_commit_or_credit", Total: -13.56, CreditType: usd},
		},
	}

	got, ok := usageSpend(inv)
	if !ok {
		t.Fatal("an invoice with usage lines reported none")
	}
	if want := 2.7588; got < want-0.0001 || got > want+0.0001 {
		t.Errorf("usageSpend = %v, want %v: the credit lines were counted", got, want)
	}
}

// An invoice carrying only credit or scheduled lines has no usage to report, and
// reporting zero as a fact would claim the account spent nothing.
func TestUsageSpendReportsAbsence(t *testing.T) {
	inv := &metronome.Invoice{LineItems: []metronome.InvoiceLineItem{
		{Name: "Test charge", Type: "scheduled", Total: 100},
	}}
	if got, ok := usageSpend(inv); ok {
		t.Errorf("usageSpend = %v, ok = true; want no usage reported", got)
	}
}
