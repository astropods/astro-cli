package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/accountlifecycle"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/billing/noop"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// spendingProvider is a provider that also reports spend. The embedded
// interface supplies every other method, so the test states only the fact under
// test.
type spendingProvider struct {
	billing.BillingProvider
	spend billing.Spend
	err   error
}

func (p spendingProvider) CustomerSpend(_ context.Context, _ string) (billing.Spend, error) {
	return p.spend, p.err
}

func TestOutstandingBalance_DraftTotalBlocksTheDelete(t *testing.T) {
	p := spendingProvider{spend: billing.Spend{HasCurrentSpend: true, CurrentSpend: 19.53}}
	owed, err := outstandingBalance(context.Background(), p, nil, "acct-1", "cust-1")
	if err != nil {
		t.Fatalf("outstandingBalance: %v", err)
	}
	if !owed {
		t.Error("usage run up in the open period is money the archive would void")
	}
}

func TestOutstandingBalance_CreditCoveredSpendDoesNotBlock(t *testing.T) {
	// The draft total is net of credit drawdown, so an account still inside its
	// grant reads zero and owes nothing.
	p := spendingProvider{spend: billing.Spend{HasCurrentSpend: true, CurrentSpend: 0, CreditRemaining: 4.21, HasCredit: true}}
	owed, err := outstandingBalance(context.Background(), p, nil, "acct-1", "cust-1")
	if err != nil {
		t.Fatalf("outstandingBalance: %v", err)
	}
	if owed {
		t.Error("spend covered by credit is not a balance")
	}
}

func TestOutstandingBalance_ProviderWithoutSpendReporting(t *testing.T) {
	owed, err := outstandingBalance(context.Background(), noop.New(), nil, "acct-1", "cust-1")
	if err != nil {
		t.Fatalf("outstandingBalance: %v", err)
	}
	if owed {
		t.Error("a provider that reports no money must not block deletes")
	}
}

func TestOutstandingBalance_ReadFailureIsAnError(t *testing.T) {
	// Reported rather than swallowed: treating a failed read as "owes nothing"
	// would reopen the escape whenever the provider is down.
	p := spendingProvider{err: errors.New("provider unavailable")}
	if _, err := outstandingBalance(context.Background(), p, nil, "acct-1", "cust-1"); err == nil {
		t.Error("expected the provider error to surface")
	}
}

func TestOutstandingBalance_DunningBlocksWithNoDraftTotal(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("FROM account_billing_status").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"status", "reason", "dunning_since", "alert_active", "force_suspended",
			"credits_exhausted", "has_payment_method", "pay_link", "usage_limit_active", "not_provisioned",
		}).AddRow("past_due", "payment_failed", time.Now().Add(-48*time.Hour), false, false, false, true, nil, false, false))

	// A finalized invoice that failed to collect is in a closed period, so the
	// open draft says nothing about it.
	p := spendingProvider{spend: billing.Spend{HasCurrentSpend: true, CurrentSpend: 0}}
	owed, err := outstandingBalance(context.Background(), p, billing.NewStatusStore(db, 7), "acct-1", "cust-1")
	if err != nil {
		t.Fatalf("outstandingBalance: %v", err)
	}
	if !owed {
		t.Error("an account in dunning owes a finalized invoice")
	}
}

// The gate moved out of the delete sequence and into the handler when main
// factored the sequence into accountlifecycle.Deleter. Nothing else asserts the
// refusal reaches the wire, so a merge that dropped the gate, or ran it after
// the delete, would have gone unnoticed.
func TestDeleteAccount_OutstandingBalanceRefusesBeforeDeleting(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "acme"})
		c.Next()
	})
	accountStore := account.NewAccountStore(db)
	// A Deleter with no collaborators: reaching it at all is the failure this
	// test is looking for, and MarkDeleted would be its first write.
	deleter := &accountlifecycle.Deleter{Log: logger.New("error", "json"), Accounts: accountStore}
	provider := spendingProvider{spend: billing.Spend{HasCurrentSpend: true, CurrentSpend: 12.34}}
	r.DELETE("/accounts/:account", DeleteAccount(
		logger.New("error", "json"), deleter, nil, accountStore, provider, nil, "metronome"))

	mock.ExpectQuery("SELECT metronome_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/accounts/acct-1", nil))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	// No UPDATE was queued, so the soft-delete never ran.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("the delete proceeded past the gate: %v", err)
	}
}
