package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func setupFeedbackTestRouter() (*gin.Engine, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	router := gin.New()
	log := logger.New("error", "json")

	// Inject authenticated user for all routes by default
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{
			ID:    "user-1",
			Email: "test@example.com",
		})
		c.Next()
	})

	router.POST("/api/v1/feedback", SubmitFeedback(log, db))
	return router, mock
}

func setupFeedbackTestRouterNoAuth() *gin.Engine {
	gin.SetMode(gin.TestMode)
	db, _, _ := sqlmock.New()
	router := gin.New()
	log := logger.New("error", "json")
	router.POST("/api/v1/feedback", SubmitFeedback(log, db))
	return router
}

func TestSubmitFeedback_Success(t *testing.T) {
	router, mock := setupFeedbackTestRouter()

	mock.ExpectQuery("SELECT COUNT").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectQuery("INSERT INTO feedback_submissions").
		WithArgs("user-1", "test@example.com", "Great product!", "/dashboard").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("00000000-0000-0000-0000-000000000001"))

	body := `{"message": "Great product!", "page_url": "/dashboard"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var resp FeedbackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.ID != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("expected id '00000000-0000-0000-0000-000000000001', got %s", resp.ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestSubmitFeedback_NoAuth(t *testing.T) {
	router := setupFeedbackTestRouterNoAuth()

	body := `{"message": "Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
}

func TestSubmitFeedback_EmptyMessage(t *testing.T) {
	router, _ := setupFeedbackTestRouter()

	tests := []struct {
		name string
		body string
	}{
		{"empty string", `{"message": ""}`},
		{"whitespace only", `{"message": "   "}`},
		{"missing field", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}

			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			if resp["error"] != "message is required" {
				t.Errorf("expected 'message is required' error, got %v", resp["error"])
			}
		})
	}
}

func TestSubmitFeedback_MessageTooLong(t *testing.T) {
	router, _ := setupFeedbackTestRouter()

	longMessage := strings.Repeat("a", 5001)
	body := `{"message": "` + longMessage + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(body))
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
	if resp["error"] != "message must be 5000 characters or fewer" {
		t.Errorf("expected length error, got %v", resp["error"])
	}
}

func TestSubmitFeedback_RateLimited(t *testing.T) {
	router, mock := setupFeedbackTestRouter()

	mock.ExpectQuery("SELECT COUNT").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

	body := `{"message": "Another one"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected status %d, got %d: %s", http.StatusTooManyRequests, rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["error"] != "too many feedback submissions, please try again later" {
		t.Errorf("expected rate limit error, got %v", resp["error"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestSubmitFeedback_DBInsertError(t *testing.T) {
	router, mock := setupFeedbackTestRouter()

	mock.ExpectQuery("SELECT COUNT").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectQuery("INSERT INTO feedback_submissions").
		WithArgs("user-1", "test@example.com", "Feedback", "").
		WillReturnError(sqlmock.ErrCancelled)

	body := `{"message": "Feedback"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d: %s", http.StatusInternalServerError, rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestSubmitFeedback_RateLimitCheckError(t *testing.T) {
	router, mock := setupFeedbackTestRouter()

	mock.ExpectQuery("SELECT COUNT").
		WithArgs("user-1").
		WillReturnError(sqlmock.ErrCancelled)

	body := `{"message": "Feedback"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d: %s", http.StatusInternalServerError, rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestSubmitFeedback_InvalidJSON(t *testing.T) {
	router, _ := setupFeedbackTestRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestSubmitFeedback_WhitespaceTrimmed(t *testing.T) {
	router, mock := setupFeedbackTestRouter()

	mock.ExpectQuery("SELECT COUNT").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	// Message with leading/trailing whitespace should be trimmed before insert
	mock.ExpectQuery("INSERT INTO feedback_submissions").
		WithArgs("user-1", "test@example.com", "trimmed", "").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("00000000-0000-0000-0000-000000000002"))

	body := `{"message": "  trimmed  "}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(body))
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
