package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func setupAccountTestRouter() (*gin.Engine, *account.AccountStore, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	router := gin.New()
	return router, store, mock
}

var accountColumns = []string{"id", "name", "type", "created_at", "updated_at", "avatar_updated_at"}

func TestSearchAccounts_Success(t *testing.T) {
	router, store, mock := setupAccountTestRouter()
	log := logger.New("error", "json")

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store, nil))

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("foo%", 10).
		WillReturnRows(sqlmock.NewRows(accountColumns).
			AddRow("id-1", "foobar", "personal", now, now, nil).
			AddRow("id-2", "foocorp", "organization", now, now, nil))

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

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store, nil))

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts .+ AND a\\.type").
		WithArgs("bar%", "personal", 10).
		WillReturnRows(sqlmock.NewRows(accountColumns).
			AddRow("id-1", "barry", "personal", now, now, nil))

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

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store, nil))

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("abc%", 5).
		WillReturnRows(sqlmock.NewRows(accountColumns).
			AddRow("id-1", "abcdef", "personal", now, now, nil))

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

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store, nil))

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

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store, nil))

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

			router.GET("/api/v1/accounts/search", SearchAccounts(log, store, nil))

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

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store, nil))

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

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store, nil))

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

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store, nil))

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

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store, nil))

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

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store, nil))

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

// --- CreateAccount handler tests ---

func TestCreateAccount_OrgRequiresDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		wantDetails string
	}{
		{"missing", "", "display name is required for organization accounts"},
		{"whitespace only", "   ", "display name is required for organization accounts"},
		{
			"too long",
			strings.Repeat("a", account.OrganizationDisplayNameMaxLength+1),
			"organization names cannot exceed 39 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			db, _, _ := sqlmock.New()
			store := account.NewAccountStore(db)
			log := logger.New("error", "json")

			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
				c.Next()
			})
			router.POST("/api/v1/accounts", CreateAccount(log, store, nil, nil, nil, nil, "", nil, nil))

			body := fmt.Sprintf(`{"name":"valid-org","type":"organization","display_name":%q}`, tt.displayName)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for display_name=%q, got %d: %s", tt.displayName, rec.Code, rec.Body.String())
			}

			var resp map[string]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}
			if resp["details"] != tt.wantDetails {
				t.Errorf("details = %v, want %q", resp["details"], tt.wantDetails)
			}
		})
	}
}

func TestCreateAccount_InvalidName(t *testing.T) {
	tests := []struct {
		name    string
		orgName string
	}{
		{"empty", ""},
		{"too short", "ab"},
		{"starts with digit", "1abc"},
		{"ends with hyphen", "abc-"},
		{"consecutive hyphens", "ab--cd"},
		{"uppercase", "MyOrg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			db, _, _ := sqlmock.New()
			store := account.NewAccountStore(db)
			log := logger.New("error", "json")

			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
				c.Next()
			})
			router.POST("/api/v1/accounts", CreateAccount(log, store, nil, nil, nil, nil, "", nil, nil))

			// Include a valid display_name so the org-display-name check doesn't
			// fire before we reach name validation.
			body := fmt.Sprintf(`{"name":%q,"type":"organization","display_name":"My Org"}`, tt.orgName)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for name=%q, got %d: %s", tt.orgName, rec.Code, rec.Body.String())
			}
		})
	}
}

// --- DeleteAccount handler tests ---

// deleteAccountDeploymentColumns matches deploymentstore.deploymentColumns for sqlmock.
var deleteAccountDeploymentColumns = []string{
	"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
	"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
	"status", "error_message", "error_details", "status_changed_at", "current_revision",
	"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
}

// deleteAccountMockQueue tracks calls to InsertUndeployJob.
type deleteAccountMockQueue struct {
	undeployIDs []string
	err         error // if non-nil, InsertUndeployJob returns this
}

func (q *deleteAccountMockQueue) InsertDeployJob(_ context.Context, _, _ string) error { return nil }
func (q *deleteAccountMockQueue) InsertUndeployJob(_ context.Context, id string, _ string) error {
	q.undeployIDs = append(q.undeployIDs, id)
	return q.err
}
func (q *deleteAccountMockQueue) InsertWakeUpJob(_ context.Context, _, _ string) error { return nil }

func setupDeleteAccountTest(t *testing.T) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock, *deleteAccountMockQueue) {
	return setupDeleteAccountTestWithJudgeKeys(t, nil, nil)
}

func setupDeleteAccountTestWithJudgeKeys(t *testing.T, provisioner *aigateway.Provisioner, judgeStore *aigateway.JudgeStore) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock, *deleteAccountMockQueue) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	queue := &deleteAccountMockQueue{}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), &account.Account{
			ID:   "acct-1",
			Name: "testaccount",
			Type: "personal",
		})
		c.Next()
	})
	router.DELETE("/api/v1/accounts/:account", DeleteAccount(log, accountStore, deployStore, queue, provisioner, judgeStore, nil, nil, "", nil))

	return router, accountMock, deployMock, queue
}

func TestDeleteAccount_RevokesJudgeKey(t *testing.T) {
	var deletes int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/governance/virtual-keys/vk-judge" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		deletes++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	judgeDB, judgeMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("judge sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = judgeDB.Close() })

	provisioner := aigateway.NewProvisioner(aigateway.NewClient(upstream.URL, upstream.URL, ""), nil, nil)
	router, accountMock, deployMock, _ := setupDeleteAccountTestWithJudgeKeys(t, provisioner, aigateway.NewJudgeStore(judgeDB))

	accountMock.ExpectExec(`UPDATE accounts SET deleted_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	judgeMock.ExpectQuery("SELECT key_id FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow("vk-judge"))
	judgeMock.ExpectExec("DELETE FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(deleteAccountDeploymentColumns))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/testaccount", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if deletes != 1 {
		t.Fatalf("upstream deletes = %d, want 1", deletes)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("account mock: %v", err)
	}
	if err := deployMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("deploy mock: %v", err)
	}
	if err := judgeMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("judge mock: %v", err)
	}
}

func TestDeleteAccount_JudgeRevokeFailureContinues(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	t.Cleanup(upstream.Close)

	judgeDB, judgeMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("judge sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = judgeDB.Close() })

	provisioner := aigateway.NewProvisioner(aigateway.NewClient(upstream.URL, upstream.URL, ""), nil, nil)
	router, accountMock, deployMock, _ := setupDeleteAccountTestWithJudgeKeys(t, provisioner, aigateway.NewJudgeStore(judgeDB))

	accountMock.ExpectExec(`UPDATE accounts SET deleted_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	judgeMock.ExpectQuery("SELECT key_id FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow("vk-judge"))
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(deleteAccountDeploymentColumns))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/testaccount", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 despite judge revoke failure, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("account mock: %v", err)
	}
	if err := deployMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("deploy mock: %v", err)
	}
	if err := judgeMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("judge mock: %v", err)
	}
}

func TestDeleteAccount_Success(t *testing.T) {
	router, accountMock, deployMock, queue := setupDeleteAccountTest(t)

	// MarkDeleted succeeds
	accountMock.ExpectExec(`UPDATE accounts SET deleted_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// GetVisibleDeploymentsByAccount returns one active deployment
	now := time.Now()
	rev := 1
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(deleteAccountDeploymentColumns).AddRow(
			"dep-1", "acct-1", nil, "my-agent", "build-1", "astro-abc-0",
			"My Agent", `{}`, nil, nil, nil,
			"active", nil, nil, now, &rev,
			now, nil, nil, nil,
		))

	// EnqueueUndeploy: UpdateStatus + InsertUndeployJob
	deployMock.ExpectBegin()
	deployMock.ExpectExec(`UPDATE`).WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_events`).WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectCommit()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/testaccount", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["message"] != "account deleted" {
		t.Errorf("expected 'account deleted', got %v", resp["message"])
	}

	if len(queue.undeployIDs) != 1 || queue.undeployIDs[0] != "dep-1" {
		t.Errorf("expected undeploy enqueued for dep-1, got %v", queue.undeployIDs)
	}

	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account mock: %v", err)
	}
	if err := deployMock.ExpectationsWereMet(); err != nil {
		t.Errorf("deploy mock: %v", err)
	}
}

func TestDeleteAccount_NoDeployments(t *testing.T) {
	router, accountMock, deployMock, queue := setupDeleteAccountTest(t)

	accountMock.ExpectExec(`UPDATE accounts SET deleted_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// No deployments
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(deleteAccountDeploymentColumns))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/testaccount", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if len(queue.undeployIDs) != 0 {
		t.Errorf("expected no undeploy jobs, got %v", queue.undeployIDs)
	}
}

func TestDeleteAccount_AlreadyDeleted(t *testing.T) {
	router, accountMock, _, _ := setupDeleteAccountTest(t)

	// MarkDeleted returns 0 rows affected
	accountMock.ExpectExec(`UPDATE accounts SET deleted_at`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/testaccount", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteAccount_MarkDeletedDBError(t *testing.T) {
	router, accountMock, _, _ := setupDeleteAccountTest(t)

	accountMock.ExpectExec(`UPDATE accounts SET deleted_at`).
		WillReturnError(fmt.Errorf("connection refused"))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/testaccount", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- UpdateAccount handler tests ---

func setupUpdateAccountRouter(accountType ...string) (*gin.Engine, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	auditDB, _, _ := sqlmock.New()
	auditStore := auditlog.NewStore(auditDB)
	log := logger.New("error", "json")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		typ := "personal"
		if len(accountType) > 0 {
			typ = accountType[0]
		}
		c.Set(string(auth.AccountContextKey), &account.Account{
			ID:   "acct-1",
			Name: "testaccount",
			Type: typ,
		})
		c.Next()
	})
	router.PATCH("/api/v1/accounts/:account", UpdateAccount(log, store, auditStore))
	return router, mock
}

func TestUpdateAccount_OrgDisplayNameTooLong(t *testing.T) {
	router, _ := setupUpdateAccountRouter("organization")
	body := fmt.Sprintf(`{"display_name":%q}`, strings.Repeat("a", account.OrganizationDisplayNameMaxLength+1))
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/testaccount", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "organization names cannot exceed 39 characters") {
		t.Fatalf("expected organization display-name limit error, got %s", rec.Body.String())
	}
}

func TestUpdateAccount_OrgDisplayNameRequiredWhenProvided(t *testing.T) {
	router, _ := setupUpdateAccountRouter("organization")
	body := `{"display_name":"   "}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/testaccount", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "organization name can't be empty") {
		t.Fatalf("expected organization display-name required error, got %s", rec.Body.String())
	}
}

func TestUpdateAccount_OrgDisplayNameCountsCharacters(t *testing.T) {
	router, mock := setupUpdateAccountRouter("organization")
	displayName := strings.Repeat("é", account.OrganizationDisplayNameMaxLength)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE accounts SET display_name`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := fmt.Sprintf(`{"display_name":%q}`, displayName)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/testaccount", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestUpdateAccount_OrgProfilePatchAllowsOmittedDisplayName(t *testing.T) {
	router, mock := setupUpdateAccountRouter("organization")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM accounts`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO account_profile`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := `{"bio":"Org bio"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/testaccount", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestUpdateAccount_TooManySocialLinks(t *testing.T) {
	router, _ := setupUpdateAccountRouter()
	body := `{"display_name":"Test","social_links":["https://a.com","https://b.com","https://c.com","https://d.com","https://e.com"]}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/testaccount", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for 5 social links, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateAccount_SocialLinksAccepted(t *testing.T) {
	router, mock := setupUpdateAccountRouter()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE accounts SET display_name`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO account_profile`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := `{"display_name":"Test","social_links":["https://github.com/sohum","https://linkedin.com/in/sohum"]}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/testaccount", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- GetAccountOrgs handler tests ---

func expectGetByName(mock sqlmock.Sqlmock, name, id string) {
	now := time.Now()
	mock.ExpectQuery("SELECT a.id, a.name, a.type").
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow(id, name, "personal", nil, nil, now, now)...))
}

func expectGetFirstMemberUserID(mock sqlmock.Sqlmock, accountID, userID string) {
	mock.ExpectQuery("SELECT user_id FROM account_members").
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(userID))
}

func TestGetAccountOrgs_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	router := gin.New()
	router.GET("/api/v1/accounts/:account/orgs", GetAccountOrgs(log, store))

	now := time.Now()
	expectGetByName(mock, "taylor", "acct-1")
	expectGetFirstMemberUserID(mock, "acct-1", "user-1")

	orgColumns := account.SQLMockScanColumns
	mock.ExpectQuery("SELECT a.id, a.name, a.type").
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows(orgColumns).
			AddRow("org-1", "astro-inc", "organization", nil, nil, now, now, "Astro Inc", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/taylor/orgs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Orgs []AccountOrgResponse `json:"orgs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Orgs) != 1 {
		t.Fatalf("expected 1 org, got %d", len(resp.Orgs))
	}
	if resp.Orgs[0].Name != "astro-inc" {
		t.Errorf("expected name 'astro-inc', got %q", resp.Orgs[0].Name)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestGetAccountOrgs_AccountNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	router := gin.New()
	router.GET("/api/v1/accounts/:account/orgs", GetAccountOrgs(log, store))

	mock.ExpectQuery("SELECT a.id, a.name, a.type").
		WithArgs("nobody").
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/nobody/orgs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetAccountOrgs_NoMembers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	router := gin.New()
	router.GET("/api/v1/accounts/:account/orgs", GetAccountOrgs(log, store))

	expectGetByName(mock, "orphan", "acct-2")
	// No member rows — GetFirstMemberUserID returns no rows
	mock.ExpectQuery("SELECT user_id FROM account_members").
		WithArgs("acct-2").
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/orphan/orgs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Orgs []AccountOrgResponse `json:"orgs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Orgs) != 0 {
		t.Errorf("expected empty orgs, got %d", len(resp.Orgs))
	}
}

func TestGetAccountOrgs_OrgAccountReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	router := gin.New()
	router.GET("/api/v1/accounts/:account/orgs", GetAccountOrgs(log, store))

	now := time.Now()
	mock.ExpectQuery("SELECT a.id, a.name, a.type").
		WithArgs("astro-inc").
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow("org-1", "astro-inc", "organization", nil, nil, now, now, "Astro Inc", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/astro-inc/orgs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for org account, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteAccount_UndeployFailureContinues(t *testing.T) {
	router, accountMock, deployMock, queue := setupDeleteAccountTest(t)
	queue.err = fmt.Errorf("queue down")

	accountMock.ExpectExec(`UPDATE accounts SET deleted_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	now := time.Now()
	rev := 1
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(deleteAccountDeploymentColumns).AddRow(
			"dep-1", "acct-1", nil, "my-agent", "build-1", "astro-abc-0",
			"My Agent", `{}`, nil, nil, nil,
			"active", nil, nil, now, &rev,
			now, nil, nil, nil,
		))

	deployMock.ExpectBegin()
	deployMock.ExpectExec(`UPDATE`).WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_events`).WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectCommit()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/testaccount", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Should still return 200 despite undeploy failure (best-effort)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 despite undeploy failure, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- blueprint_order handler tests ---

func TestUpdateAccount_BlueprintOrderAccepted(t *testing.T) {
	router, mock := setupUpdateAccountRouter()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE accounts SET display_name`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO account_profile`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	body := `{"display_name":"Test","blueprint_order":["code-reviewer","slack-bot","data-pipeline"]}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/testaccount", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

func TestUpdateAccount_BlueprintOrderEmpty(t *testing.T) {
	router, mock := setupUpdateAccountRouter()

	// No display_name → store verifies account exists then upserts profile only
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM accounts`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`INSERT INTO account_profile`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// Empty slice is valid — clears the saved order
	body := `{"blueprint_order":[]}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/accounts/testaccount", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}
