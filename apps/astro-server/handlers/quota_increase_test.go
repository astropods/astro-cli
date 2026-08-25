package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func injectUser(id string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: id})
		c.Next()
	}
}

func setupQuotaIncreaseRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(injectAccount(testAccount()))
	router.Use(injectUser("user-1"))
	log := logger.New("error", "json")
	router.POST("/quota-increase", RequestQuotaIncrease(log, db))
	return router
}

func postQuotaIncrease(router *gin.Engine, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/quota-increase", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRequestQuotaIncrease_InvalidBody(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	router := setupQuotaIncreaseRouter(db)

	req := httptest.NewRequest(http.MethodPost, "/quota-increase", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequestQuotaIncrease_InvalidFeatureKey(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	router := setupQuotaIncreaseRouter(db)

	rec := postQuotaIncrease(router, map[string]any{
		"feature_key": "not_a_real_feature",
		"reason":      "need more",
	})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequestQuotaIncrease_MissingReason(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	router := setupQuotaIncreaseRouter(db)

	rec := postQuotaIncrease(router, map[string]any{
		"feature_key": "agent_builds",
	})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequestQuotaIncrease_WhitespaceOnlyReason(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	router := setupQuotaIncreaseRouter(db)

	rec := postQuotaIncrease(router, map[string]any{
		"feature_key": "agent_builds",
		"reason":      "   ",
	})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequestQuotaIncrease_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	// Pins requested_by (the 7th bound param) to the authenticated user's ID;
	// a handler that reads the wrong context key fails this expectation
	// instead of silently inserting an empty string.
	mock.ExpectQuery(`INSERT INTO quota_increase_requests`).
		WithArgs(testAccount().ID, "agent_builds", 8.5, nil, nil, "running large workloads", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("req-abc123"))

	router := setupQuotaIncreaseRouter(db)
	rec := postQuotaIncrease(router, map[string]any{
		"feature_key":   "agent_builds",
		"current_usage": 8.5,
		"reason":        "running large workloads",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp QuotaIncreaseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != "req-abc123" {
		t.Errorf("id: want %q, got %q", "req-abc123", resp.ID)
	}
	if resp.Status != "pending" {
		t.Errorf("status: want %q, got %q", "pending", resp.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRequestQuotaIncrease_Unauthenticated(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(injectAccount(testAccount()))
	// No injectUser: simulates a request that reached this handler without
	// the auth middleware ever populating the user context.
	log := logger.New("error", "json")
	router.POST("/quota-increase", RequestQuotaIncrease(log, db))

	rec := postQuotaIncrease(router, map[string]any{
		"feature_key": "agent_builds",
		"reason":      "running large workloads",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequestQuotaIncrease_DBError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectQuery(`INSERT INTO quota_increase_requests`).
		WillReturnError(sql.ErrConnDone)

	router := setupQuotaIncreaseRouter(db)
	rec := postQuotaIncrease(router, map[string]any{
		"feature_key": "blueprints",
		"reason":      "scaling up",
	})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}
