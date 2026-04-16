package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

// --- SetAgentVisibility tests ---

func TestSetAgentVisibility_Success(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.PUT("/agents/:account/:name/visibility", injectTestAccount(), SetAgentVisibility(log, index, nil))

	mock.ExpectExec("UPDATE agents SET visibility").
		WithArgs("public", sqlmock.AnyArg(), "test-account-id", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"visibility": "public"}`
	req := httptest.NewRequest(http.MethodPut, "/agents/testaccount/my-agent/visibility", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["visibility"] != "public" {
		t.Errorf("expected visibility 'public', got %v", resp["visibility"])
	}
	if resp["message"] != "visibility updated" {
		t.Errorf("expected success message, got %v", resp["message"])
	}
}

func TestSetAgentVisibility_SetPrivate(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.PUT("/agents/:account/:name/visibility", injectTestAccount(), SetAgentVisibility(log, index, nil))

	mock.ExpectExec("UPDATE agents SET visibility").
		WithArgs("private", sqlmock.AnyArg(), "test-account-id", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"visibility": "private"}`
	req := httptest.NewRequest(http.MethodPut, "/agents/testaccount/my-agent/visibility", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSetAgentVisibility_InvalidVisibility(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.PUT("/agents/:account/:name/visibility", injectTestAccount(), SetAgentVisibility(log, index, nil))

	// SetVisibility will return error for invalid value
	mock.ExpectExec("UPDATE agents SET visibility").
		WithArgs("unlisted", sqlmock.AnyArg(), "test-account-id", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"visibility": "unlisted"}`
	req := httptest.NewRequest(http.MethodPut, "/agents/testaccount/my-agent/visibility", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid visibility, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSetAgentVisibility_MissingBody(t *testing.T) {
	router, index, _ := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.PUT("/agents/:account/:name/visibility", injectTestAccount(), SetAgentVisibility(log, index, nil))

	req := httptest.NewRequest(http.MethodPut, "/agents/testaccount/my-agent/visibility", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing visibility, got %d", rec.Code)
	}
}

func TestSetAgentVisibility_AgentNotFound(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.PUT("/agents/:account/:name/visibility", injectTestAccount(), SetAgentVisibility(log, index, nil))

	mock.ExpectExec("UPDATE agents SET visibility").
		WithArgs("public", sqlmock.AnyArg(), "test-account-id", "nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	body := `{"visibility": "public"}`
	req := httptest.NewRequest(http.MethodPut, "/agents/testaccount/nonexistent/visibility", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for nonexistent agent, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSetAgentVisibility_NoAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _, _ := sqlmock.New()
	index := agentindex.NewIndexWithDB(db)
	log := logger.New("error", "json")

	router := gin.New()
	// No account injected
	router.PUT("/agents/:account/:name/visibility", SetAgentVisibility(log, index, nil))

	body := `{"visibility": "public"}`
	req := httptest.NewRequest(http.MethodPut, "/agents/testaccount/my-agent/visibility", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for no account, got %d", rec.Code)
	}
}

// --- ArchiveAgent tests ---

func TestArchiveAgent_Success(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/agents/:account/:name/archive", injectTestAccount(), ArchiveAgent(log, index, nil, nil, nil, nil, nil))

	mock.ExpectExec("UPDATE agents SET archived_at").
		WithArgs(sqlmock.AnyArg(), "test-account-id", "my-agent").
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPost, "/agents/testaccount/my-agent/archive", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestArchiveAgent_NotFound(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/agents/:account/:name/archive", injectTestAccount(), ArchiveAgent(log, index, nil, nil, nil, nil, nil))

	mock.ExpectExec("UPDATE agents SET archived_at").
		WithArgs(sqlmock.AnyArg(), "test-account-id", "nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	req := httptest.NewRequest(http.MethodPost, "/agents/testaccount/nonexistent/archive", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nonexistent agent, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestArchiveAgent_DBError(t *testing.T) {
	router, index, mock := setupAgentTestRouter()
	log := logger.New("error", "json")

	router.POST("/agents/:account/:name/archive", injectTestAccount(), ArchiveAgent(log, index, nil, nil, nil, nil, nil))

	mock.ExpectExec("UPDATE agents SET archived_at").
		WithArgs(sqlmock.AnyArg(), "test-account-id", "my-agent").
		WillReturnError(sqlmock.ErrCancelled)

	req := httptest.NewRequest(http.MethodPost, "/agents/testaccount/my-agent/archive", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on DB error, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["error"] != "failed to archive agent" {
		t.Errorf("expected 'failed to archive agent' error, got %v", resp["error"])
	}
}

func TestArchiveAgent_NoAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _, _ := sqlmock.New()
	index := agentindex.NewIndexWithDB(db)
	log := logger.New("error", "json")

	router := gin.New()
	// No account injected
	router.POST("/agents/:account/:name/archive", ArchiveAgent(log, index, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodPost, "/agents/testaccount/my-agent/archive", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for no account in context, got %d", rec.Code)
	}
}

// --- GetAgent visibility tests ---

func setupAgentGetRouter(withUser bool, userID string) (*gin.Engine, *agentindex.Index, *account.AccountStore, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)

	indexDB, indexMock, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()
	heartDB, _, _ := sqlmock.New()

	index := agentindex.NewIndexWithDB(indexDB)
	store := account.NewAccountStore(accountDB)
	hearts := heartstore.New(heartDB)
	log := logger.New("error", "json")

	router := gin.New()
	if withUser {
		router.Use(func(c *gin.Context) {
			c.Set(string(auth.UserContextKey), &auth.User{ID: userID})
			c.Next()
		})
	}
	router.GET("/agents/:account/:name", GetAgent(log, index, store, hearts, nil, nil, nil))

	return router, index, store, indexMock, accountMock
}

func TestGetAgent_PublicAgent_NoAuth(t *testing.T) {
	router, _, _, indexMock, accountMock := setupAgentGetRouter(false, "")

	now := time.Now()

	// Account lookup
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	// Agent lookup
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", "public", nil, now, now))

	// Versions
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "testaccount", `{"name":"test"}`, "", "", "[]", now, now))

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("public agent should be visible without auth, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp AgentResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Visibility != "public" {
		t.Errorf("expected visibility 'public', got %q", resp.Visibility)
	}
}

func TestGetAgent_PrivateAgent_NoAuth(t *testing.T) {
	router, _, _, indexMock, accountMock := setupAgentGetRouter(false, "")

	now := time.Now()

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", "private", nil, now, now))

	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "testaccount", `{"name":"test"}`, "", "", "[]", now, now))

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("private agent should return 404 to unauthenticated users, got %d", rec.Code)
	}
}

func TestGetAgent_PrivateAgent_NonMember(t *testing.T) {
	router, _, _, indexMock, accountMock := setupAgentGetRouter(true, "user-2")

	now := time.Now()

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", "private", nil, now, now))

	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "testaccount", `{"name":"test"}`, "", "", "[]", now, now))

	// IsMember check - not a member
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("private agent should return 404 to non-members, got %d", rec.Code)
	}
}

func TestGetAgent_PrivateAgent_Member(t *testing.T) {
	router, _, _, indexMock, accountMock := setupAgentGetRouter(true, "user-1")

	now := time.Now()

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", "private", nil, now, now))

	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "testaccount", `{"name":"test"}`, "", "", `[{"field":"test","message":"warning"}]`, now, now))

	// IsMember check - is a member
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("private agent should be visible to members, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp AgentResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Visibility != "private" {
		t.Errorf("expected visibility 'private', got %q", resp.Visibility)
	}
	// Members should see validation warnings
	if len(resp.Versions) == 0 {
		t.Fatal("expected at least one version")
	}
	if len(resp.Versions[0].ValidationWarnings) == 0 {
		t.Error("members should see validation warnings")
	}
}

func TestGetAgent_PublicAgent_NonMember_NoWarnings(t *testing.T) {
	router, _, _, indexMock, accountMock := setupAgentGetRouter(true, "user-outside")

	now := time.Now()

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", "public", nil, now, now))

	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "testaccount", `{"name":"test"}`, "", "", `[{"field":"test","message":"warning"}]`, now, now))

	// IsMember check - not a member
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-outside").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("public agent should be visible, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp AgentResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	// Non-members should NOT see validation warnings
	if len(resp.Versions) == 0 {
		t.Fatal("expected at least one version")
	}
	if len(resp.Versions[0].ValidationWarnings) != 0 {
		t.Error("non-members should not see validation warnings")
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	router, _, _, indexMock, accountMock := setupAgentGetRouter(false, "")

	now := time.Now()

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "nonexistent").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}))

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/nonexistent", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- ListAgents tests ---

func TestListAgents_OnlyPublic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	indexDB, indexMock, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()
	heartDB, _, _ := sqlmock.New()

	index := agentindex.NewIndexWithDB(indexDB)
	store := account.NewAccountStore(accountDB)
	hearts := heartstore.New(heartDB)
	log := logger.New("error", "json")

	router := gin.New()
	router.GET("/agents", ListAgents(log, index, store, hearts, nil, nil, nil))

	now := time.Now()

	// ListPublicAgents query
	indexMock.ExpectQuery("SELECT .+ FROM agents a.+WHERE a.visibility = 'public'").
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "name", "registry", "visibility", "created_at", "updated_at",
			"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "published_at", "updated_at",
		}).
			AddRow("acct-1", "public-agent", "registry.io", "public", now, now,
				"build-1", "myorg", `{"name":"test"}`, "", "", now, now))

	// Account lookup for name resolution
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	count := resp["count"].(float64)
	if count != 1 {
		t.Errorf("expected count 1, got %v", count)
	}

	agents := resp["agents"].([]any)
	agent := agents[0].(map[string]any)
	if agent["visibility"] != "public" {
		t.Errorf("expected visibility 'public', got %v", agent["visibility"])
	}
	if agent["account"] != "myorg" {
		t.Errorf("expected account name 'myorg', got %v", agent["account"])
	}
}

func TestListAgents_Empty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	indexDB, indexMock, _ := sqlmock.New()
	accountDB, _, _ := sqlmock.New()
	heartDB, _, _ := sqlmock.New()

	index := agentindex.NewIndexWithDB(indexDB)
	store := account.NewAccountStore(accountDB)
	hearts := heartstore.New(heartDB)
	log := logger.New("error", "json")

	router := gin.New()
	router.GET("/agents", ListAgents(log, index, store, hearts, nil, nil, nil))

	indexMock.ExpectQuery("SELECT .+ FROM agents a.+WHERE a.visibility = 'public'").
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "name", "registry", "visibility", "created_at", "updated_at",
			"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "published_at", "updated_at",
		}))

	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["count"].(float64) != 0 {
		t.Errorf("expected count 0, got %v", resp["count"])
	}
}
