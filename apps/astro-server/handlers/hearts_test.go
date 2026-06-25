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
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow(id, name, "personal", nil, nil, now, now)...))
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "cluster_id", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}))

	req := httptest.NewRequest(http.MethodPost, "/agents/nonexistent/my-agent/heart", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- ListHearted handler tests ---

func setupListHeartedRouter() (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New()
	heartDB, heartMock, _ := sqlmock.New()

	accountStore := account.NewAccountStore(accountDB)
	hearts := heartstore.New(heartDB)
	log := logger.New("error", "json")

	router := gin.New()
	router.GET("/api/v1/accounts/:account/hearts", ListHearted(log, hearts, accountStore))

	return router, accountMock, heartMock
}

func expectListHeartedAccountLookup(mock sqlmock.Sqlmock, name, id string) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow(id, name, "personal", nil, nil, now, now)...))
}

func TestListHearted_EmptyList(t *testing.T) {
	router, accountMock, heartMock := setupListHeartedRouter()

	expectListHeartedAccountLookup(accountMock, "taylor", "acct-1")
	accountMock.ExpectQuery("SELECT user_id FROM account_members").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("user-1"))

	heartCols := []string{"account", "name", "visibility", "avatar_colors", "heart_count", "deploy_count", "hearted_at", "description"}
	heartMock.ExpectQuery("SELECT owner.name").
		WithArgs("user-1", 21).
		WillReturnRows(sqlmock.NewRows(heartCols))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/taylor/hearts", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	items := resp["items"].([]interface{})
	if len(items) != 0 {
		t.Errorf("expected empty items, got %d", len(items))
	}
	if _, hasCursor := resp["next_cursor"]; hasCursor {
		t.Error("expected no next_cursor in response when list is empty")
	}
}

func TestListHearted_WithResults(t *testing.T) {
	router, accountMock, heartMock := setupListHeartedRouter()

	expectListHeartedAccountLookup(accountMock, "taylor", "acct-1")
	accountMock.ExpectQuery("SELECT user_id FROM account_members").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("user-1"))

	now := time.Now()
	heartCols := []string{"account", "name", "visibility", "avatar_colors", "heart_count", "deploy_count", "hearted_at", "description"}
	heartMock.ExpectQuery("SELECT owner.name").
		WithArgs("user-1", 21).
		WillReturnRows(sqlmock.NewRows(heartCols).
			AddRow("someorg", "cool-agent", "public", nil, 5, 3, now, "A cool agent"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/taylor/hearts", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	items := resp["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestListHearted_AccountNotFound(t *testing.T) {
	router, accountMock, _ := setupListHeartedRouter()

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("nobody").
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/nobody/hearts", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListHearted_NoMemberReturnsEmpty(t *testing.T) {
	router, accountMock, _ := setupListHeartedRouter()

	expectListHeartedAccountLookup(accountMock, "orphan", "acct-2")
	accountMock.ExpectQuery("SELECT user_id FROM account_members").
		WithArgs("acct-2").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/orphan/hearts", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	items := resp["items"].([]interface{})
	if len(items) != 0 {
		t.Errorf("expected empty items for memberless account, got %d", len(items))
	}
}

func TestListHearted_OrgAccountReturns404(t *testing.T) {
	router, accountMock, _ := setupListHeartedRouter()

	now := time.Now()
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("astro-inc").
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow("org-1", "astro-inc", "organization", nil, nil, now, now, "Astro Inc", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/astro-inc/hearts", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for org account, got %d: %s", rec.Code, rec.Body.String())
	}
}
