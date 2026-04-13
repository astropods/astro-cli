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
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

var knowledgeColumns = []string{
	"id", "account_id", "name", "arn", "provider", "status", "namespace", "storage",
	"public", "public_host", "encrypted_data_key", "kms_key_arn", "error",
	"created_at", "updated_at",
}

func knowledgeRow(id, accountID, name, provider, status string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(knowledgeColumns).AddRow(
		id, accountID, name,
		"arn:knowledge:acme:"+name,
		provider, status,
		"knowledge-"+accountID, "10Gi",
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

	var resp knowledgeResponse
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

	var resp []knowledgeResponse
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
			"arn:knowledge:acme:"+name, "qdrant", "ready",
			"knowledge-"+testAccount().ID, "10Gi",
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

	var resp []knowledgeResponse
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

	var resp knowledgeResponse
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
// use the immutable account ID, not the account name which can change.
func TestCreateKnowledgeStore_ARN_UsesAccountID(t *testing.T) {
	router, ksStore, mock := setupKS()
	log := logger.New("error", "json")
	router.POST("/knowledge", CreateKnowledgeStore(log, ksStore, nil, minimalCfg()))

	acct := testAccount()
	expectedARN := "arn:knowledge:" + acct.ID + ":pg-main"

	// WithArgs verifies the ARN passed to INSERT uses account ID (arg $4), not account name.
	mock.ExpectQuery("INSERT INTO knowledge_stores").
		WithArgs(
			sqlmock.AnyArg(), // $1: store ID
			acct.ID,          // $2: account ID
			"pg-main",        // $3: name
			expectedARN,      // $4: ARN — must use account ID, not name
			"postgres",       // $5: provider
			sqlmock.AnyArg(), // $6: namespace
			"10Gi",           // $7: storage
			false,            // $8: public
			sqlmock.AnyArg(), // $9: public_host
			sqlmock.AnyArg(), // $10: encrypted_data_key
			sqlmock.AnyArg(), // $11: kms_key_arn
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
