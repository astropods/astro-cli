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
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// --- deploymentNamespace tests ---

func TestDeploymentNamespace_Format(t *testing.T) {
	id := deployid.New() // e.g. "a1b-c2d-e3f"
	ns := deploymentNamespace(id)

	if !strings.HasPrefix(ns, "astro-") {
		t.Errorf("expected prefix 'astro-', got %q", ns)
	}
	// astro- (6) + 9 chars + -0 (2) = 17
	if len(ns) != 17 {
		t.Errorf("expected length 17, got %d (%q)", len(ns), ns)
	}
	if !strings.HasSuffix(ns, "-0") {
		t.Errorf("expected suffix '-0', got %q", ns)
	}
}

func TestDeploymentNamespace_Deterministic(t *testing.T) {
	id := deployid.New()
	ns1 := deploymentNamespace(id)
	ns2 := deploymentNamespace(id)
	if ns1 != ns2 {
		t.Errorf("same ID should produce same namespace: %q != %q", ns1, ns2)
	}
}

func TestDeploymentNamespace_UniquePerID(t *testing.T) {
	ns1 := deploymentNamespace(deployid.New())
	ns2 := deploymentNamespace(deployid.New())
	if ns1 == ns2 {
		t.Errorf("different IDs should produce different namespaces: %q == %q", ns1, ns2)
	}
}

// --- Undeploy handler tests ---

// setupUndeployTest creates a gin engine wired with the UndeployAgent handler.
func setupUndeployTest(t *testing.T) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	index := agentindex.NewIndexWithDB(accountDB) // not used but required

	k8sHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept all K8s API calls (namespace delete)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"kind":"Status","status":"Success"}`))
	})
	k8sClient := newMockK8sClient(k8sHandler)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.POST("/api/v1/undeploy", UndeployAgent(log, index, accountStore, nil, k8sClient, deployStore))

	return router, deployMock, accountMock
}

func TestUndeploy_Success(t *testing.T) {
	router, deployMock, accountMock := setupUndeployTest(t)

	depID := deployid.New()
	acctID := uuid.New().String()

	now := time.Now()

	// GetDeploymentByID query
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at", "undeployed_at",
		}).AddRow(
			depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "active", now, nil,
		))

	// IsMember check (SELECT COUNT(*) FROM account_members)
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// MarkUndeployedByID
	deployMock.ExpectExec(`UPDATE`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"deployment_id":"` + depID + `"}`
	req := httptest.NewRequest("POST", "/api/v1/undeploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if err := deployMock.ExpectationsWereMet(); err != nil {
		t.Logf("unfulfilled deploy expectations: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Logf("unfulfilled account expectations: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "success" {
		t.Errorf("expected status 'success', got %v", resp["status"])
	}
	if resp["name"] != "my-agent" {
		t.Errorf("expected name 'my-agent', got %v", resp["name"])
	}
}

func TestUndeploy_NotFound(t *testing.T) {
	router, deployMock, _ := setupUndeployTest(t)

	depID := deployid.New()

	// GetDeploymentByID returns no rows
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at", "undeployed_at",
		}))

	body := `{"deployment_id":"` + depID + `"}`
	req := httptest.NewRequest("POST", "/api/v1/undeploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUndeploy_InactiveDeployment(t *testing.T) {
	router, deployMock, _ := setupUndeployTest(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()
	later := now.Add(time.Hour)

	// GetDeploymentByID returns an undeployed record
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at", "undeployed_at",
		}).AddRow(
			depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "undeployed", now, later,
		))

	body := `{"deployment_id":"` + depID + `"}`
	req := httptest.NewRequest("POST", "/api/v1/undeploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for inactive deployment, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUndeploy_Forbidden(t *testing.T) {
	router, deployMock, accountMock := setupUndeployTest(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	// GetDeploymentByID returns active deployment
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at", "undeployed_at",
		}).AddRow(
			depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "active", now, nil,
		))

	// IsMember returns count=0 (user is not a member)
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	body := `{"deployment_id":"` + depID + `"}`
	req := httptest.NewRequest("POST", "/api/v1/undeploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUndeploy_NoAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deployDB, _, _ := sqlmock.New()
	accountDB, _, _ := sqlmock.New()
	deployStore := deploymentstore.NewStore(deployDB)
	accountStore := account.NewAccountStore(accountDB)
	log := logger.New("error", "json")
	index := agentindex.NewIndexWithDB(accountDB)

	router := gin.New()
	// No auth middleware — user not set
	router.POST("/api/v1/undeploy", UndeployAgent(log, index, accountStore, nil, nil, deployStore))

	body := `{"deployment_id":"some-id"}`
	req := httptest.NewRequest("POST", "/api/v1/undeploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUndeploy_MissingDeploymentID(t *testing.T) {
	router, _, _ := setupUndeployTest(t)

	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/undeploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
