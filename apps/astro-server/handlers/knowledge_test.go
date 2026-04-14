package handlers

import (
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
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

var knowledgeColumns = []string{
	"id", "account_id", "name", "arn", "provider", "mode", "status", "storage", "storage_class",
	"public", "public_host", "encrypted_data_key", "kms_key_arn", "error",
	"created_at", "updated_at",
}

func knowledgeRow(id, accountID, name, provider, status string) *sqlmock.Rows {
	return knowledgeRowWithMode(id, accountID, name, provider, "managed", status)
}

func externalKnowledgeRow(id, accountID, name, provider, status string) *sqlmock.Rows {
	return knowledgeRowWithMode(id, accountID, name, provider, "external", status)
}

func knowledgeRowWithMode(id, accountID, name, provider, mode, status string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(knowledgeColumns).AddRow(
		id, accountID, name,
		"arn:knowledge:acme:"+name,
		provider, mode, status,
		"10Gi", nil, // storage, storage_class
		false, nil, nil, nil, nil,
		now, now,
	)
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

func minimalCfg() *config.Config {
	return &config.Config{}
}

// --- CreateKnowledgeStore ---

func TestCreateKnowledgeStore_InvalidProvider(t *testing.T) {
	router, ksStore, _ := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge", CreateKnowledgeStore(log, ksStore, nil, minimalCfg()))

	body := `{"name":"mystore","provider":"nonexistent"}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateKnowledgeStore_MissingName(t *testing.T) {
	router, ksStore, _ := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge", CreateKnowledgeStore(log, ksStore, nil, minimalCfg()))

	body := `{"provider":"postgres"}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateKnowledgeStore_Conflict(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge", CreateKnowledgeStore(log, ksStore, nil, minimalCfg()))

	mock.ExpectQuery("INSERT INTO knowledge_stores").
		WillReturnError(&pq.Error{Code: "23505", Message: "duplicate key"})

	body := `{"name":"pg-main","provider":"postgres"}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateKnowledgeStore_DBError(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge", CreateKnowledgeStore(log, ksStore, nil, minimalCfg()))

	mock.ExpectQuery("INSERT INTO knowledge_stores").
		WillReturnError(sqlmock.ErrCancelled)

	body := `{"name":"pg-main","provider":"postgres"}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateKnowledgeStore_Success_NoKMS(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	// No k8sClient (nil) and no KMS — provisioning skipped, store created.
	router.POST("/knowledge", CreateKnowledgeStore(log, ksStore, nil, minimalCfg()))

	mock.ExpectQuery("INSERT INTO knowledge_stores").
		WillReturnRows(knowledgeRow("abc-def-ghi", testAccount().ID, "pg-main", "postgres", "provisioning"))

	body := `{"name":"pg-main","provider":"postgres","storage":"20Gi"}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp KnowledgeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Provider != "postgres" {
		t.Errorf("expected provider 'postgres', got %q", resp.Provider)
	}
	if resp.Status != knowledgestore.StatusProvisioning {
		t.Errorf("expected status %q, got %q", knowledgestore.StatusProvisioning, resp.Status)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations: %v", err)
	}
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
			"10Gi", nil, // storage, storage_class
			false, nil, nil, nil, nil, now, now,
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

	router.GET("/knowledge/:name", GetKnowledgeStore(log, ksStore, nil))

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

	router.GET("/knowledge/:name", GetKnowledgeStore(log, ksStore, nil))

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
	mock.ExpectExec("DELETE FROM knowledge_stores").WillReturnResult(sqlmock.NewResult(1, 1))

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

	router.GET("/knowledge/:name/credentials", GetKnowledgeStoreCredentials(log, ksStore))

	mock.ExpectQuery("SELECT .+ FROM knowledge_stores WHERE account_id").
		WillReturnRows(knowledgeRow("abc-def-ghi", testAccount().ID, "pg-main", "postgres", "ready"))
	// GetCredentials returns empty rows — KMS was not configured at creation.
	mock.ExpectQuery("SELECT key, value_encrypted, nonce FROM knowledge_store_credentials").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value_encrypted", "nonce"}))

	req := httptest.NewRequest(http.MethodGet, "/knowledge/pg-main/credentials", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 when no credentials stored, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- Name / storage validation ---

func TestCreateKnowledgeStore_InvalidName(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"starts with hyphen", `{"name":"-bad","provider":"postgres"}`},
		{"ends with hyphen", `{"name":"bad-","provider":"postgres"}`},
		{"uppercase", `{"name":"MyStore","provider":"postgres"}`},
		{"too long", `{"name":"` + strings.Repeat("a", 64) + `","provider":"postgres"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, ksStore, _ := setupKS()
			log := logger.New("error", "json")
			router.POST("/knowledge", CreateKnowledgeStore(log, ksStore, nil, minimalCfg()))

			req := httptest.NewRequest(http.MethodPost, "/knowledge", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateKnowledgeStore_InvalidStorage(t *testing.T) {
	router, ksStore, _ := setupKS()
	log := logger.New("error", "json")
	router.POST("/knowledge", CreateKnowledgeStore(log, ksStore, nil, minimalCfg()))

	body := `{"name":"my-db","provider":"postgres","storage":"notasize"}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCreateKnowledgeStore_ARN_UsesAccountID is a regression test: ARNs must
// use the short account ID (FNV-64a hash), not the account name which can change.
func TestCreateKnowledgeStore_ARN_UsesAccountID(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")
	router.POST("/knowledge", CreateKnowledgeStore(log, ksStore, nil, minimalCfg()))

	acct := testAccount()
	expectedARN := arn.KnowledgeStore(acct.ID, "pg-main")

	// WithArgs verifies the ARN passed to INSERT uses the short account ID (arg $4), not the name.
	mock.ExpectQuery("INSERT INTO knowledge_stores").
		WithArgs(
			sqlmock.AnyArg(), // $1: store ID
			acct.ID,          // $2: account ID
			"pg-main",        // $3: name
			expectedARN,      // $4: ARN — must use short account ID, not name
			"postgres",       // $5: provider
			"managed",        // $6: mode
			"provisioning",   // $7: status
			"10Gi",           // $8: storage
			sqlmock.AnyArg(), // $9: storage_class (nil)
			false,            // $10: public
			sqlmock.AnyArg(), // $11: public_host
			sqlmock.AnyArg(), // $12: encrypted_data_key
			sqlmock.AnyArg(), // $13: kms_key_arn
		).
		WillReturnRows(knowledgeRow(acct.ID, acct.ID, "pg-main", "postgres", "provisioning"))

	body := `{"name":"pg-main","provider":"postgres"}`
	req := httptest.NewRequest(http.MethodPost, "/knowledge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled mock expectations (ARN arg check failed): %v", err)
	}
}

// --- ConnectKnowledgeStore ---

func TestConnectKnowledgeStore_Success(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, minimalCfg()))

	mock.ExpectQuery("INSERT INTO knowledge_stores").
		WillReturnRows(externalKnowledgeRow("ext-abc-def", testAccount().ID, "postgres-prod", "postgres", "ready"))
	// No KMS configured in minimalCfg() — SaveCredentials is skipped.

	body := `{"name":"postgres-prod","provider":"postgres","host":"db.example.com","port":5432,"database":"vectors","username":"app","password":"secret"}`
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

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, minimalCfg()))

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

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, minimalCfg()))

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

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, minimalCfg()))

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

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, minimalCfg()))

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

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, minimalCfg()))

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

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, minimalCfg()))

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

	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, minimalCfg()))

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
	router.POST("/knowledge/connect", ConnectKnowledgeStore(log, ksStore, minimalCfg()))

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
			"",               // $8: storage (empty for external — DB default applies)
			sqlmock.AnyArg(), // $9: storage_class (nil)
			false,            // $10: public
			sqlmock.AnyArg(), // $11: public_host
			sqlmock.AnyArg(), // $12: encrypted_data_key
			sqlmock.AnyArg(), // $13: kms_key_arn
		).
		WillReturnRows(externalKnowledgeRow(acct.ID, acct.ID, "pg-prod", "postgres", "ready"))
	// No KMS configured in minimalCfg() — SaveCredentials is skipped.

	body := `{"name":"pg-prod","provider":"postgres","host":"db.example.com","port":5432,"database":"vectors","username":"app","password":"secret"}`
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
	mock.ExpectExec("DELETE FROM knowledge_stores").WillReturnResult(sqlmock.NewResult(1, 1))

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
		"20Gi", nil, false, nil, nil, nil, nil, now, now,
	)
	rows.AddRow(
		"id-external", testAccount().ID, "pg-prod",
		"arn:knowledge:acme:pg-prod", "postgres", "external", "ready",
		"10Gi", nil, false, nil, nil, nil, nil, now, now,
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
