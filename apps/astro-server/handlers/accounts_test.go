package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/logger"
)

func setupAccountTestRouter() (*gin.Engine, *account.AccountStore, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	router := gin.New()
	return router, store, mock
}

var accountColumns = []string{"id", "name", "type", "workos_org_id", "created_at", "updated_at"}

func TestSearchAccounts_Success(t *testing.T) {
	router, store, mock := setupAccountTestRouter()
	log := logger.New("error", "json")

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store))

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("foo%", 10).
		WillReturnRows(sqlmock.NewRows(accountColumns).
			AddRow("id-1", "foobar", "personal", "", now, now).
			AddRow("id-2", "foocorp", "organization", "workos-123", now, now))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search?q=foo", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Results []SearchAccountResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.Results[0].Name != "foobar" {
		t.Errorf("expected 'foobar', got %q", resp.Results[0].Name)
	}
	if resp.Results[1].Name != "foocorp" {
		t.Errorf("expected 'foocorp', got %q", resp.Results[1].Name)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestSearchAccounts_WithTypeFilter(t *testing.T) {
	router, store, mock := setupAccountTestRouter()
	log := logger.New("error", "json")

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store))

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts .+ AND a\\.type").
		WithArgs("bar%", "personal", 10).
		WillReturnRows(sqlmock.NewRows(accountColumns).
			AddRow("id-1", "barry", "personal", "", now, now))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search?q=bar&type=personal", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Results []SearchAccountResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Type != "personal" {
		t.Errorf("expected type 'personal', got %q", resp.Results[0].Type)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestSearchAccounts_CustomLimit(t *testing.T) {
	router, store, mock := setupAccountTestRouter()
	log := logger.New("error", "json")

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store))

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("abc%", 5).
		WillReturnRows(sqlmock.NewRows(accountColumns).
			AddRow("id-1", "abcdef", "personal", "", now, now))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search?q=abc&limit=5", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestSearchAccounts_LimitCappedAt10(t *testing.T) {
	router, store, mock := setupAccountTestRouter()
	log := logger.New("error", "json")

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store))

	// Even though limit=50 is requested, store caps at 10
	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("abc%", 10).
		WillReturnRows(sqlmock.NewRows(accountColumns))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search?q=abc&limit=50", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestSearchAccounts_MissingQuery(t *testing.T) {
	router, store, _ := setupAccountTestRouter()
	log := logger.New("error", "json")

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSearchAccounts_QueryTooShort(t *testing.T) {
	tests := []struct {
		name string
		q    string
	}{
		{"empty", ""},
		{"one char", "a"},
		{"two chars", "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, store, _ := setupAccountTestRouter()
			log := logger.New("error", "json")

			router.GET("/api/v1/accounts/search", SearchAccounts(log, store))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search?q="+tt.q, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for q=%q, got %d: %s", tt.q, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSearchAccounts_InvalidType(t *testing.T) {
	router, store, _ := setupAccountTestRouter()
	log := logger.New("error", "json")

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search?q=foo&type=invalid", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["error"] != "type must be 'personal' or 'organization'" {
		t.Errorf("unexpected error message: %v", resp["error"])
	}
}

func TestSearchAccounts_QueryLowercased(t *testing.T) {
	router, store, mock := setupAccountTestRouter()
	log := logger.New("error", "json")

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store))

	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("foo%", 10).
		WillReturnRows(sqlmock.NewRows(accountColumns))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search?q=FOO", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestSearchAccounts_WildcardsEscaped(t *testing.T) {
	router, store, mock := setupAccountTestRouter()
	log := logger.New("error", "json")

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store))

	// % and _ in query should be escaped so they match literally
	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs(`fo\%o%`, 10).
		WillReturnRows(sqlmock.NewRows(accountColumns))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search?q=fo%25o", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestSearchAccounts_EmptyResults(t *testing.T) {
	router, store, mock := setupAccountTestRouter()
	log := logger.New("error", "json")

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store))

	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("zzz%", 10).
		WillReturnRows(sqlmock.NewRows(accountColumns))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search?q=zzz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Results []SearchAccountResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestSearchAccounts_DBError(t *testing.T) {
	router, store, mock := setupAccountTestRouter()
	log := logger.New("error", "json")

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store))

	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("foo%", 10).
		WillReturnError(sqlmock.ErrCancelled)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/search?q=foo", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}
