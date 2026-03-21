package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func setupHeartRouter(withUser bool, userID string) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New()
	heartDB, heartMock, _ := sqlmock.New()

	accountStore := account.NewAccountStore(accountDB)
	hearts := heartstore.New(heartDB)
	log := logger.New("error", "json")

	router := gin.New()
	if withUser {
		router.Use(func(c *gin.Context) {
			c.Set(string(auth.UserContextKey), &auth.User{ID: userID})
			c.Next()
		})
	}

	router.POST("/agents/:account/:name/heart", ToggleHeart(log, hearts, accountStore))

	return router, accountMock, heartMock
}

func expectHeartAccountLookup(mock sqlmock.Sqlmock, name, id string) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version"}).
			AddRow(id, name, "personal", nil, nil, now, now, 0))
}

func TestToggleHeart_AddHeart(t *testing.T) {
	router, accountMock, heartMock := setupHeartRouter(true, "user-1")

	expectHeartAccountLookup(accountMock, "myorg", "acct-1")

	// Toggle query — insert succeeds (no existing row to delete)
	heartMock.ExpectQuery("WITH toggled AS").
		WithArgs("acct-1", "my-agent", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"hearted", "count"}).AddRow(true, 1))

	req := httptest.NewRequest(http.MethodPost, "/agents/myorg/my-agent/heart", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp HeartResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.Hearted {
		t.Error("expected hearted=true")
	}
	if resp.HeartCount != 1 {
		t.Errorf("expected heart_count=1, got %d", resp.HeartCount)
	}
}

func TestToggleHeart_RemoveHeart(t *testing.T) {
	router, accountMock, heartMock := setupHeartRouter(true, "user-1")

	expectHeartAccountLookup(accountMock, "myorg", "acct-1")

	// Toggle query — delete succeeds (existing row removed, no insert)
	heartMock.ExpectQuery("WITH toggled AS").
		WithArgs("acct-1", "my-agent", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"hearted", "count"}).AddRow(false, 0))

	req := httptest.NewRequest(http.MethodPost, "/agents/myorg/my-agent/heart", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp HeartResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Hearted {
		t.Error("expected hearted=false")
	}
	if resp.HeartCount != 0 {
		t.Errorf("expected heart_count=0, got %d", resp.HeartCount)
	}
}

func TestToggleHeart_NoAuth(t *testing.T) {
	router, _, _ := setupHeartRouter(false, "")

	req := httptest.NewRequest(http.MethodPost, "/agents/myorg/my-agent/heart", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestToggleHeart_AccountNotFound(t *testing.T) {
	router, accountMock, _ := setupHeartRouter(true, "user-1")

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("nonexistent").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version"}))

	req := httptest.NewRequest(http.MethodPost, "/agents/nonexistent/my-agent/heart", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
