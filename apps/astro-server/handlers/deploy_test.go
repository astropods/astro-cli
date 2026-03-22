package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
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
	router.POST("/api/v1/undeploy", UndeployAgent(log, index, accountStore, nil, deployStore, &mockQueue{}))

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
	router.GET("/api/v1/deployments", ListDeployments(log, accountStore, cfg, k8sClient, deployStore))

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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, 0))

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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version",
		}).AddRow("acct-1", "myaccount", "organization", nil, nil, now, now, 0))

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

func TestListDeployments_IncludesInitContainerStatuses(t *testing.T) {
	depID := deployid.New()
	namespace := "astro-init-sidecar-0"
	agentName := "my-agent"
	buildID := "build-1"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if r.Method == http.MethodGet && strings.HasSuffix(path, "/namespaces/"+namespace) {
			fmt.Fprintf(w, `{"kind":"Namespace","apiVersion":"v1","metadata":{"name":%q}}`, namespace)
			return
		}
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
					"spec":{"replicas":1},
					"status":{"replicas":1,"readyReplicas":1,"availableReplicas":1}
				}]
			}`, namespace, agentName, buildID)
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
						}],
						"initContainerStatuses":[{
							"name":"messaging",
							"ready":true,
							"restartCount":0,
							"state":{"running":{"startedAt":"2026-03-12T21:08:24Z"}}
						}]
					},
					"spec":{
						"containers":[{"name":"app"}],
						"initContainers":[{"name":"messaging"}]
					}
				}]
			}`, namespace, agentName, buildID)
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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, 0))
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
	if len(resp.Deployments[0].Workloads) == 0 {
		t.Fatalf("expected at least one workload in response")
	}

	var names []string
	for _, c := range resp.Deployments[0].Workloads[0].Containers {
		names = append(names, c.Name)
	}
	if !slices.Contains(names, "app") || !slices.Contains(names, "messaging") {
		t.Fatalf("expected app and messaging in workload containers, got %v", names)
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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, 0))

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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, 0))

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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, 0))

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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, 0))
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
	router.GET("/api/v1/deployments", ListDeployments(log, accountStore, cfg, k8sClient, nil))

	now := time.Now()
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, 0))
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
	router.POST("/api/v1/undeploy", UndeployAgent(log, index, accountStore, nil, deployStore, &mockQueue{}))

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
			RegistryURL: "docker.io/library",
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
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version"}).
			AddRow("src-acct", "source-org", "organization", nil, nil, now, now, 0))

	// Target == source (no target.account in spec), so no second account lookup

	// IsMember(target=source, user) → member of target
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("src-acct", "user-cross").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get → private agent
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("src-acct", "secret-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "created_at", "updated_at"}).
			AddRow("src-acct", "secret-agent", "r.io", "private", now, now))
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
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version"}).
			AddRow("src-acct", "source-org", "organization", nil, nil, now, now, 0))

	// Target account lookup (different from source)
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("target-org").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version"}).
			AddRow("tgt-acct", "target-org", "organization", nil, nil, now, now, 0))

	// IsMember(target, user) → member of target account
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("tgt-acct", "user-target").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get → private agent
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("src-acct", "secret-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "created_at", "updated_at"}).
			AddRow("src-acct", "secret-agent", "r.io", "private", now, now))
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
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, 0))

	// IsMember(target=source, user) → member
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get → no rows
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "nonexistent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "created_at", "updated_at"}))

	req := httptest.NewRequest(http.MethodPost, "/deploy/validate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
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
			RegistryURL: "docker.io/library",
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
			[]string{"account_id", "name", "registry", "visibility", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", visibility, now, now))
	mock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent"}`, "", "", "[]", now, now))
}

// expectLatestVersion sets up the sqlmock expectation for agentIndex.GetLatestVersion().
func expectLatestVersion(mock sqlmock.Sqlmock) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent"}`, "", "", "[]", now, now))
}

// expectAccountLookup sets up sqlmock expectation for accountStore.GetByName().
func expectAccountLookup(mock sqlmock.Sqlmock) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, 0))
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
			[]string{"account_id", "name", "registry", "visibility", "created_at", "updated_at"}))

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
			RegistryURL: "docker.io/library",
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

	// generateTemplate expectations: account lookup, agent lookup, latest version
	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "public")
	expectLatestVersion(indexMock)

	// GetDeploymentByID
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Deploy", `{"interfaces":{"adapters":["slack"]}}`, "active", now, nil))

	// IsMember check for deployment's account
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetDeploymentVariables
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "secret", "optional", "targets", "nonce",
		}))

	// GetByID for account name resolution
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version"}).
			AddRow(acctID, "myorg", "organization", nil, nil, now, now, 0))

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

func TestGetPrefilledTemplate_DifferentBuild(t *testing.T) {
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
			RegistryURL: "docker.io/library",
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

	// generateTemplate expectations with ?build=build-2
	expectAccountLookup(accountMock)
	// agentIndex.Get (visibility check) — returns build-1 as latest but we request build-2
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", "public", now, now))
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent"}`, "", "", "[]", now, now))
	// agentIndex.GetVersion for the specific build-2
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent", "build-2").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-2", "myorg", `{"name":"my-agent"}`, "", "", "[]", now, now))

	// GetDeploymentByID (old deployment was build-1)
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Deploy", `{}`, "active", now, nil))

	// IsMember check
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetDeploymentVariables
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "secret", "optional", "targets", "nonce",
		}))

	// GetByID for account name resolution
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version"}).
			AddRow(acctID, "myorg", "organization", nil, nil, now, now, 0))

	req := httptest.NewRequest(http.MethodGet,
		"/agents/myorg/my-agent/deployment-template/"+depID+"?build=build-2&format=json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	// Template should use the new build
	source, ok := resp["source"].(map[string]any)
	if !ok {
		t.Fatal("expected source to be an object")
	}
	if source["build"] != "build-2" {
		t.Errorf("expected source.build='build-2', got %v", source["build"])
	}

	// But should still carry over the deployment_id and display_name from the old deployment
	target := resp["target"].(map[string]any)
	if target["deployment_id"] != depID {
		t.Errorf("expected deployment_id=%q, got %v", depID, target["deployment_id"])
	}
	if target["display_name"] != "My Deploy" {
		t.Errorf("expected display_name='My Deploy', got %v", target["display_name"])
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
			RegistryURL: "docker.io/library",
		},
	}

	router := gin.New()
	if userID != "" {
		router.Use(func(c *gin.Context) {
			c.Set(string(auth.UserContextKey), &auth.User{ID: userID})
			c.Next()
		})
	}
	router.POST("/deploy", DeployAgent(log, index, accountStore, cfg, deployStore, nil, &mockQueue{})) //nolint:staticcheck // nil EntitlementChecker skips checks in tests

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
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, 0))

	// IsMember(target=source, user) → yes
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get (visibility check)
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "r.io", "public", now, now))
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent"}`, "", "", "[]", now, now))

	// agentIndex.GetVersion (exact build lookup)
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent", "build-1").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent"}`, "", "", "[]", now, now))
}

func expectVariableInsertsByName(deployMock sqlmock.Sqlmock, names ...string) {
	deployMock.MatchExpectationsInOrder(false)
	for _, name := range names {
		deployMock.ExpectExec(`INSERT INTO deployment_variables`).
			WithArgs(
				sqlmock.AnyArg(),
				name,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
}

// deployableSpec builds a JSON deployment spec that matches the template the server
// generates from the agent spec `{"name":"my-agent"}` with RegistryURL "docker.io/library".
// The caller can optionally set deploymentID to test the in-place update path.
func deployableSpec(deploymentID string) string {
	targetExtra := ""
	if deploymentID != "" {
		targetExtra = fmt.Sprintf(`, "deployment_id": %q`, deploymentID)
	}
	return fmt.Sprintf(`{
		"spec": "deployment/v1",
		"source": {"account": "myorg", "name": "my-agent", "build": "build-1", "registry": "docker.io/library"},
		"target": {"runtime": "kubernetes"%s},
		"agent": {
			"image": "docker.io/library/my-agent:build-1",
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
	router.GET("/api/v1/deployments/:id/status", GetDeploymentStatus(log, accountStore, deployStore))

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
	router.POST("/api/v1/deployments/:id/wakeup", WakeUpDeployment(log, accountStore, deployStore, &mockQueue{}))

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
	if resp["error"] != "deployment is not scaled down" {
		t.Errorf("expected error 'deployment is not scaled down', got %v", resp["error"])
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
	router.POST("/api/v1/deployments/:id/rollback", RollbackDeployment(log, accountStore, deployStore, &mockQueue{}))

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
		"source": {"account": "myorg", "name": "my-agent", "build": "build-1", "registry": "docker.io/library"},
		"target": {"runtime": "kubernetes"},
		"agent": {
			"image": "docker.io/library/my-agent:build-1",
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
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", ct)
	}
	want := "1970-01-01T00:00:01Z line one\n1970-01-01T00:00:02Z line two\n"
	if got := w.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
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
	specJSON := `{"name":"my-agent","ingestion":{"daily":{"container":{"image":"docker.io/library/my-agent:build-1"},"trigger":{"type":"schedule"}}}}`

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, 0))

	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "r.io", "public", now, now))
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
		"source": {"account": "myorg", "name": "my-agent", "build": "build-1", "registry": "docker.io/library"},
		"target": {"runtime": "kubernetes"},
		"agent": {
			"image": "docker.io/library/my-agent:build-1",
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
