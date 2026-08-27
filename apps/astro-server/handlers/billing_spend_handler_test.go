package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

type spendFakeProvider struct {
	billing.BillingProvider
	spend         billing.Spend
	spendErr      error
	plan          billing.Plan
	covered       bool
	thresholdErr  error
	warningAmount float64
	hasWarning    bool
}

func (f *spendFakeProvider) CreateCustomer(context.Context, billing.Account) (string, error) {
	return "cust-1", nil
}

func (f *spendFakeProvider) CustomerSpend(context.Context, string) (billing.Spend, error) {
	return f.spend, f.spendErr
}

type planReportingProvider struct {
	*spendFakeProvider
}

func (f *planReportingProvider) CustomerPlan(context.Context, string) (billing.Plan, bool, error) {
	return f.plan, f.covered, nil
}

type thresholdReadingProvider struct {
	*spendFakeProvider
}

func (f *thresholdReadingProvider) CustomerSpendThresholds(context.Context, string) (billing.SpendThresholds, error) {
	if f.thresholdErr != nil {
		return billing.SpendThresholds{}, f.thresholdErr
	}
	return billing.SpendThresholds{HasWarning: f.hasWarning, Warning: billing.SpendThreshold{Amount: f.warningAmount}}, nil
}

func spendRequest(t *testing.T, provider billing.BillingProvider) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "acme", Type: "personal"})

	var store *account.AccountStore
	GetBillingSpend(logger.New("error", "json"), store, provider, config.BillingBackendFake)(c)
	return rec
}

func TestGetBillingSpend_ReturnsTheSpendFields(t *testing.T) {
	rec := spendRequest(t, &spendFakeProvider{
		spend: billing.Spend{Currency: "USD", CurrentSpend: 12.5, HasCurrentSpend: true, HasCredit: true, CreditRemaining: 7.5},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Available bool                 `json:"available"`
		Data      BillingSpendResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal %s: %v", rec.Body.String(), err)
	}
	if !resp.Available {
		t.Fatal("available = false, want true")
	}
	if resp.Data.Currency != "USD" || resp.Data.CurrentSpend != 12.5 || !resp.Data.HasCurrentSpend {
		t.Errorf("data = %+v, want the fake's spend fields", resp.Data)
	}
	if !resp.Data.HasCredit || resp.Data.CreditRemaining != 7.5 {
		t.Errorf("data = %+v, want the fake's credit fields", resp.Data)
	}
}

func TestGetBillingSpend_UncoveredContractFailsTheRequest(t *testing.T) {
	rec := spendRequest(t, &planReportingProvider{
		spendFakeProvider: &spendFakeProvider{spend: billing.Spend{Currency: "USD"}, covered: false},
	})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s, want 500 (no billing contract covers this account)", rec.Code, rec.Body.String())
	}
}

func TestGetBillingSpend_CoveredContractSucceeds(t *testing.T) {
	rec := spendRequest(t, &planReportingProvider{
		spendFakeProvider: &spendFakeProvider{spend: billing.Spend{Currency: "USD"}, plan: "growth", covered: true},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data BillingSpendResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal %s: %v", rec.Body.String(), err)
	}
	if resp.Data.Plan != "growth" {
		t.Errorf("plan = %q, want growth", resp.Data.Plan)
	}
}

func TestGetBillingSpend_ThresholdReadFailureDoesNotHideSpend(t *testing.T) {
	rec := spendRequest(t, &thresholdReadingProvider{
		spendFakeProvider: &spendFakeProvider{
			spend:        billing.Spend{Currency: "USD", CurrentSpend: 42, HasCurrentSpend: true},
			thresholdErr: context.DeadlineExceeded,
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s, want the spend to still be returned", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data BillingSpendResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal %s: %v", rec.Body.String(), err)
	}
	if resp.Data.CurrentSpend != 42 {
		t.Errorf("current_spend = %v, want 42 despite the threshold read failing", resp.Data.CurrentSpend)
	}
	if resp.Data.Warning != nil {
		t.Errorf("warning = %+v, want nil: the read failed, so nothing should be reported", resp.Data.Warning)
	}
}
