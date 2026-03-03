package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"github.com/postman/astro/apps/astro-server/internal/waitlist"
)

func setupWaitlistTestRouter() (*gin.Engine, *waitlist.Store, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := waitlist.NewStore(db)
	router := gin.New()
	return router, store, mock
}

func TestJoinWaitlist_Success(t *testing.T) {
	router, store, mock := setupWaitlistTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/waitlist", JoinWaitlist(log, store))

	now := time.Now()
	mock.ExpectQuery("INSERT INTO waitlist").
		WithArgs("alice@example.com", "Alice").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "email", "invited_at", "created_at"}).
			AddRow("00000000-0000-0000-0000-000000000001", "Alice", "alice@example.com", nil, now))

	body := `{"name": "Alice", "email": "alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/waitlist", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["email"] != "alice@example.com" {
		t.Errorf("expected email 'alice@example.com', got %v", resp["email"])
	}
	if resp["name"] != "Alice" {
		t.Errorf("expected name 'Alice', got %v", resp["name"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestJoinWaitlist_EmailNormalization(t *testing.T) {
	router, store, mock := setupWaitlistTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/waitlist", JoinWaitlist(log, store))

	now := time.Now()
	mock.ExpectQuery("INSERT INTO waitlist").
		WithArgs("alice@example.com", "Alice").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "email", "invited_at", "created_at"}).
			AddRow("00000000-0000-0000-0000-000000000001", "Alice", "alice@example.com", nil, now))

	body := `{"name": "Alice", "email": "Alice@Example.COM"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/waitlist", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestJoinWaitlist_DuplicateEmail(t *testing.T) {
	router, store, mock := setupWaitlistTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/waitlist", JoinWaitlist(log, store))

	mock.ExpectQuery("INSERT INTO waitlist").
		WithArgs("alice@example.com", "Alice").
		WillReturnError(&pq.Error{Code: "23505", Message: "duplicate key value violates unique constraint"})

	body := `{"name": "Alice", "email": "alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/waitlist", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["error"] != "This email is already on the waitlist." {
		t.Errorf("expected duplicate error message, got %v", resp["error"])
	}
}

func TestJoinWaitlist_MissingFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing email",
			body: `{"name": "Alice"}`,
		},
		{
			name: "missing name",
			body: `{"email": "alice@example.com"}`,
		},
		{
			name: "empty body",
			body: `{}`,
		},
		{
			name: "invalid email",
			body: `{"name": "Alice", "email": "not-an-email"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, store, _ := setupWaitlistTestRouter()
			log := logger.New("error", "json")

			router.POST("/api/v1/waitlist", JoinWaitlist(log, store))

			req := httptest.NewRequest(http.MethodPost, "/api/v1/waitlist", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}

			var resp map[string]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if resp["error"] != "Name and email are required." {
				t.Errorf("expected validation error, got %v", resp["error"])
			}
		})
	}
}

func TestJoinWaitlist_InvalidJSON(t *testing.T) {
	router, store, _ := setupWaitlistTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/waitlist", JoinWaitlist(log, store))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/waitlist", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestJoinWaitlist_DBError(t *testing.T) {
	router, store, mock := setupWaitlistTestRouter()
	log := logger.New("error", "json")

	router.POST("/api/v1/waitlist", JoinWaitlist(log, store))

	mock.ExpectQuery("INSERT INTO waitlist").
		WithArgs("alice@example.com", "Alice").
		WillReturnError(sqlmock.ErrCancelled)

	body := `{"name": "Alice", "email": "alice@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/waitlist", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d: %s", http.StatusInternalServerError, rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["error"] != "Failed to join waitlist." {
		t.Errorf("expected DB error message, got %v", resp["error"])
	}
}
