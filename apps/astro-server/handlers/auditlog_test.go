package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

// --- ListAuditLogFilters tests ---

func TestListAuditLogFilters_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := auditlog.NewStore(db)
	log := logger.New("error", "json")

	mock.ExpectQuery("SELECT kind, val FROM").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"kind", "val"}).
			AddRow("act", "agent.register").
			AddRow("act", "deployment.deploy").
			AddRow("rt", "agent").
			AddRow("rt", "deployment"))

	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	user := &auth.User{ID: "user-1"}

	router := gin.New()
	router.GET("/audit-log/filters", injectTestOrgAccount(acct, user), ListAuditLogFilters(log, store))

	req := httptest.NewRequest(http.MethodGet, "/audit-log/filters", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp auditlog.FilterOptions
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.ResourceTypes) != 2 {
		t.Errorf("expected 2 resource types, got %d", len(resp.ResourceTypes))
	}
	if len(resp.Actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(resp.Actions))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestListAuditLogFilters_EmptyAccount(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := auditlog.NewStore(db)
	log := logger.New("error", "json")

	mock.ExpectQuery("SELECT kind, val FROM").
		WithArgs("acct-empty").
		WillReturnRows(sqlmock.NewRows([]string{"kind", "val"}))

	acct := &account.Account{ID: "acct-empty", Name: "emptyorg", Type: "organization"}
	user := &auth.User{ID: "user-1"}

	router := gin.New()
	router.GET("/audit-log/filters", injectTestOrgAccount(acct, user), ListAuditLogFilters(log, store))

	req := httptest.NewRequest(http.MethodGet, "/audit-log/filters", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp auditlog.FilterOptions
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Should return empty arrays, not null
	if resp.ResourceTypes == nil {
		t.Error("resource_types should be empty array, got nil")
	}
	if resp.Actions == nil {
		t.Error("actions should be empty array, got nil")
	}
	if len(resp.ResourceTypes) != 0 {
		t.Errorf("expected 0 resource types, got %d", len(resp.ResourceTypes))
	}
	if len(resp.Actions) != 0 {
		t.Errorf("expected 0 actions, got %d", len(resp.Actions))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestListAuditLogFilters_NoAccount(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := auditlog.NewStore(db)
	log := logger.New("error", "json")

	router := gin.New()
	// No account injected — should return 500
	router.GET("/audit-log/filters", ListAuditLogFilters(log, store))

	req := httptest.NewRequest(http.MethodGet, "/audit-log/filters", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}
