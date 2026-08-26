package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/billing/fake"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// resolveBillingCustomer used to gate on the literal backend name, so any
// backend other than metronome reported unavailable and the page rendered its
// "billing isn't available" state with the provider never consulted. The fake
// backend passes a nil account store on purpose: it derives its customer id, so
// reaching the store at all means it is back on the persisted path.
func fakeBackendRequest(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(string(auth.AccountContextKey), &account.Account{
		ID:   "acct-1",
		Name: "matt",
		Type: "personal",
	})

	var store *account.AccountStore
	GetBillingInvoices(logger.New("error", "test"), store, fake.New(), config.BillingBackendFake)(c)
	return rec
}

func TestFakeBackendReportsBillingAvailable(t *testing.T) {
	rec := fakeBackendRequest(t)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp struct {
		Available bool `json:"available"`
		Data      []struct {
			Status          string `json:"status"`
			ExternalInvoice struct {
				ExternalStatus string `json:"external_status"`
			} `json:"external_invoice"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal %s: %v", rec.Body.String(), err)
	}
	if !resp.Available {
		t.Fatal("billing reports unavailable, so every section renders its unavailable state")
	}
	if len(resp.Data) == 0 {
		t.Fatal("no invoices reached the client")
	}

	// The point of the fake is the spread of outcomes, so a response carrying
	// one shape is as good as none.
	outcomes := map[string]bool{}
	for _, inv := range resp.Data {
		outcomes[inv.ExternalInvoice.ExternalStatus] = true
	}
	for _, want := range []string{"PAID", "PAYMENT_FAILED", ""} {
		if !outcomes[want] {
			t.Errorf("no invoice with external status %q reached the client", want)
		}
	}
}

// Noop keeps no customer records, so it has to keep reporting unavailable.
// Widening the gate for the fake must not widen it for noop.
func TestNoopBackendStillReportsUnavailable(t *testing.T) {
	if config.BillingBackendHasCustomers(config.BillingBackendNoop) {
		t.Error("noop reports it keeps customer records, so billing reads would try to provision one")
	}
	if !config.BillingBackendHasCustomers(config.BillingBackendMetronome) {
		t.Error("metronome reports it keeps no customer records, which disables hosted billing")
	}
	if !config.BillingBackendHasCustomers(config.BillingBackendFake) {
		t.Error("fake reports it keeps no customer records, so its reads never run")
	}
}
