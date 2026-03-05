package openmeter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/logger"
)

func TestReconciler_SyncsAccounts(t *testing.T) {
	var created atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		created.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "om-cust-new"})
	}))
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	client := NewClient(srv.URL)

	now := time.Now()

	// First batch: 2 accounts
	mock.ExpectQuery("SELECT .+ FROM accounts WHERE openmeter_customer_id IS NULL").
		WithArgs(reconcileBatchSize).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
			AddRow("acct-1", "org1", "organization", now, now).
			AddRow("acct-2", "personal1", "personal", now, now))

	mock.ExpectExec("UPDATE accounts SET openmeter_customer_id").
		WithArgs("om-cust-new", sqlmock.AnyArg(), "acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectExec("UPDATE accounts SET openmeter_customer_id").
		WithArgs("om-cust-new", sqlmock.AnyArg(), "acct-2").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Second batch: empty (signals done)
	mock.ExpectQuery("SELECT .+ FROM accounts WHERE openmeter_customer_id IS NULL").
		WithArgs(reconcileBatchSize).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}))

	r := NewReconciler(client, store, log)
	r.Run(context.Background())

	if created.Load() != 2 {
		t.Errorf("expected 2 OpenMeter customers created, got %d", created.Load())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestReconciler_SkipsOnEmpty(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	client := NewClient("http://should-not-be-called")

	mock.ExpectQuery("SELECT .+ FROM accounts WHERE openmeter_customer_id IS NULL").
		WithArgs(reconcileBatchSize).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}))

	r := NewReconciler(client, store, log)
	r.Run(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestReconciler_ContinuesOnFailure(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "om-cust-ok"})
	}))
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	client := NewClient(srv.URL)

	now := time.Now()

	// Batch with 2 accounts: first fails, second succeeds
	mock.ExpectQuery("SELECT .+ FROM accounts WHERE openmeter_customer_id IS NULL").
		WithArgs(reconcileBatchSize).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
			AddRow("acct-fail", "failing", "personal", now, now).
			AddRow("acct-ok", "working", "organization", now, now))

	// Only second account gets SetOpenMeterCustomerID
	mock.ExpectExec("UPDATE accounts SET openmeter_customer_id").
		WithArgs("om-cust-ok", sqlmock.AnyArg(), "acct-ok").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Next batch: empty
	mock.ExpectQuery("SELECT .+ FROM accounts WHERE openmeter_customer_id IS NULL").
		WithArgs(reconcileBatchSize).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}))

	r := NewReconciler(client, store, log)
	r.Run(context.Background())

	if callCount.Load() != 2 {
		t.Errorf("expected 2 API calls (1 fail + 1 success), got %d", callCount.Load())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}
