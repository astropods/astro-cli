package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

type cardSignalQueue struct {
	suspended     []string
	resumed       []string
	collected     []string
	gatewayBudget []string
}

func (q *cardSignalQueue) InsertBillingSuspend(_ context.Context, id string) error {
	q.suspended = append(q.suspended, id)
	return nil
}
func (q *cardSignalQueue) InsertBillingResume(_ context.Context, id string) error {
	q.resumed = append(q.resumed, id)
	return nil
}
func (q *cardSignalQueue) InsertBillingCollect(_ context.Context, id, _ string) error {
	q.collected = append(q.collected, id)
	return nil
}
func (q *cardSignalQueue) InsertBillingGatewayBudget(_ context.Context, id string) error {
	q.gatewayBudget = append(q.gatewayBudget, id)
	return nil
}

func statusRow(status, reason string, exhausted, hasPaymentMethod bool) *sqlmock.Rows {
	return dunningStatusRow(status, reason, nil, exhausted, hasPaymentMethod)
}

func dunningStatusRow(status, reason string, dunningSince *time.Time, exhausted, hasPaymentMethod bool) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"status", "reason", "dunning_since", "alert_active",
		"force_suspended", "credits_exhausted", "has_payment_method", "pay_link",
		"usage_limit_active", "not_provisioned",
	}).AddRow(status, reason, dunningSince, false, false, exhausted, hasPaymentMethod, nil, false, false)
}

func testGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	return c
}

func TestApplyCardSignal_AddingCardResumesAndRederivesGatewayBudget(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectExec("has_payment_method = EXCLUDED").
		WithArgs("acct-1", true).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").
		WithArgs("acct-1").
		WillReturnRows(statusRow("suspended", "credits_exhausted", true, true))
	mock.ExpectExec("account_billing_status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	q := &cardSignalQueue{}
	log := logger.New("error", "json")
	applyCardSignal(testGinContext(), log, billing.NewStatusStore(db, 7), q, "acct-1", "cus_1", billing.SignalCardAdded)

	if len(q.resumed) != 1 || q.resumed[0] != "acct-1" {
		t.Errorf("resumed = %v, want [acct-1]", q.resumed)
	}
	if len(q.suspended) != 0 {
		t.Errorf("suspended = %v, want none", q.suspended)
	}
	if len(q.gatewayBudget) != 1 || q.gatewayBudget[0] != "acct-1" {
		t.Errorf("gatewayBudget = %v, want [acct-1]", q.gatewayBudget)
	}
	if len(q.collected) != 0 {
		t.Errorf("collected = %v, want none: resuming to active must not also charge the new card", q.collected)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestApplyCardSignal_RemovingCardSuspendsAnExhaustedAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectExec("has_payment_method = EXCLUDED").
		WithArgs("acct-1", false).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").
		WithArgs("acct-1").
		WillReturnRows(statusRow("active", "", true, false))
	mock.ExpectExec("account_billing_status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	q := &cardSignalQueue{}
	log := logger.New("error", "json")
	applyCardSignal(testGinContext(), log, billing.NewStatusStore(db, 7), q, "acct-1", "cus_1", billing.SignalCardRemoved)

	if len(q.suspended) != 1 || q.suspended[0] != "acct-1" {
		t.Errorf("suspended = %v, want [acct-1]", q.suspended)
	}
	if len(q.resumed) != 0 {
		t.Errorf("resumed = %v, want none", q.resumed)
	}
	if len(q.collected) != 0 {
		t.Errorf("collected = %v, want none: removing a card must never trigger a charge attempt", q.collected)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestApplyCardSignal_CollectsEvenWhenStatusDoesNotChange(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	pastGrace := time.Now().Add(-8 * 24 * time.Hour)
	mock.ExpectExec("has_payment_method = EXCLUDED").
		WithArgs("acct-1", true).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").
		WithArgs("acct-1").
		WillReturnRows(dunningStatusRow("suspended", "payment_failed", &pastGrace, false, true))
	mock.ExpectRollback()

	q := &cardSignalQueue{}
	log := logger.New("error", "json")
	applyCardSignal(testGinContext(), log, billing.NewStatusStore(db, 7), q, "acct-1", "cus_1", billing.SignalCardAdded)

	if len(q.collected) != 1 || q.collected[0] != "acct-1" {
		t.Errorf("collected = %v, want [acct-1]: a new card must attempt to collect existing debt even if the account stays suspended", q.collected)
	}
	if len(q.gatewayBudget) != 1 {
		t.Errorf("gatewayBudget = %v, want [acct-1]: re-derived on every card change, not only on a status transition", q.gatewayBudget)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestApplyCardSignal_NilStatusStoreIsANoOp(t *testing.T) {
	q := &cardSignalQueue{}
	log := logger.New("error", "json")
	applyCardSignal(testGinContext(), log, nil, q, "acct-1", "cus_1", billing.SignalCardAdded)

	if len(q.gatewayBudget) != 0 || len(q.resumed) != 0 || len(q.suspended) != 0 || len(q.collected) != 0 {
		t.Errorf("queue calls = %+v, want none when there is no status store", q)
	}
}
