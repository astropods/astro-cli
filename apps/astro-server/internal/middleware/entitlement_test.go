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
		resp := PaymentRequiredResponse(reason)
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

	mock.ExpectQuery("SELECT status, reason FROM account_billing_status").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "reason"}).
			AddRow("suspended", billing.ReasonCreditsExhausted))

	got := NewEntitlements(billing.NewStatusStore(db, 7), true, nil).Check(context.Background(), "acct-1")
	if !got.Blocked {
		t.Fatal("Blocked = false, want true for a suspended account")
	}
	if got.Reason != billing.ReasonCreditsExhausted {
		t.Errorf("Reason = %q, want %q", got.Reason, billing.ReasonCreditsExhausted)
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
