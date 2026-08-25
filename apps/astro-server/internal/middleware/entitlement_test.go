package middleware

import (
	"context"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// With no status store wired (OSS/noop), the gate is a pass-through even in
// enforce mode. Balance-driven blocking is exercised by the state-machine tests
// in internal/billing (computeStatus) — the gate here only reads the cached row.

func TestEntitlement_WrapPassesThroughWhenNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ent := NewEntitlements(nil, true, nil)
	router := gin.New()
	router.GET("/test", ent.Wrap(func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEntitlement_AllowsWhenNoStore(t *testing.T) {
	ent := NewEntitlements(nil, true, nil)
	if ent.Blocked(context.Background(), "acct-1") {
		t.Error("expected not blocked with no status store")
	}
}

// The body is the only thing a CLI or a banner has to work from, so every
// reason must resolve to the action that actually fixes it. Telling an account
// with no card on file to update one is the bug this table prevents.
func TestPaymentRequiredResponse_NamesTheFixForEveryReason(t *testing.T) {
	cases := map[string]string{
		billing.ReasonCreditsExhausted: ActionAddCard,
		billing.ReasonDunning:          ActionUpdateCard,
		billing.ReasonPaymentFailed:    ActionUpdateCard,
		billing.ReasonUncollectible:    ActionUpdateCard,
		billing.ReasonBalanceAlert:     ActionContactSupport,
		// A reason this build predates must not send the owner to change a card
		// that may be fine, so it lands on the one non-self-serve answer.
		"reason_from_a_newer_server": ActionContactSupport,
		"":                           ActionContactSupport,
	}
	for reason, wantAction := range cases {
		resp := PaymentRequiredResponse(reason, "")
		if resp["code"] != "BILLING_SUSPENDED" {
			t.Errorf("%s: code = %v, want BILLING_SUSPENDED", reason, resp["code"])
		}
		if resp["reason"] != reason {
			t.Errorf("%s: reason = %v, want it echoed back", reason, resp["reason"])
		}
		if resp["action"] != wantAction {
			t.Errorf("%s: action = %v, want %s", reason, resp["action"], wantAction)
		}
		// Clients that render the body verbatim, which the CLI does today, must
		// not be handed an empty line.
		if d, _ := resp["details"].(string); d == "" {
			t.Errorf("%s: details is empty", reason)
		}
	}
}

// The gate must carry the reason, not just the verdict. A refusal that cannot
// name the fix is the reason the CLI could only print a JSON envelope.
func TestEntitlementsCheck_CarriesTheReasonWhenBlocking(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT status, reason").
		WithArgs("acct-1").
		WillReturnRows(statusRow("suspended", billing.ReasonCreditsExhausted, nil))

	got := NewEntitlements(billing.NewStatusStore(db, 7), true, nil).Check(context.Background(), "acct-1")
	if !got.Blocked {
		t.Fatal("Blocked = false, want true for a suspended account")
	}
	if got.Reason != billing.ReasonCreditsExhausted {
		t.Errorf("Reason = %q, want %q", got.Reason, billing.ReasonCreditsExhausted)
	}
}

func statusRow(status, reason string, payLink any) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"status", "reason", "dunning_since", "alert_active",
		"force_suspended", "credits_exhausted", "has_payment_method", "pay_link",
		"usage_limit_active", "not_provisioned",
	}).AddRow(status, reason, nil, false, false, false, true, payLink, false, false)
}

// A bank asking for authentication is not a broken card. The gate has to hand
// back the hosted page, because sending the customer to replace a working card
// leaves the charge waiting and the account stopped.
func TestEntitlementsCheck_CarriesThePayLinkForAnUnauthenticatedCharge(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	const link = "https://invoice.stripe.com/i/acct_1/test"
	mock.ExpectQuery("SELECT status, reason").
		WithArgs("acct-1").
		WillReturnRows(statusRow("suspended", billing.ReasonPaymentFailed, link))

	got := NewEntitlements(billing.NewStatusStore(db, 7), true, nil).Check(context.Background(), "acct-1")
	if got.PayLink != link {
		t.Errorf("PayLink = %q, want the hosted page", got.PayLink)
	}
	body := PaymentRequiredResponse(got.Reason, got.PayLink)
	if body["action"] != ActionCompletePayment {
		t.Errorf("action = %v, want %s", body["action"], ActionCompletePayment)
	}
	if body["pay_link"] != link {
		t.Errorf("pay_link = %v, want it in the 402 body", body["pay_link"])
	}
}

// Without a link the same reason still means the card needs replacing, or the
// banner would offer a button that goes nowhere.
func TestBillingAction_NoPayLinkKeepsUpdateCard(t *testing.T) {
	if got := BillingAction(billing.ReasonPaymentFailed, false); got != ActionUpdateCard {
		t.Errorf("action = %q, want %q", got, ActionUpdateCard)
	}
	if got := BillingAction(billing.ReasonCreditsExhausted, true); got != ActionAddCard {
		t.Errorf("action = %q, want %q: a pay link cannot fix spent credits", got, ActionAddCard)
	}
}

// Observe mode exists so the gate can be switched on without stopping anyone.
// It must allow a suspended account and report no reason, or a caller that
// branches on Reason would surface a block that never happened.
func TestEntitlementsCheck_ObserveModeAllowsASuspendedAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT status, reason FROM account_billing_status").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "reason"}).
			AddRow("suspended", billing.ReasonPaymentFailed))

	got := NewEntitlements(billing.NewStatusStore(db, 7), false, nil).Check(context.Background(), "acct-1")
	if got.Blocked || got.Reason != "" {
		t.Errorf("got %+v, want an empty decision in observe mode", got)
	}
}

// Failing open is deliberate: a database blip must not stop a paying customer.
func TestEntitlementsCheck_AllowsWhenTheStatusReadFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT status, reason FROM account_billing_status").
		WillReturnError(context.DeadlineExceeded)

	if got := NewEntitlements(billing.NewStatusStore(db, 7), true, nil).Check(context.Background(), "acct-1"); got.Blocked {
		t.Error("Blocked = true on a read error, want fail-open")
	}
}

// A write-off keeps whatever pay link the earlier authentication attempt left
// behind, because SignalUncollectible clears neither dunning nor the link. The
// link must not become the offered fix: only a void or an operator lifts
// force_suspended, so paying it takes the money and leaves the account stopped.
func TestBillingAction_WriteOffNeverOffersThePayLink(t *testing.T) {
	if got := BillingAction(billing.ReasonUncollectible, true); got != ActionUpdateCard {
		t.Errorf("action = %q, want %q: a paid link cannot lift a write-off", got, ActionUpdateCard)
	}
	body := PaymentRequiredResponse(billing.ReasonUncollectible, "https://invoice.stripe.com/i/acct_1/test")
	if _, ok := body["pay_link"]; ok {
		t.Error("the 402 body carries a pay link for a written-off invoice")
	}
}

// The gate reads Record, not Get, so an exemption honoured only in Get would
// still answer 402 here. This is the assertion that catches that.
func TestEntitlementsCheck_ExemptAccountIsNeverBlocked(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	row := func() *sqlmock.Rows {
		return statusRow("suspended", billing.ReasonCreditsExhausted, nil)
	}

	// The same suspended row through a store that does not exempt the account,
	// so a gate that never blocks anything cannot pass this test.
	mock.ExpectQuery("SELECT status, reason").WithArgs("acct-1").WillReturnRows(row())
	if got := NewEntitlements(billing.NewStatusStore(db, 7), true, nil).
		Check(context.Background(), "acct-1"); !got.Blocked {
		t.Fatal("Blocked = false without the exemption, so the case proves nothing")
	}

	mock.ExpectQuery("SELECT status, reason").WithArgs("acct-1").WillReturnRows(row())
	exempt := billing.NewStatusStore(db, 7).WithExemptAccounts([]string{"acct-1"})
	got := NewEntitlements(exempt, true, nil).Check(context.Background(), "acct-1")
	if got.Blocked {
		t.Errorf("an exempt account was blocked: reason %q", got.Reason)
	}
	if got.Reason != "" {
		t.Errorf("Reason = %q, want empty", got.Reason)
	}
}
