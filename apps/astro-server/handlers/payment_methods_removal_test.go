package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/payment"
)

// recordingPayments reports whether the card was actually detached, which is the
// only thing a refused removal has to get right.
type recordingPayments struct {
	removed bool
}

func (p *recordingPayments) card() *payment.Card {
	return &payment.Card{ID: "pm_1", Brand: "Visa", Last4: "4242"}
}

func (p *recordingPayments) CreateCustomer(context.Context, string, string, string) (string, error) {
	return "cus_1", nil
}
func (p *recordingPayments) CreateSetupIntent(context.Context, string) (string, error) {
	return "seti_secret", nil
}
func (p *recordingPayments) ConfirmSetup(context.Context, string, string) (*payment.Card, error) {
	return p.card(), nil
}
func (p *recordingPayments) DefaultCard(context.Context, string) (*payment.Card, error) {
	return p.card(), nil
}
func (p *recordingPayments) RemoveCard(context.Context, string) error {
	p.removed = true
	return nil
}
func (p *recordingPayments) PublishableKey() string { return "pk_test" }

// removalRouter supplies the resolved account the real middleware would.
func removalRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Name: "acme"})
		c.Next()
	})
	return r
}

func removalRequest(t *testing.T, r *gin.Engine) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/billing/payment-method", nil))
	return rec
}

func expectRunningDeployments(mock sqlmock.Sqlmock, running int) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM deployments`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(running))
}

// expectCustomerIDs queues the two reads the handler makes before it decides:
// the Stripe id it detaches against, and the billing id it prices against.
func expectCustomerIDs(mock sqlmock.Sqlmock, billingID string) {
	mock.ExpectQuery("SELECT stripe_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"stripe_customer_id"}).AddRow("cus_1"))
	expectRunningDeployments(mock, 0)
	mock.ExpectQuery("SELECT metronome_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow(billingID))
}

func newRemovalTest(t *testing.T, provider billing.BillingProvider) (*gin.Engine, *recordingPayments, sqlmock.Sqlmock, func()) {
	return newRemovalTestWithAudit(t, provider, nil)
}

func newRemovalTestWithAudit(t *testing.T, provider billing.BillingProvider, auditStore *auditlog.Store) (*gin.Engine, *recordingPayments, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	pay := &recordingPayments{}
	r := removalRouter(t)
	r.DELETE("/billing/payment-method", DeletePaymentMethod(
		logger.New("error", "json"), account.NewAccountStore(db), deploymentstore.NewStore(db), auditStore, pay, provider, "metronome", nil, nil))
	return r, pay, mock, func() { db.Close() }
}

// Removing the card is the cheaper way out of a bill than deleting the account:
// the spend stops either way, but the accrued draft is never charged.
func TestDeletePaymentMethod_OutstandingBalanceRefusesAndKeepsTheCard(t *testing.T) {
	provider := spendingProvider{spend: billing.Spend{HasCurrentSpend: true, CurrentSpend: 1000}}
	r, pay, mock, done := newRemovalTest(t, provider)
	defer done()
	expectCustomerIDs(mock, "cust-1")

	rec := removalRequest(t, r)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if pay.removed {
		t.Error("the card was detached anyway, so the balance became uncollectable")
	}
}

// An account inside its credit grant owes nothing, so removal is a downgrade
// rather than an escape.
func TestDeletePaymentMethod_NothingOwedRemovesTheCard(t *testing.T) {
	provider := spendingProvider{spend: billing.Spend{HasCurrentSpend: true, CurrentSpend: 0}}
	r, pay, mock, done := newRemovalTest(t, provider)
	defer done()
	expectCustomerIDs(mock, "cust-1")

	rec := removalRequest(t, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !pay.removed {
		t.Error("the card survived a removal that nothing owed blocks")
	}
}

// An unreadable balance is not permission to detach. Failing closed keeps the
// card attached, which is recoverable; detaching is not.
func TestDeletePaymentMethod_UnreadableBalanceKeepsTheCard(t *testing.T) {
	provider := spendingProvider{err: errors.New("provider down")}
	r, pay, mock, done := newRemovalTest(t, provider)
	defer done()
	expectCustomerIDs(mock, "cust-1")

	rec := removalRequest(t, r)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body.String())
	}
	if pay.removed {
		t.Error("the card was detached on an unreadable balance")
	}
}

// Without a billing provider there is no balance to read and nothing to bill,
// so removal stays available.
func TestDeletePaymentMethod_NoBillingProviderStillRemoves(t *testing.T) {
	r, pay, mock, done := newRemovalTest(t, nil)
	defer done()
	mock.ExpectQuery("SELECT stripe_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"stripe_customer_id"}).AddRow("cus_1"))
	expectRunningDeployments(mock, 0)

	rec := removalRequest(t, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !pay.removed {
		t.Error("the card survived with no billing provider configured")
	}
}

// 0.9846 cents renders as "$0.01" and settles as one cent, so it is owed.
func TestDeletePaymentMethod_SubCentBalanceThatRoundsToACentRefuses(t *testing.T) {
	provider := spendingProvider{spend: billing.Spend{HasCurrentSpend: true, CurrentSpend: 0.009846}}
	r, pay, mock, done := newRemovalTest(t, provider)
	defer done()
	expectCustomerIDs(mock, "cust-1")

	rec := removalRequest(t, r)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: a balance shown as $0.01 is owed: %s", rec.Code, rec.Body.String())
	}
	if pay.removed {
		t.Error("the card was detached over a balance the customer is shown as owing")
	}
}

// Below half a cent nothing rounds up, so it can never be charged.
func TestDeletePaymentMethod_DustBelowHalfACentStillRemoves(t *testing.T) {
	provider := spendingProvider{spend: billing.Spend{HasCurrentSpend: true, CurrentSpend: 0.004}}
	r, pay, mock, done := newRemovalTest(t, provider)
	defer done()
	expectCustomerIDs(mock, "cust-1")

	rec := removalRequest(t, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !pay.removed {
		t.Error("dust that rounds to $0.00 blocked the removal")
	}
}

// The balance can still read zero while a running agent accrues real charges.
func TestDeletePaymentMethod_RunningDeploymentRefusesEvenWithNothingBilledYet(t *testing.T) {
	provider := spendingProvider{spend: billing.Spend{HasCurrentSpend: true, CurrentSpend: 0}}
	r, pay, mock, done := newRemovalTest(t, provider)
	defer done()
	mock.ExpectQuery("SELECT stripe_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"stripe_customer_id"}).AddRow("cus_1"))
	expectRunningDeployments(mock, 1)
	// Queued so a handler ignoring the count reaches a zero balance and a 200,
	// rather than failing on an unqueued query.
	mock.ExpectQuery("SELECT metronome_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))

	rec := removalRequest(t, r)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if pay.removed {
		t.Error("the card came off while an agent was still running up a bill")
	}
}

// Pausing every agent is the way out, so a paused account must not be trapped.
func TestDeletePaymentMethod_NoRunningDeploymentsRemoves(t *testing.T) {
	provider := spendingProvider{spend: billing.Spend{HasCurrentSpend: true, CurrentSpend: 0}}
	r, pay, mock, done := newRemovalTest(t, provider)
	defer done()
	expectCustomerIDs(mock, "cust-1")

	rec := removalRequest(t, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !pay.removed {
		t.Error("a paused account could not remove its card")
	}
}
