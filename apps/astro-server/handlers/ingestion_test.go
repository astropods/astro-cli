package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/auth"
	"github.com/postman/astro/apps/astro-server/internal/k8s"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// mockClusterClient implements k8s.ClusterClient for testing, backed by a real
// kubernetes.Clientset that points at a local httptest server.
type mockClusterClient struct {
	clientset *kubernetes.Clientset
}

func (m *mockClusterClient) Clientset() *kubernetes.Clientset  { return m.clientset }
func (m *mockClusterClient) Config() *rest.Config               { return nil }
func (m *mockClusterClient) CheckHealth() error                 { return nil }
func (m *mockClusterClient) GetServerVersion() (string, error)  { return "v1.30.0", nil }
func (m *mockClusterClient) DiagnoseConnection() map[string]string { return nil }

// newMockK8sClient spins up a fake API server and returns a ClusterClient
// whose Clientset points at it. The handler func receives all K8s API requests
// so the test can return whatever resources it needs.
func newMockK8sClient(handler http.Handler) k8s.ClusterClient {
	srv := httptest.NewServer(handler)
	cs, _ := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	return &mockClusterClient{clientset: cs}
}

// setAuthUser injects an authenticated user into the gin context (same
// mechanism as the real auth middleware).
func setAuthUser(userID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: userID})
		c.Next()
	}
}

// setupIngestionRouter creates a gin engine wired up with the
// TriggerIngestion handler and all its dependencies backed by sqlmock /
// the provided k8sClient.
func setupIngestionRouter(
	k8sClient k8s.ClusterClient,
	withAuth bool,
) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New()
	indexDB, indexMock, _ := sqlmock.New()

	accountStore := account.NewAccountStore(accountDB)
	index := agentindex.NewIndexWithDB(indexDB)
	log := logger.New("error", "json")

	router := gin.New()
	if withAuth {
		router.Use(setAuthUser("user-1"))
	}
	router.POST(
		"/api/v1/deployments/:namespace/ingestion/:ingestion/trigger",
		TriggerIngestion(log, index, accountStore, k8sClient),
	)
	return router, accountMock, indexMock
}

func TestTriggerIngestion_NoAuth(t *testing.T) {
	router, _, _ := setupIngestionRouter(nil, false)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/deployments/ns/ingestion/data/trigger?account=acme", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
}

func TestTriggerIngestion_MissingAccount(t *testing.T) {
	router, _, _ := setupIngestionRouter(nil, true)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/deployments/ns/ingestion/data/trigger", nil) // no account param
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestTriggerIngestion_AccountNotFound(t *testing.T) {
	router, accountMock, _ := setupIngestionRouter(nil, true)

	accountMock.ExpectQuery("SELECT .+ FROM accounts WHERE name").
		WithArgs("unknown").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/deployments/ns/ingestion/data/trigger?account=unknown", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected %d, got %d: %s", http.StatusNotFound, rec.Code, rec.Body.String())
	}
}

func TestTriggerIngestion_NotMember(t *testing.T) {
	router, accountMock, _ := setupIngestionRouter(nil, true)

	accountMock.ExpectQuery("SELECT .+ FROM accounts WHERE name").
		WithArgs("acme").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
			AddRow("acct-1", "acme", "team", time.Now(), time.Now()))

	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/deployments/ns/ingestion/data/trigger?account=acme", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected %d, got %d: %s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
}

func TestTriggerIngestion_NilK8sClient(t *testing.T) {
	router, accountMock, _ := setupIngestionRouter(nil, true)

	accountMock.ExpectQuery("SELECT .+ FROM accounts WHERE name").
		WithArgs("acme").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
			AddRow("acct-1", "acme", "team", time.Now(), time.Now()))
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/deployments/ns/ingestion/data/trigger?account=acme", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected %d, got %d: %s", http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	}
}

func TestTriggerIngestion_NotManualTrigger(t *testing.T) {
	// Fake K8s API: return namespace with correct labels
	k8sHandler := http.NewServeMux()
	k8sHandler.HandleFunc("/api/v1/namespaces/test-ns", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"apiVersion": "v1", "kind": "Namespace",
			"metadata": {
				"name": "test-ns",
				"labels": {
					"astro.dev/account-id": "acct-1",
					"astro.dev/agent": "my-agent",
					"astro.dev/build": "build-1"
				}
			}
		}`)
	})
	k8sClient := newMockK8sClient(k8sHandler)
	router, accountMock, indexMock := setupIngestionRouter(k8sClient, true)

	// account + membership
	accountMock.ExpectQuery("SELECT .+ FROM accounts WHERE name").
		WithArgs("acme").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
			AddRow("acct-1", "acme", "team", time.Now(), time.Now()))
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agent index – ingestion with schedule trigger (not manual)
	specJSON := `{"ingestion":{"data":{"container":{"image":"img"},"trigger":{"type":"schedule"}}}}`
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions").
		WithArgs("acct-1", "my-agent", "build-1").
		WillReturnRows(sqlmock.NewRows([]string{"build_id", "spec_json", "readme", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", specJSON, "", "[]", time.Now(), time.Now()))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/deployments/test-ns/ingestion/data/trigger?account=acme", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if errMsg, ok := resp["error"].(string); !ok || errMsg == "" {
		t.Errorf("expected error about trigger type, got %v", resp["error"])
	}
}

func TestTriggerIngestion_IngestionNotInSpec(t *testing.T) {
	k8sHandler := http.NewServeMux()
	k8sHandler.HandleFunc("/api/v1/namespaces/test-ns", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"apiVersion": "v1", "kind": "Namespace",
			"metadata": {
				"name": "test-ns",
				"labels": {
					"astro.dev/account-id": "acct-1",
					"astro.dev/agent": "my-agent",
					"astro.dev/build": "build-1"
				}
			}
		}`)
	})
	k8sClient := newMockK8sClient(k8sHandler)
	router, accountMock, indexMock := setupIngestionRouter(k8sClient, true)

	accountMock.ExpectQuery("SELECT .+ FROM accounts WHERE name").
		WithArgs("acme").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
			AddRow("acct-1", "acme", "team", time.Now(), time.Now()))
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// spec has no ingestion entries
	specJSON := `{"agent":{"image":"img"}}`
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions").
		WithArgs("acct-1", "my-agent", "build-1").
		WillReturnRows(sqlmock.NewRows([]string{"build_id", "spec_json", "readme", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", specJSON, "", "[]", time.Now(), time.Now()))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/deployments/test-ns/ingestion/data/trigger?account=acme", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected %d, got %d: %s", http.StatusNotFound, rec.Code, rec.Body.String())
	}
}

func TestTriggerIngestion_Success(t *testing.T) {
	var createdJobName string

	k8sHandler := http.NewServeMux()
	k8sHandler.HandleFunc("/api/v1/namespaces/test-ns", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"apiVersion": "v1", "kind": "Namespace",
			"metadata": {
				"name": "test-ns",
				"labels": {
					"astro.dev/account-id": "acct-1",
					"astro.dev/agent": "my-agent",
					"astro.dev/build": "build-1"
				}
			}
		}`)
	})
	k8sHandler.HandleFunc("/apis/batch/v1/namespaces/test-ns/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		createdJobName = "created"

		// Return a minimal valid Job response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"apiVersion":"batch/v1","kind":"Job","metadata":{"name":"test-job","namespace":"test-ns"},"spec":{}}`)
	})

	k8sClient := newMockK8sClient(k8sHandler)
	router, accountMock, indexMock := setupIngestionRouter(k8sClient, true)

	accountMock.ExpectQuery("SELECT .+ FROM accounts WHERE name").
		WithArgs("acme").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
			AddRow("acct-1", "acme", "team", time.Now(), time.Now()))
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	specJSON := `{"ingestion":{"data":{"container":{"image":"my-agent:latest"},"trigger":{"type":"manual"}}}}`
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions").
		WithArgs("acct-1", "my-agent", "build-1").
		WillReturnRows(sqlmock.NewRows([]string{"build_id", "spec_json", "readme", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", specJSON, "", "[]", time.Now(), time.Now()))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/deployments/test-ns/ingestion/data/trigger?account=acme", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["status"] != "triggered" {
		t.Errorf("expected status 'triggered', got %v", resp["status"])
	}
	if resp["namespace"] != "test-ns" {
		t.Errorf("expected namespace 'test-ns', got %v", resp["namespace"])
	}
	if resp["job_name"] == nil || resp["job_name"] == "" {
		t.Errorf("expected job_name to be set, got %v", resp["job_name"])
	}
	if createdJobName == "" {
		t.Error("expected K8s Job to be created, but no job was received")
	}

	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled account mock: %v", err)
	}
	if err := indexMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled index mock: %v", err)
	}
}
