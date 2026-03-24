package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
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

var accountColumns = []string{"id", "name", "type", "created_at", "updated_at"}

func TestSearchAccounts_Success(t *testing.T) {
	router, store, mock := setupAccountTestRouter()
	log := logger.New("error", "json")

	router.GET("/api/v1/accounts/search", SearchAccounts(log, store))

	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("foo%", 10).
		WillReturnRows(sqlmock.NewRows(accountColumns).
			AddRow("id-1", "foobar", "personal", now, now).
			AddRow("id-2", "foocorp", "organization", now, now))

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
			AddRow("id-1", "barry", "personal", now, now))

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
			AddRow("id-1", "abcdef", "personal", now, now))

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

// --- DeleteAccount handler tests ---

// deleteAccountMockQueue tracks calls to InsertUndeployJob.
type deleteAccountMockQueue struct {
	undeployIDs []string
	err         error // if non-nil, InsertUndeployJob returns this
}

func (q *deleteAccountMockQueue) InsertDeployJob(_ context.Context, _ string) error { return nil }
func (q *deleteAccountMockQueue) InsertUndeployJob(_ context.Context, id string) error {
	q.undeployIDs = append(q.undeployIDs, id)
	return q.err
}
func (q *deleteAccountMockQueue) InsertWakeUpJob(_ context.Context, _ string) error { return nil }

func setupDeleteAccountTest(t *testing.T) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock, *deleteAccountMockQueue) {
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
	router.DELETE("/api/v1/accounts/:account", DeleteAccount(log, accountStore, deployStore, queue, nil))

	return router, accountMock, deployMock, queue
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
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "avatar_version", "deployment_spec_json", "encrypted_data_key", "kms_key_arn",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at",
		}).AddRow(
			"dep-1", "acct-1", "my-agent", "build-1", "astro-abc-0",
			"My Agent", 0, `{}`, nil, nil,
			"active", nil, nil, now, &rev,
			now, nil,
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
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "avatar_version", "deployment_spec_json", "encrypted_data_key", "kms_key_arn",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at",
		}))

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

func TestDeleteAccount_UndeployFailureContinues(t *testing.T) {
	router, accountMock, deployMock, queue := setupDeleteAccountTest(t)
	queue.err = fmt.Errorf("queue down")

	accountMock.ExpectExec(`UPDATE accounts SET deleted_at`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	now := time.Now()
	rev := 1
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "avatar_version", "deployment_spec_json", "encrypted_data_key", "kms_key_arn",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at",
		}).AddRow(
			"dep-1", "acct-1", "my-agent", "build-1", "astro-abc-0",
			"My Agent", 0, `{}`, nil, nil,
			"active", nil, nil, now, &rev,
			now, nil,
		))

	// UpdateStatus succeeds but InsertUndeployJob fails — handler should still return 200
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
