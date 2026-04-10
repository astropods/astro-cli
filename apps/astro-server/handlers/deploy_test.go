package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
// mockQueue is a no-op DeployQueue for testing.
type mockQueue struct{}

func (q *mockQueue) InsertDeployJob(_ context.Context, _ string) error   { return nil }
func (q *mockQueue) InsertUndeployJob(_ context.Context, _ string) error { return nil }
func (q *mockQueue) InsertWakeUpJob(_ context.Context, _ string) error   { return nil }

// deploymentByIDRow returns a sqlmock.Rows matching the deploymentColumns scan in scanDeployment.
func deploymentByIDRow(id, accountID, agentName, buildID, namespace, displayName, specJSON, status string, now time.Time, undeployedAt *time.Time) *sqlmock.Rows {
	rev := 1
	return sqlmock.NewRows([]string{
		"id", "account_id", "agent_name", "build_id", "namespace",
		"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at",
	}).AddRow(
		id, accountID, agentName, buildID, namespace,
		displayName, specJSON, []byte(nil), (*string)(nil),
		status, (*string)(nil), json.RawMessage(nil), now, &rev,
		now, undeployedAt,
	)
}

// emptyDeploymentByIDRows returns an empty sqlmock.Rows matching the deploymentColumns layout.
func emptyDeploymentByIDRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "account_id", "agent_name", "build_id", "namespace",
		"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at",
	})
}

func setupUndeployTest(t *testing.T) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	index := agentindex.NewIndexWithDB(accountDB) // not used but required

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.POST("/api/v1/undeploy", UndeployAgent(log, index, accountStore, nil, deployStore, &mockQueue{}, nil, nil, nil))

	return router, deployMock, accountMock
}

func TestUndeploy_Success(t *testing.T) {
	router, deployMock, accountMock := setupUndeployTest(t)

	depID := deployid.New()
	acctID := uuid.New().String()

	now := time.Now()

	// GetDeploymentByID query — must match deploymentColumns scan order
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "active", now, nil))

	// IsMember check (SELECT COUNT(*) FROM account_members)
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// UpdateStatus (undeploying) — begins a transaction, updates status, inserts event, commits
	deployMock.ExpectBegin()
	deployMock.ExpectExec(`UPDATE`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectCommit()

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

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "undeploying" {
		t.Errorf("expected status 'undeploying', got %v", resp["status"])
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
		WillReturnRows(emptyDeploymentByIDRows())

	body := `{"deployment_id":"` + depID + `"}`
	req := httptest.NewRequest("POST", "/api/v1/undeploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// --- ListDeployments tests ---

// setupListDeploymentsTest creates a gin engine wired with the ListDeployments handler.
// The k8sHandler receives all K8s API requests so the test can control responses.
func setupListDeploymentsTest(t *testing.T, k8sHandler http.Handler) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{}

	k8sClient := newMockK8sClient(k8sHandler)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments", ListDeployments(log, accountStore, cfg, k8sClient, deployStore, nil, nil, nil, k8scache.NoopCache{}))

	return router, deployMock, accountMock
}

// k8sListHandler returns an http.Handler that serves K8s API requests for
// ListDeployments: namespace GET, deployments LIST, ingresses LIST, pods LIST, jobs LIST.
// It uses the provided namespace/agent/build to populate the response objects.
// The agentLabel parameter sets the astro.dev/agent label (account-qualified format).
func k8sListHandler(namespace, agentLabel, buildID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// GET /api/v1/namespaces/{ns}
		if r.Method == http.MethodGet && strings.HasSuffix(path, "/namespaces/"+namespace) {
			fmt.Fprintf(w, `{
				"kind":"Namespace",
				"apiVersion":"v1",
				"metadata":{"name":%q,"labels":{"astro.dev/account-id":"acct-1"}}
			}`, namespace)
			return
		}

		// LIST /apis/apps/v1/namespaces/{ns}/deployments
		if strings.Contains(path, "/deployments") {
			fmt.Fprintf(w, `{
				"kind":"DeploymentList",
				"apiVersion":"apps/v1",
				"items":[{
					"metadata":{
						"name":"agent",
						"namespace":%q,
						"creationTimestamp":"2026-03-12T21:08:24Z",
						"labels":{
							"app.kubernetes.io/managed-by":"astro-server",
							"astro.dev/agent":%q,
							"app.kubernetes.io/version":%q,
							"app.kubernetes.io/component":"agent"
						}
					},
					"spec":{"replicas":1},
					"status":{"replicas":1,"readyReplicas":1,"availableReplicas":1}
				}]
			}`, namespace, agentLabel, buildID)
			return
		}

		// LIST statefulsets
		if strings.Contains(path, "/statefulsets") {
			_, _ = w.Write([]byte(`{"kind":"StatefulSetList","apiVersion":"apps/v1","items":[]}`))
			return
		}

		// LIST ingresses
		if strings.Contains(path, "/ingresses") {
			_, _ = w.Write([]byte(`{"kind":"IngressList","apiVersion":"networking.k8s.io/v1","items":[]}`))
			return
		}

		// LIST pods
		if strings.Contains(path, "/pods") {
			fmt.Fprintf(w, `{
				"kind":"PodList",
				"apiVersion":"v1",
				"items":[{
					"metadata":{
						"name":"agent-abc123",
						"namespace":%q,
						"creationTimestamp":"2026-03-12T21:08:24Z",
						"labels":{
							"app.kubernetes.io/managed-by":"astro-server",
							"astro.dev/agent":%q,
							"app.kubernetes.io/version":%q,
							"app.kubernetes.io/component":"agent"
						}
					},
					"status":{
						"phase":"Running",
						"podIP":"10.0.0.1",
						"containerStatuses":[{
							"name":"app",
							"ready":true,
							"restartCount":0,
							"state":{"running":{"startedAt":"2026-03-12T21:08:24Z"}}
						}]
					},
					"spec":{"containers":[{"name":"app"}]}
				}]
			}`, namespace, agentLabel, buildID)
			return
		}

		// LIST jobs
		if strings.Contains(path, "/jobs") {
			_, _ = w.Write([]byte(`{"kind":"JobList","apiVersion":"batch/v1","items":[]}`))
			return
		}

		// Default: 404
		w.WriteHeader(http.StatusNotFound)
	})
}

func TestListDeployments_DBFirst_ReturnsID(t *testing.T) {
	depID := deployid.New()
	namespace := "astro-abc123def-0"
	agentName := "my-agent"
	buildID := "build-1"

	router, deployMock, accountMock := setupListDeploymentsTest(t, k8sListHandler(namespace, agentName, buildID))

	now := time.Now()

	// accountStore.GetByName
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	// IsMember
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetVisibleDeploymentsByAccount
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at",
		}).AddRow(
			depID, "acct-1", agentName, buildID, namespace, "My Agent",
			`{}`, nil, nil,
			"active", nil, nil, now, 1,
			now, nil,
		))

	req := httptest.NewRequest("GET", "/api/v1/deployments?account=myorg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Count       int               `json:"count"`
		Deployments []AgentDeployment `json:"deployments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("expected count=1, got %d", resp.Count)
	}
	dep := resp.Deployments[0]
	if dep.ID != depID {
		t.Errorf("expected ID %q, got %q", depID, dep.ID)
	}
	if dep.DisplayName != "My Agent" {
		t.Errorf("expected display_name 'My Agent', got %q", dep.DisplayName)
	}
	if dep.Name != agentName {
		t.Errorf("expected name %q, got %q", agentName, dep.Name)
	}
}

func TestListDeployments_AgentLabelNotLeaked(t *testing.T) {
	// The astro.dev/agent label is account-qualified ("myaccount.sasbot") but
	// the Name returned to the frontend must be the plain agent name from the DB.
	depID := deployid.New()
	namespace := "astro-abc123def-0"
	agentName := "sasbot"
	agentLabel := "myaccount.sasbot" // account-qualified label on k8s resources
	buildID := "build-1"

	router, deployMock, accountMock := setupListDeploymentsTest(t,
		k8sListHandler(namespace, agentLabel, buildID))

	now := time.Now()

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name",
		}).AddRow("acct-1", "myaccount", "organization", nil, nil, now, now, ""))

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at",
		}).AddRow(
			depID, "acct-1", agentName, buildID, namespace, "Sas Bot",
			`{}`, nil, nil,
			"active", nil, nil, now, 1,
			now, nil,
		))

	req := httptest.NewRequest("GET", "/api/v1/deployments?account=myaccount", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Count       int               `json:"count"`
		Deployments []AgentDeployment `json:"deployments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("expected count=1, got %d", resp.Count)
	}
	dep := resp.Deployments[0]
	if dep.Name != agentName {
		t.Errorf("expected plain agent name %q, got %q (account-qualified label leaked)", agentName, dep.Name)
	}
	if dep.Name == agentLabel {
		t.Errorf("agent label value %q leaked to frontend as Name", agentLabel)
	}
}

func TestListAstroDeploymentsLight_StatusAndReplicas(t *testing.T) {
	namespace := "astro-abc123def-0"
	agentName := "my-agent"
	buildID := "build-1"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		if strings.Contains(path, "/deployments") {
			fmt.Fprintf(w, `{
				"kind":"DeploymentList","apiVersion":"apps/v1","items":[{
					"metadata":{
						"name":"agent","namespace":%q,
						"creationTimestamp":"2026-03-12T21:08:24Z",
						"labels":{
							"app.kubernetes.io/managed-by":"astro-server",
							"astro.dev/agent":%q,
							"app.kubernetes.io/version":%q,
							"app.kubernetes.io/component":"agent"
						}
					},
					"spec":{"replicas":2},
					"status":{"replicas":2,"readyReplicas":1}
				}]
			}`, namespace, agentName, buildID)
			return
		}
		if strings.Contains(path, "/statefulsets") {
			_, _ = w.Write([]byte(`{"kind":"StatefulSetList","apiVersion":"apps/v1","items":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	k8sClient := newMockK8sClient(handler)
	deps, err := listAstroDeploymentsLight(context.Background(), k8sClient, namespace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	d := deps[0]
	if d.Replicas != 2 {
		t.Errorf("expected replicas=2, got %d", d.Replicas)
	}
	if d.Ready != 1 {
		t.Errorf("expected ready=1, got %d", d.Ready)
	}
	if d.Status != "Pending" {
		t.Errorf("expected status=Pending (ready < replicas), got %q", d.Status)
	}
}

func TestListAstroDeploymentsLight_SkipsPodsIngressesJobs(t *testing.T) {
	namespace := "astro-abc123def-0"
	var calledPaths []string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calledPaths = append(calledPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/deployments") {
			_, _ = w.Write([]byte(`{"kind":"DeploymentList","apiVersion":"apps/v1","items":[]}`))
			return
		}
		if strings.Contains(r.URL.Path, "/statefulsets") {
			_, _ = w.Write([]byte(`{"kind":"StatefulSetList","apiVersion":"apps/v1","items":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	k8sClient := newMockK8sClient(handler)
	_, _ = listAstroDeploymentsLight(context.Background(), k8sClient, namespace, nil)

	for _, p := range calledPaths {
		if strings.Contains(p, "/pods") || strings.Contains(p, "/ingresses") || strings.Contains(p, "/jobs") {
			t.Errorf("light variant made unexpected K8s call: %s", p)
		}
	}
}

func TestListDeployments_NoDBRecord_ReturnsEmpty(t *testing.T) {
	// K8s namespace exists but no DB record → deployment should NOT appear
	router, deployMock, accountMock := setupListDeploymentsTest(t,
		k8sListHandler("astro-orphan-0", "orphan-agent", "build-1"))

	now := time.Now()

	// accountStore.GetByName
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	// IsMember
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetVisibleDeploymentsByAccount returns no rows
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at",
		}))

	req := httptest.NewRequest("GET", "/api/v1/deployments?account=myorg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Count       int               `json:"count"`
		Deployments []AgentDeployment `json:"deployments"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Count != 0 {
		t.Errorf("expected count=0 when no DB records, got %d", resp.Count)
	}
}

func TestListDeployments_MissingAccountParam(t *testing.T) {
	router, _, _ := setupListDeploymentsTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest("GET", "/api/v1/deployments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListDeployments_NotMember(t *testing.T) {
	router, _, accountMock := setupListDeploymentsTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	now := time.Now()

	// accountStore.GetByName
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	// IsMember returns 0
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	req := httptest.NewRequest("GET", "/api/v1/deployments?account=myorg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListDeployments_MultipleDeployments(t *testing.T) {
	depID1 := deployid.New()
	depID2 := deployid.New()
	ns1 := "astro-aaa111bbb-0"
	ns2 := "astro-ccc222ddd-0"

	// K8s handler that responds to both namespaces
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// Namespace GET for ns1
		if r.Method == http.MethodGet && strings.HasSuffix(path, "/namespaces/"+ns1) {
			fmt.Fprintf(w, `{"kind":"Namespace","apiVersion":"v1","metadata":{"name":%q}}`, ns1)
			return
		}
		// Namespace GET for ns2
		if r.Method == http.MethodGet && strings.HasSuffix(path, "/namespaces/"+ns2) {
			fmt.Fprintf(w, `{"kind":"Namespace","apiVersion":"v1","metadata":{"name":%q}}`, ns2)
			return
		}

		// Deployments list — respond based on namespace in path
		if strings.Contains(path, "/deployments") {
			agent := "agent-a"
			ns := ns1
			if strings.Contains(path, ns2) {
				agent = "agent-b"
				ns = ns2
			}
			fmt.Fprintf(w, `{
				"kind":"DeploymentList","apiVersion":"apps/v1",
				"items":[{
					"metadata":{
						"name":"%s-agent","namespace":%q,
						"creationTimestamp":"2026-03-12T21:08:24Z",
						"labels":{"app.kubernetes.io/managed-by":"astro-server","astro.dev/agent":%q,"app.kubernetes.io/version":"b1","app.kubernetes.io/component":"agent"}
					},
					"spec":{"replicas":1},
					"status":{"replicas":1,"readyReplicas":1,"availableReplicas":1}
				}]
			}`, agent, ns, agent)
			return
		}

		if strings.Contains(path, "/statefulsets") {
			_, _ = w.Write([]byte(`{"kind":"StatefulSetList","apiVersion":"apps/v1","items":[]}`))
			return
		}

		if strings.Contains(path, "/ingresses") {
			_, _ = w.Write([]byte(`{"kind":"IngressList","apiVersion":"networking.k8s.io/v1","items":[]}`))
			return
		}

		if strings.Contains(path, "/pods") {
			_, _ = w.Write([]byte(`{"kind":"PodList","apiVersion":"v1","items":[]}`))
			return
		}

		if strings.Contains(path, "/jobs") {
			_, _ = w.Write([]byte(`{"kind":"JobList","apiVersion":"batch/v1","items":[]}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	router, deployMock, accountMock := setupListDeploymentsTest(t, handler)

	now := time.Now()

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Two active deployments
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at",
		}).AddRow(
			depID1, "acct-1", "agent-a", "b1", ns1, "Agent A",
			`{}`, nil, nil,
			"active", nil, nil, now, 1,
			now, nil,
		).AddRow(
			depID2, "acct-1", "agent-b", "b1", ns2, "Agent B",
			`{}`, nil, nil,
			"active", nil, nil, now, 1,
			now, nil,
		))

	req := httptest.NewRequest("GET", "/api/v1/deployments?account=myorg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Count       int               `json:"count"`
		Deployments []AgentDeployment `json:"deployments"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Count != 2 {
		t.Fatalf("expected 2 deployments, got %d", resp.Count)
	}

	ids := map[string]bool{}
	for _, d := range resp.Deployments {
		ids[d.ID] = true
	}
	if !ids[depID1] {
		t.Errorf("missing deployment ID %q", depID1)
	}
	if !ids[depID2] {
		t.Errorf("missing deployment ID %q", depID2)
	}
}

func TestListDeployments_AgentReadinessOverridesNonPrimaryComponents(t *testing.T) {
	depID := deployid.New()
	namespace := "astro-agentstatus-0"
	agentName := "my-agent"
	buildID := "build-1"

	// Collector appears first and is pending, agent appears second and is ready.
	// The top-level deployment status should reflect the agent component.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if r.Method == http.MethodGet && strings.HasSuffix(path, "/namespaces/"+namespace) {
			fmt.Fprintf(w, `{"kind":"Namespace","apiVersion":"v1","metadata":{"name":%q}}`, namespace)
			return
		}

		if strings.Contains(path, "/deployments") {
			fmt.Fprintf(w, `{
				"kind":"DeploymentList","apiVersion":"apps/v1",
				"items":[
					{
						"metadata":{
							"name":"my-agent-collector",
							"namespace":%q,
							"creationTimestamp":"2026-03-12T21:08:24Z",
							"labels":{
								"app.kubernetes.io/managed-by":"astro-server",
								"astro.dev/agent":%q,
								"app.kubernetes.io/version":%q,
								"app.kubernetes.io/component":"collector"
							}
						},
						"spec":{"replicas":1},
						"status":{"replicas":1,"readyReplicas":0,"availableReplicas":0}
					},
					{
						"metadata":{
							"name":"my-agent-agent",
							"namespace":%q,
							"creationTimestamp":"2026-03-12T21:10:24Z",
							"labels":{
								"app.kubernetes.io/managed-by":"astro-server",
								"astro.dev/agent":%q,
								"app.kubernetes.io/version":%q,
								"app.kubernetes.io/component":"agent"
							}
						},
						"spec":{"replicas":1},
						"status":{"replicas":1,"readyReplicas":1,"availableReplicas":1}
					}
				]
			}`, namespace, agentName, buildID, namespace, agentName, buildID)
			return
		}

		if strings.Contains(path, "/statefulsets") {
			_, _ = w.Write([]byte(`{"kind":"StatefulSetList","apiVersion":"apps/v1","items":[]}`))
			return
		}

		if strings.Contains(path, "/ingresses") {
			_, _ = w.Write([]byte(`{"kind":"IngressList","apiVersion":"networking.k8s.io/v1","items":[]}`))
			return
		}

		if strings.Contains(path, "/pods") {
			_, _ = w.Write([]byte(`{"kind":"PodList","apiVersion":"v1","items":[]}`))
			return
		}

		if strings.Contains(path, "/jobs") {
			_, _ = w.Write([]byte(`{"kind":"JobList","apiVersion":"batch/v1","items":[]}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	router, deployMock, accountMock := setupListDeploymentsTest(t, handler)
	now := time.Now()

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at",
		}).AddRow(
			depID, "acct-1", agentName, buildID, namespace, "My Agent",
			`{}`, nil, nil,
			"active", nil, nil, now, 1,
			now, nil,
		))

	req := httptest.NewRequest("GET", "/api/v1/deployments?account=myorg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Count       int               `json:"count"`
		Deployments []AgentDeployment `json:"deployments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("expected count=1, got %d", resp.Count)
	}
	if got := resp.Deployments[0].Status; got != "Running" {
		t.Fatalf("expected status Running from agent readiness, got %q", got)
	}
	if got := resp.Deployments[0].Ready; got != 1 {
		t.Fatalf("expected ready replicas 1 from agent deployment, got %d", got)
	}
}

func TestListDeployments_NilDeployStore(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	accountStore := account.NewAccountStore(accountDB)
	log := logger.New("error", "json")
	cfg := &config.Config{}
	k8sClient := newMockK8sClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments", ListDeployments(log, accountStore, cfg, k8sClient, nil, nil, nil, nil, k8scache.NoopCache{}))

	now := time.Now()
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest("GET", "/api/v1/deployments?account=myorg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when deploy store is nil, got %d: %s", w.Code, w.Body.String())
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
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "undeployed", now, &later))

	body := `{"deployment_id":"` + depID + `"}`
	req := httptest.NewRequest("POST", "/api/v1/undeploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for inactive deployment, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUndeploy_FailedDeployment(t *testing.T) {
	router, deployMock, accountMock := setupUndeployTest(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	// GetDeploymentByID returns a failed deployment (e.g. recovered orphan)
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "orphan-agent", "build-1", "astro-orphan-0",
			"Orphan Agent", `{}`, "failed", now, nil))

	// IsMember check
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// UpdateStatus (undeploying)
	deployMock.ExpectBegin()
	deployMock.ExpectExec(`UPDATE`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectCommit()

	body := `{"deployment_id":"` + depID + `"}`
	req := httptest.NewRequest("POST", "/api/v1/undeploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for failed deployment undeploy, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "undeploying" {
		t.Errorf("expected status 'undeploying', got %v", resp["status"])
	}
}

func TestUndeploy_Forbidden(t *testing.T) {
	router, deployMock, accountMock := setupUndeployTest(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	// GetDeploymentByID returns active deployment
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "active", now, nil))

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
	router.POST("/api/v1/undeploy", UndeployAgent(log, index, accountStore, nil, deployStore, &mockQueue{}, nil, nil, nil))

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

// --- prepareDeployment source-agent visibility tests ---
// These use the ValidateDeployment handler since it calls prepareDeployment
// without requiring a K8s client.

// minimalDeploySpec returns a JSON deployment spec body for testing.
// The source account/name/build are set; the spec is minimal but parseable.
func minimalDeploySpec(sourceAccount, agentName, buildID string) string {
	return fmt.Sprintf(`{
		"spec": "deployment/v1",
		"source": {"account": %q, "name": %q, "build": %q, "registry": "r.io"},
		"target": {"runtime": "kubernetes"},
		"agent": {"image": "r.io/img:latest", "endpoints": {"http": {"port": 8080}}}
	}`, sourceAccount, agentName, buildID)
}

// setupValidateRouter creates a gin engine wired with ValidateDeployment.
func setupValidateRouter(userID string) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)

	indexDB, indexMock, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()

	index := agentindex.NewIndexWithDB(indexDB)
	store := account.NewAccountStore(accountDB)
	log := logger.New("error", "json")
	cfg := &config.Config{
		Deployment: config.DeploymentConfig{
			RegistryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			Environment: "test",
		},
	}

	router := gin.New()
	if userID != "" {
		router.Use(func(c *gin.Context) {
			c.Set(string(auth.UserContextKey), &auth.User{ID: userID})
			c.Next()
		})
	}
	router.POST("/deploy/validate", ValidateDeployment(log, index, store, cfg))

	return router, indexMock, accountMock
}

func TestDeploy_PrivateSourceAgent_NonMember_Rejected(t *testing.T) {
	router, indexMock, accountMock := setupValidateRouter("user-cross")

	now := time.Now()
	body := minimalDeploySpec("source-org", "secret-agent", "build-1")

	// Source account lookup
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("source-org").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("src-acct", "source-org", "organization", nil, nil, now, now, ""))

	// Target == source (no target.account in spec), so no second account lookup

	// IsMember(target=source, user) → member of target
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("src-acct", "user-cross").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get → private agent
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("src-acct", "secret-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}).
			AddRow("src-acct", "secret-agent", "r.io", "private", nil, now, now))
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("src-acct", "secret-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "testaccount", `{"name":"secret-agent"}`, "", "", "[]", now, now))

	// IsMember(source, user) → NOT a member
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("src-acct", "user-cross").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	req := httptest.NewRequest(http.MethodPost, "/deploy/validate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("private source agent should be hidden from non-members, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "source agent not found" {
		t.Errorf("expected 'source agent not found', got %v", resp["error"])
	}
}

func TestDeploy_PrivateSourceAgent_CrossAccount_Rejected(t *testing.T) {
	router, indexMock, accountMock := setupValidateRouter("user-target")

	now := time.Now()
	// Source and target are different accounts
	body := `{
		"spec": "deployment/v1",
		"source": {"account": "source-org", "name": "secret-agent", "build": "build-1", "registry": "r.io"},
		"target": {"account": "target-org", "runtime": "kubernetes"},
		"agent": {"image": "r.io/img:latest", "endpoints": {"http": {"port": 8080}}}
	}`

	// Source account lookup
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("source-org").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("src-acct", "source-org", "organization", nil, nil, now, now, ""))

	// Target account lookup (different from source)
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("target-org").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("tgt-acct", "target-org", "organization", nil, nil, now, now, ""))

	// IsMember(target, user) → member of target account
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("tgt-acct", "user-target").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get → private agent
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("src-acct", "secret-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}).
			AddRow("src-acct", "secret-agent", "r.io", "private", nil, now, now))
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("src-acct", "secret-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "testaccount", `{"name":"secret-agent"}`, "", "", "[]", now, now))

	// IsMember(source, user) → NOT a member of source
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("src-acct", "user-target").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	req := httptest.NewRequest(http.MethodPost, "/deploy/validate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-account deploy of private agent should be rejected, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeploy_SourceAgentNotFound_Rejected(t *testing.T) {
	router, indexMock, accountMock := setupValidateRouter("user-1")

	now := time.Now()
	body := minimalDeploySpec("myorg", "nonexistent", "build-1")

	// Account lookup
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	// IsMember(target=source, user) → member
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get → no rows
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "nonexistent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}))

	req := httptest.NewRequest(http.MethodPost, "/deploy/validate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeploy_OrgScopedSourceName_Rejected(t *testing.T) {
	tests := []struct {
		name      string
		agentName string
	}{
		{"scoped name", "@postman/feb19-astro"},
		{"slash in name", "org/my-agent"},
		{"bare @ prefix", "@my-agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _, accountMock := setupValidateRouter("user-1")

			now := time.Now()
			body := minimalDeploySpec("myorg", tt.agentName, "build-1")

			// Account lookup (validation happens before agent lookup)
			accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
				WithArgs("myorg").
				WillReturnRows(sqlmock.NewRows(
					[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
					AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

			// IsMember
			accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
				WithArgs("acct-1", "user-1").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

			req := httptest.NewRequest(http.MethodPost, "/deploy/validate", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			errMsg, _ := resp["error"].(string)
			if !strings.Contains(errMsg, "@org/ prefix") {
				t.Errorf("expected org prefix error, got %q", errMsg)
			}
		})
	}
}

// --- GetDeploymentTemplate visibility tests ---

// setupTemplateRouter creates a gin engine wired with GetDeploymentTemplate.
// If userID is non-empty, an auth middleware injects that user.
func setupTemplateRouter(userID string) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)

	indexDB, indexMock, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()

	index := agentindex.NewIndexWithDB(indexDB)
	store := account.NewAccountStore(accountDB)
	log := logger.New("error", "json")
	cfg := &config.Config{
		Deployment: config.DeploymentConfig{
			RegistryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			Environment: "test",
		},
	}

	router := gin.New()
	if userID != "" {
		router.Use(func(c *gin.Context) {
			c.Set(string(auth.UserContextKey), &auth.User{ID: userID})
			c.Next()
		})
	}
	router.GET("/agents/:account/:name/deployment-template",
		GetDeploymentTemplate(log, index, store, cfg))

	return router, indexMock, accountMock
}

// expectAgentLookup sets up sqlmock expectations for agentIndex.Get():
// one query on agents table, one on agent_versions table.
func expectAgentLookup(mock sqlmock.Sqlmock, visibility string) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", visibility, nil, now, now))
	mock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))
}

// expectLatestVersion sets up the sqlmock expectation for agentIndex.GetLatestVersion().
func expectLatestVersion(mock sqlmock.Sqlmock) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))
}

// expectAccountLookup sets up sqlmock expectation for accountStore.GetByName().
func expectAccountLookup(mock sqlmock.Sqlmock) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))
}

func TestGetDeploymentTemplate_PublicAgent_CrossAccount(t *testing.T) {
	router, indexMock, accountMock := setupTemplateRouter("user-outside")

	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "public")
	expectLatestVersion(indexMock)

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent/deployment-template?format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("public agent should be accessible cross-account, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["spec"] != "deployment-template/v1" {
		t.Errorf("expected spec 'deployment-template/v1', got %v", resp["spec"])
	}
}

func TestGetDeploymentTemplate_PublicAgent_Member(t *testing.T) {
	router, indexMock, accountMock := setupTemplateRouter("user-1")

	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "public")
	expectLatestVersion(indexMock)

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent/deployment-template?format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("public agent should be accessible to members, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDeploymentTemplate_PrivateAgent_Member(t *testing.T) {
	router, indexMock, accountMock := setupTemplateRouter("user-1")

	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "private")

	// IsMember check — is a member
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	expectLatestVersion(indexMock)

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent/deployment-template?format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("private agent should be accessible to members, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDeploymentTemplate_PrivateAgent_NonMember(t *testing.T) {
	router, indexMock, accountMock := setupTemplateRouter("user-outside")

	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "private")

	// IsMember check — not a member
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-outside").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent/deployment-template?format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("private agent should return 404 to non-members, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDeploymentTemplate_NoAuth(t *testing.T) {
	router, _, _ := setupTemplateRouter("") // no user

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent/deployment-template?format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDeploymentTemplate_AgentNotFound(t *testing.T) {
	router, indexMock, accountMock := setupTemplateRouter("user-1")

	expectAccountLookup(accountMock)

	// Agent lookup returns no rows
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}))

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent/deployment-template?format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDeploymentTemplate_DefaultYAML(t *testing.T) {
	router, indexMock, accountMock := setupTemplateRouter("user-1")

	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "public")
	expectLatestVersion(indexMock)

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent/deployment-template", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/yaml" {
		t.Errorf("expected Content-Type 'application/yaml', got %q", ct)
	}
}

// --- Template: deployment_id presence tests ---

func TestGetDeploymentTemplate_NoDeploymentID(t *testing.T) {
	router, indexMock, accountMock := setupTemplateRouter("user-1")

	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "public")
	expectLatestVersion(indexMock)

	req := httptest.NewRequest(http.MethodGet, "/agents/myorg/my-agent/deployment-template?format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	target, ok := resp["target"].(map[string]any)
	if !ok {
		t.Fatal("expected target to be an object")
	}
	if _, exists := target["deployment_id"]; exists {
		t.Errorf("plain template should not contain deployment_id, got %v", target["deployment_id"])
	}
}

func TestGetPrefilledTemplate_HasDeploymentID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	indexDB, indexMock, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	index := agentindex.NewIndexWithDB(indexDB)
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{
		Deployment: config.DeploymentConfig{
			RegistryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			Environment: "test",
		},
	}

	depID := "dep-123"
	acctID := "acct-1"
	now := time.Now()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/agents/:account/:name/deployment-template/:deploymentID",
		GetPrefilledDeploymentTemplate(log, index, accountStore, cfg, deployStore))

	// GetDeploymentByID — must come before generateTemplate now that we pin the build
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Deploy", `{"interfaces":{"adapters":["slack"]}}`, "active", now, nil))

	// IsMember check for deployment's account
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// generateTemplate expectations: account lookup, agent lookup, then GetVersion(existing.BuildID)
	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "public")
	// GetVersion("build-1") — pinned to current deployment's build, not latest
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent", "build-1").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))

	// GetDeploymentVariables
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}))

	// GetByID for account name resolution
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow(acctID, "myorg", "organization", nil, nil, now, now, ""))

	req := httptest.NewRequest(http.MethodGet,
		"/agents/myorg/my-agent/deployment-template/"+depID+"?format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	target, ok := resp["target"].(map[string]any)
	if !ok {
		t.Fatal("expected target to be an object")
	}
	if target["deployment_id"] != depID {
		t.Errorf("expected deployment_id=%q, got %v", depID, target["deployment_id"])
	}
	if target["display_name"] != "My Deploy" {
		t.Errorf("expected display_name='My Deploy', got %v", target["display_name"])
	}
}

// TestGetPrefilledTemplate_BuildParamOverride verifies that passing ?build= to the prefilled
// template endpoint overrides the build ID used to generate the template. This supports the
// "new build available" upgrade flow where the client needs a template for a newer build.
func TestGetPrefilledTemplate_BuildParamOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)

	indexDB, indexMock, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	index := agentindex.NewIndexWithDB(indexDB)
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{
		Deployment: config.DeploymentConfig{
			RegistryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			Environment: "test",
		},
	}

	depID := "dep-123"
	acctID := "acct-1"
	now := time.Now()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/agents/:account/:name/deployment-template/:deploymentID",
		GetPrefilledDeploymentTemplate(log, index, accountStore, cfg, deployStore))

	// GetDeploymentByID — existing deployment is on build-1; a newer build-2 exists
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Deploy", `{}`, "active", now, nil))

	// IsMember check
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// generateTemplate uses ?build=build-2 override, not the existing deployment's build-1
	expectAccountLookup(accountMock)
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", "public", nil, now, now))
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-2", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))
	// GetVersion called with "build-2" from the query param override, not "build-1"
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent", "build-2").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-2", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))

	// GetDeploymentVariables
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}))

	// GetByID for account name resolution
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow(acctID, "myorg", "organization", nil, nil, now, now, ""))

	// Pass ?build=build-2 — should override the existing deployment's build-1
	req := httptest.NewRequest(http.MethodGet,
		"/agents/myorg/my-agent/deployment-template/"+depID+"?build=build-2&format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	// Template must use build-2 from the query param, not the existing deployment's build-1
	source, ok := resp["source"].(map[string]any)
	if !ok {
		t.Fatal("expected source to be an object")
	}
	if source["build"] != "build-2" {
		t.Errorf("expected source.build='build-2' (overridden by ?build=), got %v", source["build"])
	}

	target := resp["target"].(map[string]any)
	if target["deployment_id"] != depID {
		t.Errorf("expected deployment_id=%q, got %v", depID, target["deployment_id"])
	}
}

// TestGetPrefilledTemplate_RevisionUsesBuildID verifies that when ?revision=N is
// provided, the template is generated from the revision's build_id, not the
// current deployment's build_id.
func TestGetPrefilledTemplate_RevisionUsesBuildID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	indexDB, indexMock, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	index := agentindex.NewIndexWithDB(indexDB)
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{Deployment: config.DeploymentConfig{RegistryURL: "docker.io/library"}}

	depID := "dep-123"
	acctID := "acct-1"
	now := time.Now()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/agents/:account/:name/deployment-template/:deploymentID",
		GetPrefilledDeploymentTemplate(log, index, accountStore, cfg, deployStore))

	// GetDeploymentByID — current deployment is on build-current
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-current", "astro-abc123",
			"Current Name", `{}`, "active", now, nil))

	// IsMember check
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetRevisionByNumber — revision 1 was on build-old
	deployMock.ExpectQuery(`SELECT`).
		WithArgs(depID, 1).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "deployment_id", "revision", "build_id", "spec_json", "kms_ciphertext", "kms_key_id", "created_at"}).
			AddRow(1, depID, 1, "build-old", json.RawMessage(`{"target":{"display_name":"Old Name"}}`), []byte(nil), (*string)(nil), now))

	// generateTemplate must use "build-old" (revision's build), not "build-current"
	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "public")
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent", "build-old").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-old", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))

	// GetDeploymentVariables
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce"}))

	// GetByID for account name resolution
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow(acctID, "myorg", "organization", nil, nil, now, now, ""))

	req := httptest.NewRequest(http.MethodGet,
		"/agents/myorg/my-agent/deployment-template/"+depID+"?revision=1&format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	source, ok := resp["source"].(map[string]any)
	if !ok {
		t.Fatal("expected source to be an object")
	}
	if source["build"] != "build-old" {
		t.Errorf("expected source.build='build-old' (revision's build), got %v", source["build"])
	}
}

// TestGetPrefilledTemplate_RevisionRestoresDisplayName verifies that when ?revision=N
// is provided, the display_name from the revision's spec is used instead of the
// current deployment's display_name.
func TestGetPrefilledTemplate_RevisionRestoresDisplayName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	indexDB, indexMock, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	index := agentindex.NewIndexWithDB(indexDB)
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{Deployment: config.DeploymentConfig{RegistryURL: "docker.io/library"}}

	depID := "dep-123"
	acctID := "acct-1"
	now := time.Now()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/agents/:account/:name/deployment-template/:deploymentID",
		GetPrefilledDeploymentTemplate(log, index, accountStore, cfg, deployStore))

	// Current deployment has a new display name
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-old", "astro-abc123",
			"New Name", `{}`, "active", now, nil))

	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Revision 1 spec has the old display name
	deployMock.ExpectQuery(`SELECT`).
		WithArgs(depID, 1).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "deployment_id", "revision", "build_id", "spec_json", "kms_ciphertext", "kms_key_id", "created_at"}).
			AddRow(1, depID, 1, "build-old", json.RawMessage(`{"target":{"display_name":"Old Name"}}`), []byte(nil), (*string)(nil), now))

	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "public")
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent", "build-old").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-old", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce"}))

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow(acctID, "myorg", "organization", nil, nil, now, now, ""))

	req := httptest.NewRequest(http.MethodGet,
		"/agents/myorg/my-agent/deployment-template/"+depID+"?revision=1&format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	target, ok := resp["target"].(map[string]any)
	if !ok {
		t.Fatal("expected target to be an object")
	}
	if target["display_name"] != "Old Name" {
		t.Errorf("expected display_name='Old Name' (from revision spec), got %v", target["display_name"])
	}
}

// TestGetPrefilledTemplate_RevisionNotFound verifies that requesting a non-existent
// revision returns 404.
func TestGetPrefilledTemplate_RevisionNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	indexDB, _, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	index := agentindex.NewIndexWithDB(indexDB)
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{Deployment: config.DeploymentConfig{RegistryURL: "docker.io/library"}}

	depID := "dep-123"
	acctID := "acct-1"
	now := time.Now()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/agents/:account/:name/deployment-template/:deploymentID",
		GetPrefilledDeploymentTemplate(log, index, accountStore, cfg, deployStore))

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Deploy", `{}`, "active", now, nil))

	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetRevisionByNumber returns no rows → nil revision
	deployMock.ExpectQuery(`SELECT`).
		WithArgs(depID, 99).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "deployment_id", "revision", "build_id", "spec_json", "kms_ciphertext", "kms_key_id", "created_at"}))

	req := httptest.NewRequest(http.MethodGet,
		"/agents/myorg/my-agent/deployment-template/"+depID+"?revision=99&format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing revision, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestGetPrefilledTemplate_PreservesAuth verifies that when a deployment was
// configured with OIDC auth, the prefilled template restores interfaces.auth
// so the require-authentication toggle is not reset to false in the redeploy UI.
func TestGetPrefilledTemplate_PreservesAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	indexDB, indexMock, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	index := agentindex.NewIndexWithDB(indexDB)
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{
		Deployment: config.DeploymentConfig{
			RegistryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			Environment: "test",
		},
	}

	depID := "dep-123"
	acctID := "acct-1"
	now := time.Now()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/agents/:account/:name/deployment-template/:deploymentID",
		GetPrefilledDeploymentTemplate(log, index, accountStore, cfg, deployStore))

	// Stored spec has OIDC auth enabled on the web interface
	specWithAuth := `{"interfaces":{"adapters":["web"],"auth":{"web":{"type":"oidc"}}}}`
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Deploy", specWithAuth, "active", now, nil))

	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "public")
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent", "build-1").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}))

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow(acctID, "myorg", "organization", nil, nil, now, now, ""))

	req := httptest.NewRequest(http.MethodGet,
		"/agents/myorg/my-agent/deployment-template/"+depID+"?format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	interfaces, ok := resp["interfaces"].(map[string]any)
	if !ok {
		t.Fatal("expected interfaces to be an object")
	}
	auth, ok := interfaces["auth"].(map[string]any)
	if !ok {
		t.Fatal("expected interfaces.auth to be present — toggle would reset to false without the fix")
	}
	web, ok := auth["web"].(map[string]any)
	if !ok {
		t.Fatal("expected interfaces.auth.web to be an object")
	}
	if web["type"] != "oidc" {
		t.Errorf("expected interfaces.auth.web.type='oidc', got %v", web["type"])
	}
}

// --- Deploy endpoint: deployment_id handling tests ---

// setupDeployRouter creates a gin engine wired with DeployAgent and ValidateDeployment.
// Returns (router, indexMock, accountMock, deployMock).
func setupDeployRouter(userID string) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)

	indexDB, indexMock, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	index := agentindex.NewIndexWithDB(indexDB)
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{
		Deployment: config.DeploymentConfig{
			RegistryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			Environment: "test",
		},
	}

	router := gin.New()
	if userID != "" {
		router.Use(func(c *gin.Context) {
			c.Set(string(auth.UserContextKey), &auth.User{ID: userID})
			c.Next()
		})
	}
	router.POST("/deploy", DeployAgent(log, index, accountStore, cfg, deployStore, nil, nil, &mockQueue{}, nil, nil, nil, nil)) //nolint:staticcheck // nil varsStore, EntitlementChecker, avatarStore, omClient, and auditStore skip checks/emit in tests

	return router, indexMock, accountMock, deployMock
}

// expectDeployPrep sets up mocks for the full prepareDeployment flow: account lookup,
// membership check, agent+version lookup for both agentIndex.Get and the build lookup.
func expectDeployPrep(accountMock, indexMock sqlmock.Sqlmock) {
	now := time.Now()

	// accountStore.GetByName("myorg") — source account
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	// IsMember(target=source, user) → yes
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get (visibility check)
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "r.io", "public", nil, now, now))
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))

	// agentIndex.GetVersion (exact build lookup)
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent", "build-1").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))
}

func expectVariableInsertsByName(deployMock sqlmock.Sqlmock, names ...string) {
	deployMock.MatchExpectationsInOrder(false)
	for _, name := range names {
		deployMock.ExpectExec(`INSERT INTO deployment_variables`).
			WithArgs(
				sqlmock.AnyArg(), // deployment_id
				name,             // name
				sqlmock.AnyArg(), // value
				sqlmock.AnyArg(), // ref
				sqlmock.AnyArg(), // secret
				sqlmock.AnyArg(), // optional
				sqlmock.AnyArg(), // targets
				sqlmock.AnyArg(), // nonce
			).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
}

// deployableSpec builds a JSON deployment spec that matches the template the server
// generates from the agent spec `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}` with RegistryURL "123456789.dkr.ecr.us-east-1.amazonaws.com"
// and Environment "test". The caller can optionally set deploymentID to test the in-place update path.
func deployableSpec(deploymentID string) string {
	targetExtra := ""
	if deploymentID != "" {
		targetExtra = fmt.Sprintf(`, "deployment_id": %q`, deploymentID)
	}
	return fmt.Sprintf(`{
		"spec": "deployment/v1",
		"source": {"account": "myorg", "name": "my-agent", "build": "build-1", "registry": "https://123456789.dkr.ecr.us-east-1.amazonaws.com"},
		"target": {"runtime": "kubernetes"%s},
		"agent": {
			"image": "123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1",
			"endpoints": {"http": {"port": 8080, "protocol": "http"}},
			"replicas": 1,
			"resources": {"cpu": "100m", "memory": "256Mi", "cpu_limit": "1", "memory_limit": "1Gi"},
			"environment": {"ASTRO_AGENT_NAME": "my-agent", "ASTRO_AGENT_BUILD": "build-1"},
			"update": {"strategy": "rolling", "max_unavailable": "25%%", "max_surge": "25%%"}
		},
		"variables": {
			"SLACK_BOT_TOKEN": {"secret": true, "optional": true, "targets": ["interface.slack"]},
			"SLACK_APP_TOKEN": {"secret": true, "optional": true, "targets": ["interface.slack"]},
			"SLACK_CONFIG": {"secret": false, "optional": true, "targets": ["interface.slack"]}
		},
		"observability": {"enabled": true, "provider": "langfuse"}
	}`, targetExtra)
}

func TestDeploy_WithoutDeploymentID_CreatesNew(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupDeployRouter("user-1")

	expectDeployPrep(accountMock, indexMock)

	// No deployment_id in spec → new deployment path.
	// No display name lookup needed (empty display_name).
	// No existing deployment lookup (GetActiveDeployment returns no rows).
	deployMock.ExpectQuery(`SELECT`). // GetActiveDeployment
						WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn",
			"status", "deployed_at", "undeployed_at",
		}))

	// SaveDeploymentPending transaction
	deployMock.ExpectBegin()
	deployMock.ExpectQuery(`UPDATE deployments`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	deployMock.ExpectQuery(`INSERT INTO deployments`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at",
		}).AddRow("new-id", "acct-1", "my-agent", "build-1", "astro-new", "", "{}", "pending", time.Now()))
	// Revision insert
	deployMock.ExpectExec(`INSERT INTO deployment_revisions`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Event insert
	deployMock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Normalized spec inserts (agent workload + service + collector workload + services + variables)
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	expectVariableInsertsByName(
		deployMock,
		"SLACK_BOT_TOKEN",
		"SLACK_APP_TOKEN",
		"SLACK_CONFIG",
	)
	deployMock.ExpectExec(`INSERT INTO deployment_resolved_keys`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectCommit()

	body := deployableSpec("")
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "pending" {
		t.Errorf("expected status 'pending', got %v", resp["status"])
	}
}

func TestDeploy_WithDeploymentID_UpdatesExisting(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupDeployRouter("user-1")

	expectDeployPrep(accountMock, indexMock)

	depID := "existing-dep-id"
	now := time.Now()

	// GetDeploymentByID for the provided deployment_id
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, "acct-1", "my-agent", "build-1", "astro-existing",
			"My Agent", `{}`, "active", now, nil))

	// IsMember check for deployment's account
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// UpdateDeploymentPending transaction
	deployMock.ExpectBegin()
	// Next revision query
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"next_revision"}).AddRow(2))
	deployMock.ExpectQuery(`UPDATE deployments`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at",
		}).AddRow(depID, "acct-1", "my-agent", "build-1", "astro-existing", "My Agent", "{}", "pending", now))
	// Revision insert
	deployMock.ExpectExec(`INSERT INTO deployment_revisions`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`DELETE FROM deployment_workloads`).WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`DELETE FROM deployment_sidecars`).WillReturnResult(sqlmock.NewResult(0, 0))
	deployMock.ExpectExec(`DELETE FROM deployment_variables`).WillReturnResult(sqlmock.NewResult(0, 0))
	// Event insert
	deployMock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Normalized spec re-inserts (agent workload + service + collector workload + services + variables)
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	expectVariableInsertsByName(
		deployMock,
		"SLACK_BOT_TOKEN",
		"SLACK_APP_TOKEN",
		"SLACK_CONFIG",
	)
	deployMock.ExpectExec(`INSERT INTO deployment_resolved_keys`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectCommit()

	body := deployableSpec(depID)
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "pending" {
		t.Errorf("expected status 'pending', got %v", resp["status"])
	}
}

func TestDeploy_WithDeploymentID_NotFound(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupDeployRouter("user-1")

	expectDeployPrep(accountMock, indexMock)

	// GetDeploymentByID returns no rows
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(emptyDeploymentByIDRows())

	body := deployableSpec("nonexistent")
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeploy_WithDeploymentID_InactiveRejected(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupDeployRouter("user-1")

	expectDeployPrep(accountMock, indexMock)

	now := time.Now()
	later := now.Add(time.Hour)

	// GetDeploymentByID returns inactive deployment
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow("dep-inactive", "acct-1", "my-agent", "build-1", "astro-old",
			"Old", `{}`, "undeployed", now, &later))

	body := deployableSpec("dep-inactive")
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for inactive deployment, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- GetDeploymentStatus tests ---

func setupGetDeploymentStatusRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments/:id/status", GetDeploymentStatus(log, accountStore, deployStore, nil, nil))

	return router, deployMock, accountMock
}

func TestGetDeploymentStatus_Success(t *testing.T) {
	router, deployMock, accountMock := setupGetDeploymentStatusRouter(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	// GetDeploymentByID
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "active", now, nil))

	// GetByID (account lookup for permission + avatar resolution)
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow(acctID, "myaccount", "personal", nil, nil, now, now, ""))

	// IsMember check
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetDeploymentEvents — columns: id, deployment_id, status, message, details, created_at
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "deployment_id", "status", "message", "details", "created_at"}).
			AddRow(int64(1), depID, "active", "", json.RawMessage(nil), now))

	// GetRevisions — columns: id, deployment_id, revision, build_id, spec_json, kms_ciphertext, kms_key_id, created_at
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "deployment_id", "revision", "build_id", "spec_json", "kms_ciphertext", "kms_key_id", "created_at"}).
			AddRow(int64(1), depID, 1, "build-1", json.RawMessage(`{}`), []byte(nil), (*string)(nil), now))

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID+"/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["deployment_id"] != depID {
		t.Errorf("expected deployment_id %q, got %v", depID, resp["deployment_id"])
	}
	if resp["status"] != "active" {
		t.Errorf("expected status 'active', got %v", resp["status"])
	}
	if resp["events"] == nil {
		t.Error("expected events in response")
	}
	if resp["revisions"] == nil {
		t.Error("expected revisions in response")
	}
}

func TestGetDeploymentStatus_NotFound(t *testing.T) {
	router, deployMock, _ := setupGetDeploymentStatusRouter(t)

	depID := deployid.New()

	// GetDeploymentByID returns no rows
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(emptyDeploymentByIDRows())

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID+"/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// --- WakeUpDeployment tests ---

func setupWakeUpRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.POST("/api/v1/deployments/:id/wakeup", WakeUpDeployment(log, accountStore, deployStore, &mockQueue{}, nil))

	return router, deployMock, accountMock
}

// deploymentByIDRowWithStatus is like deploymentByIDRow but allows specifying a custom current_revision.
func deploymentByIDRowWithStatus(id, accountID, agentName, buildID, namespace, displayName, specJSON, status string, revision *int, now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "account_id", "agent_name", "build_id", "namespace",
		"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at",
	}).AddRow(
		id, accountID, agentName, buildID, namespace,
		displayName, specJSON, []byte(nil), (*string)(nil),
		status, (*string)(nil), json.RawMessage(nil), now, revision,
		now, (*time.Time)(nil),
	)
}

func TestWakeUpDeployment_Success(t *testing.T) {
	router, deployMock, accountMock := setupWakeUpRouter(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()
	rev := 1

	// GetDeploymentByID — scaled_down status
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRowWithStatus(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "scaled_down", &rev, now))

	// IsMember check
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// UpdateStatus (pending) — begins a transaction, updates status, inserts event, commits
	deployMock.ExpectBegin()
	deployMock.ExpectExec(`UPDATE`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectCommit()

	req := httptest.NewRequest("POST", "/api/v1/deployments/"+depID+"/wakeup", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "pending" {
		t.Errorf("expected status 'pending', got %v", resp["status"])
	}
	if resp["deployment_id"] != depID {
		t.Errorf("expected deployment_id %q, got %v", depID, resp["deployment_id"])
	}
}

func TestWakeUpDeployment_NotScaledDown(t *testing.T) {
	router, deployMock, _ := setupWakeUpRouter(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	// GetDeploymentByID — active status (not scaled_down)
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "active", now, nil))

	req := httptest.NewRequest("POST", "/api/v1/deployments/"+depID+"/wakeup", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "deployment is not stopped or scaled down" {
		t.Errorf("expected error 'deployment is not stopped or scaled down', got %v", resp["error"])
	}
}

// --- RollbackDeployment tests ---

func setupRollbackRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.POST("/api/v1/deployments/:id/rollback", RollbackDeployment(log, accountStore, deployStore, &mockQueue{}, nil))

	return router, deployMock, accountMock
}

func TestRollbackDeployment_Success(t *testing.T) {
	router, deployMock, accountMock := setupRollbackRouter(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()
	rev := 2

	// GetDeploymentByID — active deployment with current_revision=2
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRowWithStatus(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "active", &rev, now))

	// IsMember check
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// SetCurrentRevision — begin tx, check revision exists, update current_revision, update status, insert event, commit
	deployMock.ExpectBegin()
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	deployMock.ExpectExec(`UPDATE`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`UPDATE`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectCommit()

	body := `{"revision": 1}`
	req := httptest.NewRequest("POST", "/api/v1/deployments/"+depID+"/rollback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "pending" {
		t.Errorf("expected status 'pending', got %v", resp["status"])
	}
	if resp["deployment_id"] != depID {
		t.Errorf("expected deployment_id %q, got %v", depID, resp["deployment_id"])
	}
	if resp["current_revision"] != float64(1) {
		t.Errorf("expected current_revision 1, got %v", resp["current_revision"])
	}
	if resp["message"] != "Rolling back to revision 1" {
		t.Errorf("expected rollback message, got %v", resp["message"])
	}
}

func TestRollbackDeployment_WrongStatus(t *testing.T) {
	router, deployMock, _ := setupRollbackRouter(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()
	rev := 1

	// GetDeploymentByID — pending status (cannot rollback)
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRowWithStatus(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "pending", &rev, now))

	body := `{"revision": 1}`
	req := httptest.NewRequest("POST", "/api/v1/deployments/"+depID+"/rollback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "can only rollback active or failed deployments" {
		t.Errorf("expected rollback status error, got %v", resp["error"])
	}
}

// Handler-level integration test for the variable consolidation migration. Submits
// a deploy spec containing both SLACK_CONFIG and the three legacy individual Slack
// variables. The mock DB only expects INSERTs for SLACK_BOT_TOKEN, SLACK_APP_TOKEN,
// and SLACK_CONFIG — any unexpected INSERT (from a leaked legacy var) would cause
// sqlmock to error and the handler to return non-202. ExpectationsWereMet confirms
// no stale variables slipped through to persistence.
func TestDeploy_LegacyVariablesStripped_DeploySucceeds(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupDeployRouter("user-1")

	expectDeployPrep(accountMock, indexMock)

	deployMock.ExpectQuery(`SELECT`). // GetActiveDeployment — no existing
						WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn",
			"status", "deployed_at", "undeployed_at",
		}))

	deployMock.ExpectBegin()
	deployMock.ExpectQuery(`UPDATE deployments`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	deployMock.ExpectQuery(`INSERT INTO deployments`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at",
		}).AddRow("new-id", "acct-1", "my-agent", "build-1", "astro-new", "", "{}", "pending", time.Now()))
	deployMock.ExpectExec(`INSERT INTO deployment_revisions`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	expectVariableInsertsByName(
		deployMock,
		"SLACK_BOT_TOKEN",
		"SLACK_APP_TOKEN",
		"SLACK_CONFIG",
	)
	deployMock.ExpectExec(`INSERT INTO deployment_resolved_keys`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectCommit()

	body := deployableSpecWithLegacySlackVars()
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}

	if err := deployMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled deploy expectations (legacy vars may have leaked): %v", err)
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "pending" {
		t.Errorf("expected status 'pending', got %v", resp["status"])
	}
}

// deployableSpecWithLegacySlackVars returns a deploy payload that includes the
// three legacy individual Slack variables alongside SLACK_CONFIG, mimicking what
// an older client or cached form would submit after a spec upgrade.
func deployableSpecWithLegacySlackVars() string {
	return `{
		"spec": "deployment/v1",
		"source": {"account": "myorg", "name": "my-agent", "build": "build-1", "registry": "https://123456789.dkr.ecr.us-east-1.amazonaws.com"},
		"target": {"runtime": "kubernetes"},
		"agent": {
			"image": "123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1",
			"endpoints": {"http": {"port": 8080, "protocol": "http"}},
			"replicas": 1,
			"resources": {"cpu": "100m", "memory": "256Mi", "cpu_limit": "1", "memory_limit": "1Gi"},
			"environment": {"ASTRO_AGENT_NAME": "my-agent", "ASTRO_AGENT_BUILD": "build-1"},
			"update": {"strategy": "rolling", "max_unavailable": "25%", "max_surge": "25%"}
		},
		"variables": {
			"SLACK_BOT_TOKEN": {"secret": true, "optional": true, "targets": ["interface.slack"]},
			"SLACK_APP_TOKEN": {"secret": true, "optional": true, "targets": ["interface.slack"]},
			"SLACK_CONFIG": {"secret": false, "optional": true, "targets": ["interface.slack"]},
			"SLACK_ACTIONABLE_REACTIONS": {"secret": false, "optional": true, "targets": ["interface.slack"], "value": "ticket"},
			"SLACK_ALLOWED_CHANNEL_IDS": {"secret": false, "optional": true, "targets": ["interface.slack"], "value": "C123"},
			"SLACK_ALLOWED_USER_IDS": {"secret": false, "optional": true, "targets": ["interface.slack"], "value": ""}
		},
		"observability": {"enabled": true, "provider": "langfuse"}
	}`
}

// TestImagePullPolicyForMode verifies that local mode returns IfNotPresent
// (allowing locally-built images to be used as-is while still pulling third-
// party images on first use) and all other modes return Always.
func TestImagePullPolicyForMode(t *testing.T) {
	cases := []struct {
		mode string
		want corev1.PullPolicy
	}{
		{"local", corev1.PullIfNotPresent},
		{"", corev1.PullAlways},
		{"prod", corev1.PullAlways},
		{"staging", corev1.PullAlways},
	}
	for _, tc := range cases {
		name := tc.mode
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			got := imagePullPolicyForMode(tc.mode)
			if got != tc.want {
				t.Errorf("imagePullPolicyForMode(%q) = %v, want %v", tc.mode, got, tc.want)
			}
		})
	}
}

// --- GetDeploymentLogs tests ---

func setupLogsTest(t *testing.T, lokiClient *loki.Client) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments/:id/logs",
		GetDeploymentLogs(log, accountStore, cfg, nil, deployStore, lokiClient))

	return router, deployMock, accountMock
}

func TestGetDeploymentLogs_LokiPath(t *testing.T) {
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "streams",
				"result": [{
					"stream": {"pod": "my-pod"},
					"values": [
						["1000000000", "line one\n"],
						["2000000000", "line two\n"]
					]
				}]
			}
		}`)) //nolint:errcheck
	}))
	defer lokiSrv.Close()

	lokiClient := loki.New(lokiSrv.URL)
	router, deployMock, accountMock := setupLogsTest(t, lokiClient)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123-0",
			"My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet,
		"/api/v1/deployments/"+depID+"/logs?account=my-acct", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var entries []struct {
		Timestamp string `json:"timestamp"`
		Level     string `json:"level"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Timestamp != "1970-01-01T00:00:01Z" {
		t.Errorf("entries[0].Timestamp = %q, want 1970-01-01T00:00:01Z", entries[0].Timestamp)
	}
	if entries[0].Message != "line one" {
		t.Errorf("entries[0].Message = %q, want \"line one\"", entries[0].Message)
	}
	if entries[1].Timestamp != "1970-01-01T00:00:02Z" {
		t.Errorf("entries[1].Timestamp = %q, want 1970-01-01T00:00:02Z", entries[1].Timestamp)
	}
	if entries[1].Message != "line two" {
		t.Errorf("entries[1].Message = %q, want \"line two\"", entries[1].Message)
	}
}

func TestGetDeploymentLogs_LokiPath_PodOptional(t *testing.T) {
	var gotQuery string
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[]}}`)) //nolint:errcheck
	}))
	defer lokiSrv.Close()

	lokiClient := loki.New(lokiSrv.URL)
	router, deployMock, accountMock := setupLogsTest(t, lokiClient)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-ns-0",
			"My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	w := httptest.NewRecorder()
	// No pod param — should be fine with Loki
	req, _ := http.NewRequest(http.MethodGet,
		"/api/v1/deployments/"+depID+"/logs?account=my-acct", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	// Query should use namespace only (no pod label)
	if gotQuery != `{namespace="astro-ns-0"}` {
		t.Errorf("loki query = %q, want {namespace=\"astro-ns-0\"}", gotQuery)
	}
}

// Submits a deployment spec with a schedule ingestion containing a valid cron
// expression. The handler regenerates the template from the registered agent spec
// (which includes a schedule trigger), runs EnforceEditable and ValidateAndResolve,
// and should accept the spec with 202.
func TestDeploy_WithScheduleIngestion_Succeeds(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupDeployRouter("user-1")

	expectDeployPrepWithIngestion(accountMock, indexMock)

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn",
			"status", "deployed_at", "undeployed_at",
		}))

	deployMock.ExpectBegin()
	deployMock.ExpectQuery(`UPDATE deployments`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	deployMock.ExpectQuery(`INSERT INTO deployments`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at",
		}).AddRow("new-sched-id", "acct-1", "my-agent", "build-1", "astro-new", "", "{}", "pending", time.Now()))
	deployMock.ExpectExec(`INSERT INTO deployment_revisions`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Normalized insert order: agent workload → agent service → ingestion workload
	// → collector workload → collector services → variables → resolved keys
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	deployMock.ExpectExec(`INSERT INTO deployment_variables`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_variables`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_variables`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_resolved_keys`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectCommit()

	body := deployableSpecWithScheduleIngestion("0 0 * * *")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := deployMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Submits a deployment spec with a schedule ingestion but an empty cron
// expression. The resolver should reject it with a validation error.
func TestDeploy_WithEmptySchedule_Rejected(t *testing.T) {
	router, indexMock, accountMock, _ := setupDeployRouter("user-1")

	expectDeployPrepWithIngestion(accountMock, indexMock)

	body := deployableSpecWithScheduleIngestion("")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	errors, ok := resp["validation_errors"].([]interface{})
	if !ok || len(errors) == 0 {
		t.Fatalf("expected validation_errors array, got %v", resp)
	}
	firstErr := errors[0].(map[string]interface{})
	msg, _ := firstErr["message"].(string)
	if !strings.Contains(msg, "cron expression required") {
		t.Errorf("expected cron error, got: %s", msg)
	}
}

// expectDeployPrepWithIngestion sets up mocks for prepareDeployment with an
// agent spec that includes a schedule ingestion trigger.
func expectDeployPrepWithIngestion(accountMock, indexMock sqlmock.Sqlmock) {
	now := time.Now()
	specJSON := `{"name":"my-agent","ingestion":{"daily":{"container":{"image":"docker.io/library/my-agent:build-1"},"trigger":{"type":"schedule"}}},"agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "r.io", "public", nil, now, now))
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", specJSON, "", "", "[]", now, now))

	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent", "build-1").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", specJSON, "", "", "[]", now, now))
}

func deployableSpecWithScheduleIngestion(schedule string) string {
	return fmt.Sprintf(`{
		"spec": "deployment/v1",
		"source": {"account": "myorg", "name": "my-agent", "build": "build-1", "registry": "https://123456789.dkr.ecr.us-east-1.amazonaws.com"},
		"target": {"runtime": "kubernetes"},
		"agent": {
			"image": "123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1",
			"endpoints": {"http": {"port": 8080, "protocol": "http"}},
			"replicas": 1,
			"resources": {"cpu": "100m", "memory": "256Mi", "cpu_limit": "1", "memory_limit": "1Gi"},
			"environment": {"ASTRO_AGENT_NAME": "my-agent", "ASTRO_AGENT_BUILD": "build-1"},
			"update": {"strategy": "rolling", "max_unavailable": "25%%", "max_surge": "25%%"}
		},
		"variables": {
			"SLACK_BOT_TOKEN": {"secret": true, "optional": true, "targets": ["interface.slack"]},
			"SLACK_APP_TOKEN": {"secret": true, "optional": true, "targets": ["interface.slack"]},
			"SLACK_CONFIG": {"secret": false, "optional": true, "targets": ["interface.slack"]}
		},
		"ingestion": {
			"daily": {
				"image": "docker.io/library/my-agent:build-1",
				"trigger": {"type": "schedule", "schedule": %q},
				"resources": {"cpu": "100m", "memory": "256Mi", "cpu_limit": "1", "memory_limit": "1Gi"}
			}
		},
		"observability": {"enabled": true, "provider": "langfuse"}
	}`, schedule)
}

func TestInjectManagedCredentials(t *testing.T) {
	t.Run("injects ANTHROPIC_API_KEY when configured", func(t *testing.T) {
		resolved := &deployment.ResolvedEnv{
			ConfigMapData: map[string]string{},
			SecretData:    map[string]string{},
		}
		cfg := &config.Config{
			Deployment: config.DeploymentConfig{
				ManagedAnthropicAPIKey: "sk-managed-test-key",
			},
		}
		injectManagedCredentials(resolved, cfg)
		if got := resolved.SecretData["ANTHROPIC_API_KEY"]; got != "sk-managed-test-key" {
			t.Errorf("ANTHROPIC_API_KEY = %q, want %q", got, "sk-managed-test-key")
		}
	})

	t.Run("does not inject when not configured", func(t *testing.T) {
		resolved := &deployment.ResolvedEnv{
			ConfigMapData: map[string]string{},
			SecretData:    map[string]string{},
		}
		cfg := &config.Config{
			Deployment: config.DeploymentConfig{},
		}
		injectManagedCredentials(resolved, cfg)
		if _, ok := resolved.SecretData["ANTHROPIC_API_KEY"]; ok {
			t.Error("ANTHROPIC_API_KEY should not be present when ManagedAnthropicAPIKey is empty")
		}
	})

	t.Run("does not overwrite existing user-provided key", func(t *testing.T) {
		resolved := &deployment.ResolvedEnv{
			ConfigMapData: map[string]string{},
			SecretData: map[string]string{
				"ANTHROPIC_API_KEY": "sk-user-provided",
			},
		}
		cfg := &config.Config{
			Deployment: config.DeploymentConfig{
				ManagedAnthropicAPIKey: "sk-managed-key",
			},
		}
		injectManagedCredentials(resolved, cfg)
		// Current behavior: managed key overwrites. This is fine because if the
		// user uses anthropic-managed, there's no user variable, so no conflict.
		// If they use regular anthropic, managed key is empty.
		if got := resolved.SecretData["ANTHROPIC_API_KEY"]; got != "sk-managed-key" {
			t.Errorf("ANTHROPIC_API_KEY = %q, want %q", got, "sk-managed-key")
		}
	})
}

func TestGetDeploymentLogs_NoBackend_Returns503(t *testing.T) {
	router, deployMock, accountMock := setupLogsTest(t, nil /* no Loki, no k8s */)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123-0",
			"My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet,
		"/api/v1/deployments/"+depID+"/logs?account=my-acct&pod=my-pod", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

// --- StreamDeploymentLogs tests ---

var streamWSUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func setupStreamLogsTest(t *testing.T, lokiClient *loki.Client) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments/:id/logs/stream",
		StreamDeploymentLogs(log, accountStore, cfg, nil, deployStore, lokiClient))

	return router, deployMock, accountMock
}

func TestStreamDeploymentLogs_Unauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountDB, _, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, _, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	router := gin.New()
	// No auth middleware — user is not set.
	router.GET("/api/v1/deployments/:id/logs/stream",
		StreamDeploymentLogs(logger.New("error", "json"), account.NewAccountStore(accountDB),
			&config.Config{}, nil, deploymentstore.NewStore(deployDB), nil))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/logs/stream?account=my-acct", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestStreamDeploymentLogs_NoBackend_Returns503(t *testing.T) {
	router, deployMock, accountMock := setupStreamLogsTest(t, nil /* no Loki, no k8s */)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123-0",
			"My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet,
		"/api/v1/deployments/"+depID+"/logs/stream?account=my-acct", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestStreamDeploymentLogs_LokiPath(t *testing.T) {
	// Mock Loki server: query_range returns historical line; tail WS delivers live line.
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/loki/api/v1/query_range":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"pod":"my-pod"},"values":[["1000000000","historical line"]]}]}}`)) //nolint:errcheck
		case "/loki/api/v1/tail":
			conn, err := streamWSUpgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Errorf("websocket upgrade: %v", err)
				return
			}
			defer conn.Close() //nolint:errcheck
			frame := map[string]interface{}{
				"streams": []map[string]interface{}{{
					"stream": map[string]string{"pod": "my-pod"},
					"values": [][]string{{"2000000000", "live line"}},
				}},
			}
			data, _ := json.Marshal(frame)
			conn.WriteMessage(websocket.TextMessage, data) //nolint:errcheck
			conn.WriteMessage(websocket.CloseMessage,      //nolint:errcheck
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			conn.ReadMessage() //nolint:errcheck
		default:
			http.NotFound(w, r)
		}
	}))
	defer lokiSrv.Close()

	lokiClient := loki.New(lokiSrv.URL)
	router, deployMock, accountMock := setupStreamLogsTest(t, lokiClient)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123-0",
			"My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Use a real HTTP server so SSE streaming (http.Flusher) works.
	srv := httptest.NewServer(router)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		srv.URL+"/api/v1/deployments/"+depID+"/logs/stream?account=my-acct", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	// Collect SSE lines until EOF (handler exits after WS closes).
	var sseLines []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			sseLines = append(sseLines, line)
		}
	}

	// Expect: event:ready (emitted on connect), then both log data lines from tail.
	var logLines, eventLines []string
	for _, l := range sseLines {
		if strings.HasPrefix(l, "data:") && l != "data: {}" {
			logLines = append(logLines, l)
		} else if strings.HasPrefix(l, "event:") {
			eventLines = append(eventLines, l)
		}
	}

	if len(logLines) < 2 {
		t.Errorf("got %d log data lines, want ≥2; all lines: %v", len(logLines), sseLines)
	}
	if len(logLines) >= 1 && !strings.Contains(logLines[0], "historical line") {
		t.Errorf("first log line should contain historical line, got: %s", logLines[0])
	}
	if len(logLines) >= 2 && !strings.Contains(logLines[len(logLines)-1], "live line") {
		t.Errorf("last log line should contain live line, got: %s", logLines[len(logLines)-1])
	}
	if len(eventLines) == 0 || !strings.Contains(eventLines[0], "ready") {
		t.Errorf("expected event: ready, got: %v", eventLines)
	}
}

func TestStreamDeploymentLogs_LokiPath_EmitsIDFields(t *testing.T) {
	// Timestamp 1000000000 ns = 1s since epoch — delivered via backfill query_range.
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/loki/api/v1/query_range":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"pod":"p"},"values":[["1000000000","log line"]]}]}}`)) //nolint:errcheck
		case "/loki/api/v1/tail":
			conn, err := streamWSUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()                                                                                        //nolint:errcheck
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")) //nolint:errcheck
			conn.ReadMessage()                                                                                        //nolint:errcheck
		default:
			http.NotFound(w, r)
		}
	}))
	defer lokiSrv.Close()

	lokiClient := loki.New(lokiSrv.URL)
	router, deployMock, accountMock := setupStreamLogsTest(t, lokiClient)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123-0",
			"My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	srv := httptest.NewServer(router)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		srv.URL+"/api/v1/deployments/"+depID+"/logs/stream?account=my-acct", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var idLines []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if line := scanner.Text(); strings.HasPrefix(line, "id:") {
			idLines = append(idLines, line)
		}
	}

	if len(idLines) == 0 {
		t.Fatal("expected id: fields in SSE output, got none")
	}
	if !strings.Contains(idLines[0], "1000000000") {
		t.Errorf("id field = %q, want nanosecond timestamp 1000000000", idLines[0])
	}
}

func TestStreamDeploymentLogs_LokiPath_LastEventID_AdjustsHistStart(t *testing.T) {
	var gotStart string
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/tail" {
			http.NotFound(w, r)
			return
		}
		gotStart = r.URL.Query().Get("start")
		conn, err := streamWSUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()                                                                                        //nolint:errcheck
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")) //nolint:errcheck
		conn.ReadMessage()                                                                                        //nolint:errcheck
	}))
	defer lokiSrv.Close()

	lokiClient := loki.New(lokiSrv.URL)
	router, deployMock, accountMock := setupStreamLogsTest(t, lokiClient)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123-0",
			"My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	srv := httptest.NewServer(router)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		srv.URL+"/api/v1/deployments/"+depID+"/logs/stream?account=my-acct", nil)
	req.Header.Set("Last-Event-ID", "5000000000")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	// Tail WS should receive start=5000000001 (Last-Event-ID + 1 ns, exclusive).
	if gotStart != "5000000001" {
		t.Errorf("Loki tail start = %q, want 5000000001", gotStart)
	}
}

func TestStreamDeploymentLogs_LokiPath_LastEventID_InvalidIgnored(t *testing.T) {
	var gotStart string
	before := time.Now()
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/loki/api/v1/query_range":
			// Backfill is attempted on invalid Last-Event-ID; return empty result.
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[]}}`)) //nolint:errcheck
		case "/loki/api/v1/tail":
			gotStart = r.URL.Query().Get("start")
			conn, err := streamWSUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()                                                                                        //nolint:errcheck
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")) //nolint:errcheck
			conn.ReadMessage()                                                                                        //nolint:errcheck
		default:
			http.NotFound(w, r)
		}
	}))
	defer lokiSrv.Close()

	lokiClient := loki.New(lokiSrv.URL)
	router, deployMock, accountMock := setupStreamLogsTest(t, lokiClient)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123-0",
			"My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	srv := httptest.NewServer(router)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		srv.URL+"/api/v1/deployments/"+depID+"/logs/stream?account=my-acct", nil)
	req.Header.Set("Last-Event-ID", "not-a-number")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	// Invalid Last-Event-ID is ignored — tail should start from approximately now.
	startNano, err := strconv.ParseInt(gotStart, 10, 64)
	if err != nil {
		t.Fatalf("parse tail start %q: %v", gotStart, err)
	}
	gotTime := time.Unix(0, startNano)
	if gotTime.Before(before) || gotTime.After(time.Now().Add(5*time.Second)) {
		t.Errorf("tail start %v should be approximately now", gotTime)
	}
}

// --- StopDeployment tests ---

func setupStopRouter(t *testing.T, k8sHandler http.Handler) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")

	// Use JSON content-type so request bodies are readable as JSON in tests.
	srv := httptest.NewServer(k8sHandler)
	t.Cleanup(srv.Close)
	cs, _ := kubernetes.NewForConfig(&rest.Config{
		Host:          srv.URL,
		ContentConfig: rest.ContentConfig{ContentType: "application/json"},
	})
	k8sClient := &mockClusterClient{clientset: cs}

	router := gin.New()
	router.Use(setAuthUser("user-1"))
	router.POST("/api/v1/deployments/:id/stop", StopDeployment(log, accountStore, k8sClient, deployStore, nil, k8scache.NoopCache{}))

	return router, deployMock, accountMock
}

func expectStopDBMocks(t *testing.T, deployMock, accountMock sqlmock.Sqlmock, depID, acctID, namespace string, now time.Time) {
	t.Helper()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", namespace, "My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// UpdateStatus transaction
	deployMock.ExpectBegin()
	deployMock.ExpectExec(`UPDATE`).WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT`).WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectCommit()
}

func TestStopDeployment_SuspendsCronJobs(t *testing.T) {
	namespace := "astro-abc123-0"
	var suspendedCronJob bool

	k8sHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && strings.Contains(path, "/deployments"):
			fmt.Fprintf(w, `{"kind":"DeploymentList","apiVersion":"apps/v1","items":[{"metadata":{"name":"agent","namespace":%q},"spec":{"replicas":1}}]}`, namespace)
		case r.Method == http.MethodPut && strings.Contains(path, "/deployments/"):
			fmt.Fprintf(w, `{"kind":"Deployment","apiVersion":"apps/v1","metadata":{"name":"agent","namespace":%q}}`, namespace)
		case r.Method == http.MethodGet && strings.Contains(path, "/statefulsets"):
			fmt.Fprint(w, `{"kind":"StatefulSetList","apiVersion":"apps/v1","items":[]}`)
		case r.Method == http.MethodGet && strings.Contains(path, "/cronjobs"):
			fmt.Fprintf(w, `{"kind":"CronJobList","apiVersion":"batch/v1","items":[{"metadata":{"name":"my-agent-ingestion-daily","namespace":%q},"spec":{"schedule":"0 0 * * *"}}]}`, namespace)
		case r.Method == http.MethodPut && strings.Contains(path, "/cronjobs/"):
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				if spec, ok := body["spec"].(map[string]interface{}); ok {
					suspendedCronJob, _ = spec["suspend"].(bool)
				}
			}
			fmt.Fprintf(w, `{"kind":"CronJob","apiVersion":"batch/v1","metadata":{"name":"my-agent-ingestion-daily","namespace":%q}}`, namespace)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	router, deployMock, accountMock := setupStopRouter(t, k8sHandler)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	expectStopDBMocks(t, deployMock, accountMock, depID, acctID, namespace, now)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/deployments/"+depID+"/stop", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "stopped" {
		t.Errorf("expected status 'stopped', got %v", resp["status"])
	}
	if !suspendedCronJob {
		t.Error("expected CronJob Spec.Suspend=true in PUT request")
	}
}

func TestStopDeployment_NoCronJobs(t *testing.T) {
	namespace := "astro-abc123-0"

	k8sHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && strings.Contains(path, "/apis/apps/v1/namespaces/") && strings.HasSuffix(path, "/deployments"):
			fmt.Fprintf(w, `{"kind":"DeploymentList","apiVersion":"apps/v1","items":[{"metadata":{"name":"agent","namespace":%q},"spec":{"replicas":1}}]}`, namespace)
		case r.Method == http.MethodPut && strings.Contains(path, "/apis/apps/v1/namespaces/") && strings.Contains(path, "/deployments/"):
			fmt.Fprintf(w, `{"kind":"Deployment","apiVersion":"apps/v1","metadata":{"name":"agent","namespace":%q}}`, namespace)
		case r.Method == http.MethodGet && strings.Contains(path, "/apis/apps/v1/namespaces/") && strings.HasSuffix(path, "/statefulsets"):
			fmt.Fprint(w, `{"kind":"StatefulSetList","apiVersion":"apps/v1","items":[]}`)
		case r.Method == http.MethodGet && strings.Contains(path, "/apis/batch/v1/namespaces/") && strings.HasSuffix(path, "/cronjobs"):
			fmt.Fprint(w, `{"kind":"CronJobList","apiVersion":"batch/v1","items":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	router, deployMock, accountMock := setupStopRouter(t, k8sHandler)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	expectStopDBMocks(t, deployMock, accountMock, depID, acctID, namespace, now)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/deployments/"+depID+"/stop", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStopDeployment_Undeploying_Returns400(t *testing.T) {
	k8sHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	router, deployMock, accountMock := setupStopRouter(t, k8sHandler)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123-0", "My Agent", `{}`, "undeploying", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/deployments/"+depID+"/stop", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// --- GetDeployment tests ---

func setupGetDeploymentTest(t *testing.T, k8sHandler http.Handler) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{}
	k8sClient := newMockK8sClient(k8sHandler)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments/:id", GetDeployment(log, accountStore, cfg, k8sClient, deployStore, nil, nil, k8scache.NoopCache{}))

	return router, deployMock, accountMock
}

func TestGetDeployment_Success(t *testing.T) {
	depID := deployid.New()
	namespace := "astro-abc123def-0"
	agentName := "my-agent"
	buildID := "build-1"
	acctID := uuid.New().String()
	now := time.Now()

	router, deployMock, accountMock := setupGetDeploymentTest(t, k8sListHandler(namespace, agentName, buildID))

	// deployStore.GetDeploymentByID
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, agentName, buildID, namespace, "My Agent", `{}`, "active", now, nil))

	// accountStore.IsMember (called by resolveDeployment)
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Deployment AgentDeployment `json:"deployment"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Deployment.ID != depID {
		t.Errorf("expected ID %q, got %q", depID, resp.Deployment.ID)
	}
	if resp.Deployment.Name != agentName {
		t.Errorf("expected name %q, got %q", agentName, resp.Deployment.Name)
	}
	if resp.Deployment.DisplayName != "My Agent" {
		t.Errorf("expected display_name 'My Agent', got %q", resp.Deployment.DisplayName)
	}
}

func TestGetDeployment_NoNamespace_ReturnsDBEntry(t *testing.T) {
	depID := deployid.New()
	namespace := "astro-abc123def-0"
	acctID := uuid.New().String()
	now := time.Now()

	// K8s handler returns 404 for all requests (namespace does not exist)
	k8sHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	router, deployMock, accountMock := setupGetDeploymentTest(t, k8sHandler)

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", namespace, "My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Deployment AgentDeployment `json:"deployment"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Deployment.ID != depID {
		t.Errorf("expected ID %q, got %q", depID, resp.Deployment.ID)
	}
}

func TestGetDeployment_NotFound(t *testing.T) {
	router, deployMock, _ := setupGetDeploymentTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(emptyDeploymentByIDRows())

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+deployid.New(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDeploy_SourcePropertiesFromDB verifies that source.account and source.build in the
// deployment template are taken from the database, not the client-submitted spec.
//
// Setup: the DB's agentVersion.BuildID is "canonical-build", but the client submits a spec
// with source.build = "build-1" (the ID used to look up the version). After the fix,
// the template is stamped with "canonical-build" from the DB, so EnforceEditable rejects
// the submitted spec (which still says "build-1") with a 400.
func TestDeploy_SourcePropertiesFromDB(t *testing.T) {
	router, indexMock, accountMock, _ := setupDeployRouter("user-1")

	now := time.Now()

	// accountStore.GetByName("myorg") → DB returns name="myorg"
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, ""))

	// IsMember → yes
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get (visibility check)
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "r.io", "public", nil, now, now))
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))

	// agentIndex.GetVersion — queried with "build-1" but DB returns canonical "canonical-build"
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent", "build-1").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("canonical-build", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))

	// Submit a spec with source.build = "build-1" (does not match the DB's "canonical-build")
	body := deployableSpec("")
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Template has source.build="canonical-build" from DB; submitted spec has "build-1".
	// EnforceEditable must reject the mismatch.
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when source.build differs from DB value, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] != "server-owned fields were modified" {
		t.Errorf("expected 'server-owned fields were modified' error, got: %v", resp["error"])
	}
}

func TestGetDeployment_NotMember(t *testing.T) {
	router, deployMock, accountMock := setupGetDeploymentTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc-0", "My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Variable ref security tests for GetPrefilledDeploymentTemplate ---
//
// These tests cover the fix for the security issue where secret variable values
// were being returned in the prefilled template instead of the original account
// variable reference. The fix ensures:
//   - Variables set via account variable refs → ref is restored, value is hidden
//   - Secret variables set directly → value is hidden (never expose plaintext)
//   - Non-secret variables set directly → value is returned as-is

// specWithVarInputs is a minimal agent spec JSON that declares two inputs:
// API_KEY (secret) and LOG_LEVEL (non-secret). These become entries in
// template.Variables so the prefilled-template merge logic has keys to populate.
const specWithVarInputs = `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1","inputs":[{"name":"API_KEY","secret":true,"description":"API key"},{"name":"LOG_LEVEL","secret":false,"description":"Log level"}]}}`

// setupPrefilledVarsRouter wires GetPrefilledDeploymentTemplate and sets up all
// sqlmock expectations except GetDeploymentVariables (the caller adds that).
// Returns the router and the deploy DB mock to append variable row expectations.
func setupPrefilledVarsRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	indexDB, indexMock, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	index := agentindex.NewIndexWithDB(indexDB)
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{
		Deployment: config.DeploymentConfig{
			RegistryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			Environment: "test",
		},
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/agents/:account/:name/deployment-template/:deploymentID",
		GetPrefilledDeploymentTemplate(log, index, accountStore, cfg, deployStore))

	now := time.Now()
	depID := "dep-vars-test"
	acctID := "acct-1"

	// GetDeploymentByID
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Deploy", `{}`, "active", now, nil))
	// IsMember
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// generateTemplate: account + agent + pinned build version
	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "public")
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs(acctID, "my-agent", "build-1").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", specWithVarInputs, "", "", "[]", now, now))
	// GetByID for target.account resolution (after variable merge)
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow(acctID, "myorg", "organization", nil, nil, now, now, ""))

	return router, deployMock
}

// prefilledVarsRequest fires a GET to the prefilled template endpoint and
// returns the parsed variables map from the JSON response.
func prefilledVarsRequest(t *testing.T, router *gin.Engine) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet,
		"/agents/myorg/my-agent/deployment-template/dep-vars-test?format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	vars, _ := resp["variables"].(map[string]any)
	return vars
}

// TestGetPrefilledTemplate_SecretRefRestored verifies the core security fix:
// when a secret variable was deployed via an account variable reference, the
// prefilled template returns the original ref and hides the resolved value.
func TestGetPrefilledTemplate_SecretRefRestored(t *testing.T) {
	router, deployMock := setupPrefilledVarsRouter(t)

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}).AddRow("dep-vars-test", "API_KEY", "encrypted-blob", "MY_ACCOUNT_SECRET", true, false, `{"agent"}`, nil))

	vars := prefilledVarsRequest(t, router)

	apiKey, ok := vars["API_KEY"].(map[string]any)
	if !ok {
		t.Fatalf("expected API_KEY in variables, got %v", vars)
	}
	if apiKey["ref"] != "MY_ACCOUNT_SECRET" {
		t.Errorf("expected ref=%q, got %v", "MY_ACCOUNT_SECRET", apiKey["ref"])
	}
	if v, hasValue := apiKey["value"]; hasValue && v != "" {
		t.Errorf("expected value to be empty, got %v", v)
	}
}

// TestGetPrefilledTemplate_SecretNoRefValueHidden verifies that a secret variable
// deployed with a direct value (not a ref) never exposes the plaintext in the
// prefilled template — the value field must be absent or empty.
func TestGetPrefilledTemplate_SecretNoRefValueHidden(t *testing.T) {
	router, deployMock := setupPrefilledVarsRouter(t)

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}).AddRow("dep-vars-test", "API_KEY", "encrypted-blob", "", true, false, `{"agent"}`, nil))

	vars := prefilledVarsRequest(t, router)

	apiKey, ok := vars["API_KEY"].(map[string]any)
	if !ok {
		t.Fatalf("expected API_KEY in variables, got %v", vars)
	}
	if ref, hasRef := apiKey["ref"]; hasRef && ref != "" {
		t.Errorf("expected no ref, got %v", ref)
	}
	if v, hasValue := apiKey["value"]; hasValue && v != "" {
		t.Errorf("plaintext secret exposed in prefilled template: %v", v)
	}
}

// TestGetPrefilledTemplate_NonSecretRefRestored verifies that refs are restored
// for non-secret variables too — not just secrets.
func TestGetPrefilledTemplate_NonSecretRefRestored(t *testing.T) {
	router, deployMock := setupPrefilledVarsRouter(t)

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}).AddRow("dep-vars-test", "LOG_LEVEL", "debug", "SHARED_LOG_LEVEL", false, false, `{"agent"}`, nil))

	vars := prefilledVarsRequest(t, router)

	logLevel, ok := vars["LOG_LEVEL"].(map[string]any)
	if !ok {
		t.Fatalf("expected LOG_LEVEL in variables, got %v", vars)
	}
	if logLevel["ref"] != "SHARED_LOG_LEVEL" {
		t.Errorf("expected ref=%q, got %v", "SHARED_LOG_LEVEL", logLevel["ref"])
	}
	if v, hasValue := logLevel["value"]; hasValue && v != "" {
		t.Errorf("expected value to be empty when ref is set, got %v", v)
	}
}

// TestGetPrefilledTemplate_NonSecretDirectValue verifies that non-secret variables
// with no ref have their value returned as-is (regression guard).
func TestGetPrefilledTemplate_NonSecretDirectValue(t *testing.T) {
	router, deployMock := setupPrefilledVarsRouter(t)

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}).AddRow("dep-vars-test", "LOG_LEVEL", "debug", "", false, false, `{"agent"}`, nil))

	vars := prefilledVarsRequest(t, router)

	logLevel, ok := vars["LOG_LEVEL"].(map[string]any)
	if !ok {
		t.Fatalf("expected LOG_LEVEL in variables, got %v", vars)
	}
	if logLevel["value"] != "debug" {
		t.Errorf("expected value=%q, got %v", "debug", logLevel["value"])
	}
	if ref, hasRef := logLevel["ref"]; hasRef && ref != "" {
		t.Errorf("expected no ref, got %v", ref)
	}
}
