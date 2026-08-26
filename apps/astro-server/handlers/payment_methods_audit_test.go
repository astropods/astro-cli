package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/payment"
)

func auditContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	return c
}

// The entry has to name the card without holding anything chargeable.
func TestCardAuditEvent_NamesTheCardByBrandAndLastFour(t *testing.T) {
	card := &payment.Card{ID: "pm_1", Brand: "Visa", Last4: "4242", ExpMonth: 12, ExpYear: 2030}
	evt := cardAuditEvent(auditContext(t), "acct-1", auditlog.PaymentMethodRemove, "Removed payment method", card)

	if evt.Action != "payment_method.remove" {
		t.Errorf("action = %q", evt.Action)
	}
	if evt.ResourceType != "payment_method" {
		t.Errorf("resource type = %q", evt.ResourceType)
	}
	if evt.Description != "Removed payment method Visa ending 4242" {
		t.Errorf("description = %q", evt.Description)
	}
	meta, ok := evt.Metadata.(map[string]any)
	if !ok {
		t.Fatalf("metadata = %T, want a map", evt.Metadata)
	}
	if meta["last4"] != "4242" || meta["brand"] != "Visa" {
		t.Errorf("metadata = %v, want the brand and last four", meta)
	}

	// The expiry identifies nothing a reader needs and is card data we have no
	// reason to keep in a second place.
	encoded, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, leaked := range []string{"2030", "exp"} {
		if strings.Contains(string(encoded), leaked) {
			t.Errorf("event carries %q: %s", leaked, encoded)
		}
	}
}

// A gap in the trail looks the same as nothing having happened.
func TestCardAuditEvent_UnreadableCardStillRecordsTheAction(t *testing.T) {
	evt := cardAuditEvent(auditContext(t), "acct-1", auditlog.PaymentMethodRemove, "Removed payment method", nil)

	if evt.Action != "payment_method.remove" {
		t.Errorf("action = %q", evt.Action)
	}
	if evt.Description != "Removed payment method" {
		t.Errorf("description = %q", evt.Description)
	}
	if evt.Metadata != nil {
		t.Errorf("metadata = %v, want none for a card nobody could read", evt.Metadata)
	}
}

// A provider that answers with an empty card must not produce "Visa ending "
// with nothing after it.
func TestCardAuditEvent_BlankCardDetailsLeaveTheDescriptionAlone(t *testing.T) {
	evt := cardAuditEvent(auditContext(t), "acct-1", auditlog.PaymentMethodAdd, "Added payment method", &payment.Card{ID: "pm_2"})

	if evt.Description != "Added payment method" {
		t.Errorf("description = %q, want no dangling label", evt.Description)
	}
	if evt.ResourceName != "" {
		t.Errorf("resource name = %q", evt.ResourceName)
	}
}

// auditWaiter signals once an entry is durably written, which is the only
// moment a LogAsync goroutine is observable from a test.
type auditWaiter struct{ written chan auditlog.Event }

func (w *auditWaiter) OnAudit(_ context.Context, e auditlog.Event) { w.written <- e }

// The handler has to reach the store, not merely be able to build an event.
func TestDeletePaymentMethod_WritesAnAuditEntry(t *testing.T) {
	waiter := &auditWaiter{written: make(chan auditlog.Event, 1)}
	provider := spendingProvider{spend: billing.Spend{HasCurrentSpend: true, CurrentSpend: 0}}

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	store := auditlog.NewStore(db).Observe(waiter)

	r, pay, mock2, done := newRemovalTestWithAudit(t, provider, store)
	defer done()
	expectCustomerIDs(mock2, "cust-1")
	mock.ExpectExec("INSERT INTO audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))

	rec := removalRequest(t, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !pay.removed {
		t.Fatal("the card was not removed")
	}
	select {
	case evt := <-waiter.written:
		if evt.Action != auditlog.PaymentMethodRemove {
			t.Errorf("action = %q, want %q", evt.Action, auditlog.PaymentMethodRemove)
		}
		if evt.AccountID != "acct-1" {
			t.Errorf("account = %q", evt.AccountID)
		}
		if evt.Description != "Removed payment method Visa ending 4242" {
			t.Errorf("description = %q, want the card named", evt.Description)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no audit entry was written, so the removal left no trail")
	}
}

// Saving a card is the other half of the trail, and it has its own call site.
func TestConfirmPaymentMethod_WritesAnAuditEntry(t *testing.T) {
	waiter := &auditWaiter{written: make(chan auditlog.Event, 1)}

	auditDB, auditMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer auditDB.Close() //nolint:errcheck
	auditMock.ExpectExec("INSERT INTO audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck
	mock.ExpectQuery("SELECT stripe_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"stripe_customer_id"}).AddRow("cus_1"))

	pay := &recordingPayments{}
	r := removalRouter(t)
	r.POST("/billing/payment-method", ConfirmPaymentMethod(
		logger.New("error", "json"), account.NewAccountStore(db), auditlog.NewStore(auditDB).Observe(waiter),
		pay, nil, "metronome", nil, nil))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/billing/payment-method",
		strings.NewReader(`{"setup_intent_id":"seti_1"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	select {
	case evt := <-waiter.written:
		if evt.Action != auditlog.PaymentMethodAdd {
			t.Errorf("action = %q, want %q", evt.Action, auditlog.PaymentMethodAdd)
		}
		if evt.Description != "Added payment method Visa ending 4242" {
			t.Errorf("description = %q, want the card named", evt.Description)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no audit entry was written, so saving a card left no trail")
	}
}
