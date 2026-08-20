package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/arn"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

var knowledgeColumns = []string{
	"id", "account_id", "name", "arn", "provider", "mode", "status",
	"encrypted_data_key", "kms_key_arn", "error", "annotations",
	"created_at", "updated_at",
}

func knowledgeRow(id, accountID, name, provider, status string) *sqlmock.Rows {
	return knowledgeRowWithMode(id, accountID, name, provider, "managed", status)
}

func externalKnowledgeRow(id, accountID, name, provider, status string) *sqlmock.Rows {
	return knowledgeRowWithMode(id, accountID, name, provider, "external", status)
}

// supabaseExternalRow is an external store carrying the source=supabase origin
// annotation, used to exercise the Supabase-managed-field guards.
func supabaseExternalRow(id, accountID, name string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(knowledgeColumns).AddRow(
		id, accountID, name,
		"arn:knowledge:acme:"+name,
		"postgres", "external", "error",
		nil, nil, "health check failed",
		[]byte(`{"source":"supabase"}`),
		now, now,
	)
}

func knowledgeRowWithMode(id, accountID, name, provider, mode, status string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(knowledgeColumns).AddRow(
		id, accountID, name,
		"arn:knowledge:acme:"+name,
		provider, mode, status,
		nil, nil, nil, // encrypted_data_key, kms_key_arn, error
		nil,      // annotations
		now, now, // created_at, updated_at
	)
}

func minimalCfg() *config.Config {
	return &config.Config{Deployment: config.DeploymentConfig{}}
}

// injectAccount sets the resolved account on the gin context, simulating the
// accountMember middleware that runs before knowledge handlers in production.
func injectAccount(acct *account.Account) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(string(auth.AccountContextKey), acct)
		c.Next()
	}
}

func testAccount() *account.Account {
	return &account.Account{
		ID:   "acct-00000000-0000-0000-0000-000000000001",
		Name: "acme",
		Type: "personal",
	}
}

func setupKS() (*gin.Engine, *knowledgestore.Store, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)
	rawDB, mock, _ := sqlmock.New()
	ksStore := knowledgestore.NewStore(rawDB)
	router := gin.New()
	router.Use(injectAccount(testAccount()))
	return router, ksStore, mock
}

// --- ListKnowledgeStores ---

func TestListKnowledgeStores_Empty(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.GET("/knowledge", ListKnowledgeStores(log, ksStore))

	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(sqlmock.NewRows(knowledgeColumns))

	req := httptest.NewRequest(http.MethodGet, "/knowledge", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp []KnowledgeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 0 {
		t.Errorf("expected empty list, got %d items", len(resp))
	}
}

func TestListKnowledgeStores_WithItems(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.GET("/knowledge", ListKnowledgeStores(log, ksStore))

	rows := sqlmock.NewRows(knowledgeColumns)
	now := time.Now()
	for _, name := range []string{"store-a", "store-b"} {
		rows.AddRow(
			"id-"+name, testAccount().ID, name,
			"arn:knowledge:acme:"+name, "qdrant", "managed", "ready",
			nil, nil, nil, nil, now, now,
		)
	}
	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/knowledge", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp []KnowledgeResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp))
	}
}

// --- GetKnowledgeStore ---

func TestGetKnowledgeStore_Found(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.GET("/knowledge/:name", GetKnowledgeStore(log, ksStore))

	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(knowledgeRow("abc-def-ghi", testAccount().ID, "pg-main", "postgres", "ready"))

	req := httptest.NewRequest(http.MethodGet, "/knowledge/pg-main", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp KnowledgeResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Name != "pg-main" {
		t.Errorf("expected name 'pg-main', got %q", resp.Name)
	}
	if resp.Status != "ready" {
		t.Errorf("expected status 'ready', got %q", resp.Status)
	}
}

func TestGetKnowledgeStore_NotFound(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.GET("/knowledge/:name", GetKnowledgeStore(log, ksStore))

	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(sqlmock.NewRows(knowledgeColumns))

	req := httptest.NewRequest(http.MethodGet, "/knowledge/nope", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- DeleteKnowledgeStore ---

func TestDeleteKnowledgeStore_NotFound(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.DELETE("/knowledge/:name", DeleteKnowledgeStore(log, ksStore, nil))

	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(sqlmock.NewRows(knowledgeColumns))

	req := httptest.NewRequest(http.MethodDelete, "/knowledge/nope", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteKnowledgeStore_NoK8s(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.DELETE("/knowledge/:name", DeleteKnowledgeStore(log, ksStore, nil))

	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(knowledgeRow("abc-def-ghi", testAccount().ID, "pg-main", "postgres", "ready"))
	mock.ExpectQuery(`SELECT b\.deployment_id, d\.agent_name, d\.display_name, b\.knowledge_name`).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "agent_name", "display_name", "knowledge_name"}))
	mock.ExpectQuery("DELETE FROM knowledge_stores").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(testAccount().ID))

	req := httptest.NewRequest(http.MethodDelete, "/knowledge/pg-main", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

// --- GetKnowledgeStoreCredentials ---

func TestGetKnowledgeStoreCredentials_NoKMS(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.GET("/knowledge/:name/credentials", GetKnowledgeStoreCredentials(log, ksStore, testVault(t)))

	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(knowledgeRow("abc-def-ghi", testAccount().ID, "pg-main", "postgres", "ready"))
	// GetCredentials returns empty rows — KMS was not configured at creation.
	mock.ExpectQuery("SELECT key, value_encrypted, nonce FROM knowledge_store_credentials").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value_encrypted", "nonce"}))

	req := httptest.NewRequest(http.MethodGet, "/knowledge/pg-main/credentials", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when no KMS and no secret reader, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- Name / storage validation ---

// --- ConnectKnowledgeStore ---

func TestConnectKnowledgeStore_Success(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, nil, minimalCfg(), testVault(t), nil, nil, nil))

	mock.ExpectQuery("INSERT INTO knowledge_stores").
		WillReturnRows(externalKnowledgeRow("ext-abc-def", testAccount().ID, "postgres-prod", "postgres", "ready"))
	expectCredentialSave(mock, 5)

	body := `{"name":"postgres-prod","provider":"postgres","host":"db.example.com","port":5432,"database":"vectors","username":"app","password":"secret","skip_health_check":true}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp KnowledgeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Mode != knowledgestore.ModeExternal {
		t.Errorf("expected mode %q, got %q", knowledgestore.ModeExternal, resp.Mode)
	}
	if resp.Status != knowledgestore.StatusReady {
		t.Errorf("expected status %q, got %q", knowledgestore.StatusReady, resp.Status)
	}
	if resp.Provider != "postgres" {
		t.Errorf("expected provider 'postgres', got %q", resp.Provider)
	}
}

func TestConnectKnowledgeStore_MissingHost(t *testing.T) {
	router, ksStore, _ := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, nil, minimalCfg(), testVault(t), nil, nil, nil))

	body := `{"name":"pg-prod","provider":"postgres","port":5432}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing host, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestConnectKnowledgeStore_MissingPort(t *testing.T) {
	router, ksStore, _ := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, nil, minimalCfg(), testVault(t), nil, nil, nil))

	body := `{"name":"pg-prod","provider":"postgres","host":"db.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing port, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestConnectKnowledgeStore_InvalidProvider(t *testing.T) {
	router, ksStore, _ := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, nil, minimalCfg(), testVault(t), nil, nil, nil))

	body := `{"name":"my-store","provider":"cassandra","host":"db.example.com","port":9042}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unsupported provider, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestConnectKnowledgeStore_InvalidName(t *testing.T) {
	router, ksStore, _ := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, nil, minimalCfg(), testVault(t), nil, nil, nil))

	body := `{"name":"My-Store","provider":"postgres","host":"db.example.com","port":5432}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid name, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestConnectKnowledgeStore_MissingCredentials(t *testing.T) {
	router, ksStore, _ := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, nil, minimalCfg(), testVault(t), nil, nil, nil))

	// Postgres requires PASSWORD but it's not provided.
	body := `{"name":"pg-prod","provider":"postgres","host":"db.example.com","port":5432,"database":"mydb","username":"app"}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing credentials, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestConnectKnowledgeStore_Conflict(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, nil, minimalCfg(), testVault(t), nil, nil, nil))

	mock.ExpectQuery("INSERT INTO knowledge_stores").
		WillReturnError(&pq.Error{Code: "23505", Message: "duplicate key"})

	body := `{"name":"pg-prod","provider":"postgres","host":"db.example.com","port":5432,"database":"vectors","username":"app","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestConnectKnowledgeStore_DBError(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, nil, minimalCfg(), testVault(t), nil, nil, nil))

	mock.ExpectQuery("INSERT INTO knowledge_stores").
		WillReturnError(sqlmock.ErrCancelled)

	body := `{"name":"pg-prod","provider":"postgres","host":"db.example.com","port":5432,"database":"vectors","username":"app","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestConnectKnowledgeStore_ARNUsesAccountID(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")
	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, nil, minimalCfg(), testVault(t), nil, nil, nil))

	acct := testAccount()
	expectedARN := arn.KnowledgeStore(acct.ID, "pg-prod")

	mock.ExpectQuery("INSERT INTO knowledge_stores").
		WithArgs(
			sqlmock.AnyArg(), // $1: store ID
			acct.ID,          // $2: account ID
			"pg-prod",        // $3: name
			expectedARN,      // $4: ARN
			"postgres",       // $5: provider
			"external",       // $6: mode
			"ready",          // $7: status
			sqlmock.AnyArg(), // $8: encrypted_data_key
			sqlmock.AnyArg(), // $9: kms_key_arn
			sqlmock.AnyArg(), // $10: annotations
		).
		WillReturnRows(externalKnowledgeRow(acct.ID, acct.ID, "pg-prod", "postgres", "ready"))
	expectCredentialSave(mock, 5)

	body := `{"name":"pg-prod","provider":"postgres","host":"db.example.com","port":5432,"database":"vectors","username":"app","password":"secret","skip_health_check":true}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge/connect", strings.NewReader(body))
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

// --- DeleteKnowledgeStore (external) ---

func TestDeleteKnowledgeStore_ExternalSkipsK8s(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	// Pass nil for k8sClient — if the handler tried to use it for an external store, it would panic.
	router.DELETE("/knowledge/:name", DeleteKnowledgeStore(log, ksStore, nil))

	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(externalKnowledgeRow("ext-abc-def", testAccount().ID, "pg-prod", "postgres", "ready"))
	mock.ExpectQuery(`SELECT b\.deployment_id, d\.agent_name, d\.display_name, b\.knowledge_name`).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "agent_name", "display_name", "knowledge_name"}))
	mock.ExpectQuery("DELETE FROM knowledge_stores").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(testAccount().ID))

	req := httptest.NewRequest(http.MethodDelete, "/knowledge/pg-prod", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
}

// --- ListKnowledgeStores (mixed modes) ---

func TestListKnowledgeStores_MixedModes(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.GET("/knowledge", ListKnowledgeStores(log, ksStore))

	rows := sqlmock.NewRows(knowledgeColumns)
	now := time.Now()
	rows.AddRow(
		"id-managed", testAccount().ID, "pg-main",
		"arn:knowledge:acme:pg-main", "postgres", "managed", "ready",
		nil, nil, nil, nil, now, now,
	)
	rows.AddRow(
		"id-external", testAccount().ID, "pg-prod",
		"arn:knowledge:acme:pg-prod", "postgres", "external", "ready",
		nil, nil, nil, nil, now, now,
	)
	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/knowledge", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp []KnowledgeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp))
	}

	modes := map[string]bool{}
	for _, r := range resp {
		modes[r.Mode] = true
	}
	if !modes["managed"] || !modes["external"] {
		t.Errorf("expected both managed and external modes, got %v", modes)
	}
}

// --- Entitlement checks ---

// disabledQuotaChecker simulates a resource disabled for the account (limit 0 →
// FEATURE_NOT_IN_PLAN).
type disabledQuotaChecker struct{ resource string }

func (d *disabledQuotaChecker) Check(_ context.Context, _ string, _ ...string) (quota.Result, error) {
	return quota.Result{Blocked: true, Resource: d.resource, Limit: 0}, nil
}

// exceededQuotaChecker simulates a resource over its limit (ENTITLEMENT_LIMIT_REACHED).
type exceededQuotaChecker struct{ resource string }

func (e *exceededQuotaChecker) Check(_ context.Context, _ string, _ ...string) (quota.Result, error) {
	return quota.Result{Blocked: true, Resource: e.resource, Limit: 5, Used: 5}, nil
}

func assertEntitlementResponse(t *testing.T, rec *httptest.ResponseRecorder, wantCode int, wantEntCode string, wantDetailsSubstr string) {
	t.Helper()
	if rec.Code != wantCode {
		t.Errorf("expected HTTP %d, got %d: %s", wantCode, rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["code"] != wantEntCode {
		t.Errorf("expected code %q, got %v", wantEntCode, resp["code"])
	}
	if details, _ := resp["details"].(string); !strings.Contains(details, wantDetailsSubstr) {
		t.Errorf("expected details to contain %q, got %q", wantDetailsSubstr, details)
	}
}

func TestConnectKnowledgeStore_EntitlementBlocked_KnowledgeEndpoints(t *testing.T) {
	privateLinkCfg := &config.Config{
		Deployment: config.DeploymentConfig{
			PrivateLinkVpcID: "vpc-12345678",
		},
	}
	privateLinkBody := `{"name":"pg-prod","provider":"postgres","host":"com.amazonaws.vpce.us-east-1.vpce-svc-abc123","port":5432,"database":"vectors","username":"app","password":"secret","private_link":true}`

	t.Run("not_in_plan", func(t *testing.T) {
		router, ksStore, mock := setupKS()
		log := logger.New("error", "json")
		router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, nil, privateLinkCfg, testVault(t), nil, nil, &disabledQuotaChecker{resource: "knowledge_endpoints"}))
		mock.ExpectQuery("INSERT INTO knowledge_stores").
			WillReturnRows(externalKnowledgeRow("ext-abc-def", testAccount().ID, "pg-prod", "postgres", "ready"))
		expectCredentialSave(mock, 5)

		req := httptest.NewRequest(http.MethodPost, "/knowledge/connect", strings.NewReader(privateLinkBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assertEntitlementResponse(t, rec, http.StatusPaymentRequired, "FEATURE_NOT_IN_PLAN", "not included in your current plan")
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})

	t.Run("quota_exceeded", func(t *testing.T) {
		router, ksStore, mock := setupKS()
		log := logger.New("error", "json")
		router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, nil, privateLinkCfg, testVault(t), nil, nil, &exceededQuotaChecker{resource: "knowledge_endpoints"}))
		mock.ExpectQuery("INSERT INTO knowledge_stores").
			WillReturnRows(externalKnowledgeRow("ext-abc-def", testAccount().ID, "pg-prod", "postgres", "ready"))
		expectCredentialSave(mock, 5)

		req := httptest.NewRequest(http.MethodPost, "/knowledge/connect", strings.NewReader(privateLinkBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assertEntitlementResponse(t, rec, http.StatusPaymentRequired, "ENTITLEMENT_LIMIT_REACHED", "limit reached")
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled mock expectations: %v", err)
		}
	})
}

// --- UpdateKnowledgeStoreCredentials ---

func updateCredsRouter(t *testing.T) (*gin.Engine, *knowledgestore.Store, sqlmock.Sqlmock) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")
	router.PUT("/knowledge/:name/credentials", UpdateKnowledgeStoreCredentials(log, ksStore, nil, minimalCfg(), testVault(t)))
	return router, ksStore, mock
}

func doUpdateCreds(router *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/knowledge/pg-prod/credentials", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestUpdateKnowledgeStoreCredentials_NotFound(t *testing.T) {
	router, _, mock := updateCredsRouter(t)
	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(sqlmock.NewRows(knowledgeColumns))

	rec := doUpdateCreds(router, `{"password":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateKnowledgeStoreCredentials_NoFields(t *testing.T) {
	router, _, mock := updateCredsRouter(t)
	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(externalKnowledgeRow("ext-1", testAccount().ID, "pg-prod", "postgres", "ready"))

	rec := doUpdateCreds(router, `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty update, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no credential fields") {
		t.Errorf("expected 'no credential fields' error, got %s", rec.Body.String())
	}
}

func TestUpdateKnowledgeStoreCredentials_RejectsSupabaseManagedField(t *testing.T) {
	router, _, mock := updateCredsRouter(t)
	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(supabaseExternalRow("ext-sb", testAccount().ID, "pg-prod"))

	rec := doUpdateCreds(router, `{"host":"evil.example.com"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for supabase host edit, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "managed by Supabase") {
		t.Errorf("expected supabase-managed error, got %s", rec.Body.String())
	}
}

func TestUpdateKnowledgeStoreCredentials_NoStoredCredentials(t *testing.T) {
	router, _, mock := updateCredsRouter(t)
	// externalKnowledgeRow has a nil encrypted_data_key.
	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(externalKnowledgeRow("ext-1", testAccount().ID, "pg-prod", "postgres", "error"))

	rec := doUpdateCreds(router, `{"password":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for store without data key, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no stored credentials") {
		t.Errorf("expected 'no stored credentials' error, got %s", rec.Body.String())
	}
}

// --- No account in context ---

func TestKnowledgeHandlers_NoAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rawDB, _, _ := sqlmock.New()
	ksStore := knowledgestore.NewStore(rawDB)
	log := logger.New("error", "json")

	router := gin.New()
	// No injectAccount middleware — context has no account.
	router.GET("/knowledge", ListKnowledgeStores(log, ksStore))

	req := httptest.NewRequest(http.MethodGet, "/knowledge", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when no account in context, got %d", rec.Code)
	}
}

func expectCredentialSave(mock sqlmock.Sqlmock, credentials int) {
	mock.ExpectBegin()
	for i := 0; i < credentials; i++ {
		mock.ExpectExec("INSERT INTO knowledge_store_credentials").
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
}
