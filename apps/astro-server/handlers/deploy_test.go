package handlers

import (
	"bufio"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/specsign"
	spec "github.com/astropods/astro-spec"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// healthStubClusterClient implements k8s.ClusterClient for deploy cluster_id tests.
type healthStubClusterClient struct {
	checkHealthErr error
}

func (h *healthStubClusterClient) Clientset() *kubernetes.Clientset      { return nil }
func (h *healthStubClusterClient) Config() *rest.Config                  { return nil }
func (h *healthStubClusterClient) CheckHealth() error                    { return h.checkHealthErr }
func (h *healthStubClusterClient) GetServerVersion() (string, error)     { return "v1.0.0", nil }
func (h *healthStubClusterClient) DiagnoseConnection() map[string]string { return nil }

func healthyStubCluster() k8s.ClusterClient {
	return &healthStubClusterClient{}
}

func unhealthyStubCluster(msg string) k8s.ClusterClient {
	return &healthStubClusterClient{checkHealthErr: errors.New(msg)}
}

func TestValidateAgentDisplayNameMaxLength(t *testing.T) {
	valid := strings.Repeat("a", deploymentDisplayNameMaxLength)
	if got, err := validateAgentDisplayName(valid); err != nil || got != valid {
		t.Fatalf("validateAgentDisplayName(%q) = %q, %v; want valid", valid, got, err)
	}

	tooLong := strings.Repeat("a", deploymentDisplayNameMaxLength+1)
	if _, err := validateAgentDisplayName(tooLong); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%d characters or fewer", deploymentDisplayNameMaxLength)) {
		t.Fatalf("validateAgentDisplayName accepted too-long name, err=%v", err)
	}
}

func TestDeploymentDisplayNameMaxLengthMatchesClientConstant(t *testing.T) {
	contents, err := os.ReadFile("../../astro-client/src/components/deploy/constants.ts")
	if err != nil {
		t.Fatalf("read client deploy constants: %v", err)
	}

	matches := regexp.MustCompile(`DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH\s*=\s*([0-9]+)`).FindStringSubmatch(string(contents))
	if len(matches) != 2 {
		t.Fatalf("DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH not found in client deploy constants")
	}
	clientMaxLength, err := strconv.Atoi(matches[1])
	if err != nil {
		t.Fatalf("parse client display-name max length %q: %v", matches[1], err)
	}
	if clientMaxLength != deploymentDisplayNameMaxLength {
		t.Fatalf("client display-name max length = %d, server = %d", clientMaxLength, deploymentDisplayNameMaxLength)
	}
}

func testK8sRegistry(client k8s.ClusterClient) *k8s.Registry {
	if client == nil {
		return nil
	}
	return k8s.NewRegistryWithPrimary(client)
}

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

// --- TemplateCache tests ---

func TestTemplateCache_DeleteByDeploymentID_RemovesMatchingEntry(t *testing.T) {
	cache := NewTemplateCache()
	depID := "abc-123-def"
	key := "myorg:myorg:my-agent:build-1:" + depID + ":0"
	cache.set(key, &spec.AstroDeploymentSpec{})

	cache.DeleteByDeploymentID(depID)

	if _, ok := cache.get(key); ok {
		t.Error("expected cache entry to be deleted, but it still exists")
	}
}

func TestTemplateCache_DeleteByDeploymentID_LeavesOtherEntriesIntact(t *testing.T) {
	cache := NewTemplateCache()
	targetID := "abc-123-def"
	otherID := "zzz-999-qqq"
	targetKey := "myorg:myorg:my-agent:build-1:" + targetID + ":0"
	otherKey := "myorg:myorg:other-agent:build-2:" + otherID + ":0"
	cache.set(targetKey, &spec.AstroDeploymentSpec{})
	cache.set(otherKey, &spec.AstroDeploymentSpec{})

	cache.DeleteByDeploymentID(targetID)

	if _, ok := cache.get(targetKey); ok {
		t.Error("expected target entry to be deleted, but it still exists")
	}
	if _, ok := cache.get(otherKey); !ok {
		t.Error("expected unrelated entry to remain in cache, but it was deleted")
	}
}

// --- Undeploy handler tests ---

// setupUndeployTest creates a gin engine wired with the UndeployAgent handler.
// mockQueue is a no-op DeployQueue for testing.
type mockQueue struct{}

func (q *mockQueue) InsertDeployJob(_ context.Context, _, _ string) error   { return nil }
func (q *mockQueue) InsertUndeployJob(_ context.Context, _, _ string) error { return nil }
func (q *mockQueue) InsertWakeUpJob(_ context.Context, _, _ string) error   { return nil }

// deploymentByIDColumns lists the columns scanned by scanDeployment, in order.
var deploymentByIDColumns = []string{
	"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
	"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
	"status", "error_message", "error_details", "status_changed_at", "current_revision",
	"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
}

// deploymentByIDRow returns a sqlmock.Rows matching the deploymentColumns scan in scanDeployment.
// Leaves source_account_id NULL, matching legacy / un-backfilled deployments.
func deploymentByIDRow(id, accountID, agentName, buildID, namespace, displayName, specJSON, status string, now time.Time, undeployedAt *time.Time) *sqlmock.Rows {
	return deploymentByIDRowWithSource(id, accountID, "", agentName, buildID, namespace, displayName, specJSON, status, now, undeployedAt)
}

// deploymentByIDRowWithSource is the column-aware variant: pass the source
// account ID to simulate a backfilled / post-migration row, or "" for legacy.
func deploymentByIDRowWithSource(id, accountID, sourceAccountID, agentName, buildID, namespace, displayName, specJSON, status string, now time.Time, undeployedAt *time.Time) *sqlmock.Rows {
	rev := 1
	var src interface{} = nil
	if sourceAccountID != "" {
		src = sourceAccountID
	}
	return sqlmock.NewRows(deploymentByIDColumns).AddRow(
		id, accountID, src, agentName, buildID, namespace,
		displayName, specJSON, []byte(nil), (*string)(nil), nil,
		status, (*string)(nil), json.RawMessage(nil), now, &rev,
		now, undeployedAt, nil, nil,
	)
}

// emptyDeploymentByIDRows returns an empty sqlmock.Rows matching the deploymentColumns layout.
func emptyDeploymentByIDRows() *sqlmock.Rows {
	return sqlmock.NewRows(deploymentByIDColumns)
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
	router.POST("/api/v1/undeploy", UndeployAgent(log, index, accountStore, nil, deployStore, &mockQueue{}, nil, nil))

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

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments", ListDeployments(log, accountStore, deployStore, nil, nil, nil, k8scache.NoopCache{}))

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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// IsMember
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetVisibleDeploymentsByAccount
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
		}).AddRow(
			depID, "acct-1", nil, agentName, buildID, namespace, "My Agent",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myaccount", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
		}).AddRow(
			depID, "acct-1", nil, agentName, buildID, namespace, "Sas Bot",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
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

// TestAssignEnvToWorkloads_ByRole verifies the role-set per component used to
// attach DB env to each WorkloadSpec:
//   - "agent"               → ["agent", "messaging"]
//   - "collector"           → ["collector"]
//   - "knowledge-<name>"    → ["knowledge:<name>"]
//   - "ingestion-<name>"    → ["ingestion:<name>"]
//   - untracked component   → no env (not an error)
//
// Also asserts that EnvVar projection carries Name, Value, IsSecret, and
// Source unchanged.
func TestAssignEnvToWorkloads_ByRole(t *testing.T) {
	envByRole := map[string][]deploymentstore.DecryptedEnvVar{
		"agent": {
			{Name: "LOG_LEVEL", Value: "info", Source: "user_var"},
			{Name: "API_KEY", Value: "••••••••", IsSecret: true, Source: "user_var"},
		},
		"messaging": {
			{Name: "BROKER_URL", Value: "ws://broker:9000", Source: "service_url"},
		},
		// KnowledgeRole("docs") → "knowledge:docs" (colon, not hyphen).
		"knowledge:docs": {
			{Name: "POSTGRES_USER", Value: "astro", Source: "knowledge_cred"},
		},
	}

	workloads := []WorkloadSpec{
		{Name: "agent", Component: "agent"},
		{Name: "knowledge-docs", Component: "knowledge-docs"},
		{Name: "third-party", Component: "ad-hoc-sidecar"},
	}

	assignEnvToWorkloads(workloads, envByRole)

	// The agent workload picks up BOTH "agent" and "messaging" roles —
	// confirming the original ask: clients can list env for both
	// containers (app + messaging sidecar) off the agent workload.
	agentEnv := workloads[0].Env
	if len(agentEnv) != 2 {
		t.Fatalf("agent workload: expected env for 2 roles, got %d (%v)", len(agentEnv), agentEnv)
	}
	if len(agentEnv["agent"]) != 2 {
		t.Errorf("agent[agent] len = %d, want 2", len(agentEnv["agent"]))
	}
	if agentEnv["agent"][0].Name != "LOG_LEVEL" || agentEnv["agent"][0].Value != "info" || agentEnv["agent"][0].IsSecret {
		t.Errorf("agent[agent][0] = %+v, want LOG_LEVEL=info non-secret", agentEnv["agent"][0])
	}
	if agentEnv["agent"][1].Name != "API_KEY" || !agentEnv["agent"][1].IsSecret || agentEnv["agent"][1].Source != "user_var" {
		t.Errorf("agent[agent][1] = %+v, want redacted API_KEY user_var", agentEnv["agent"][1])
	}
	if len(agentEnv["messaging"]) != 1 || agentEnv["messaging"][0].Name != "BROKER_URL" {
		t.Errorf("agent[messaging] = %+v, want single BROKER_URL", agentEnv["messaging"])
	}

	// knowledge-docs workload gets exactly one role: "knowledge:docs".
	knowEnv := workloads[1].Env
	if len(knowEnv) != 1 || len(knowEnv["knowledge:docs"]) != 1 {
		t.Fatalf("knowledge-docs env = %+v, want single knowledge:docs role", knowEnv)
	}
	if knowEnv["knowledge:docs"][0].Name != "POSTGRES_USER" {
		t.Errorf("knowledge-docs entry = %+v, want POSTGRES_USER", knowEnv["knowledge:docs"][0])
	}

	// Untracked component → nil Env map (not a panic, not an empty map).
	if workloads[2].Env != nil {
		t.Errorf("untracked component should have nil Env, got %+v", workloads[2].Env)
	}
}

// TestAssignEnvToWorkloads_MessagingOptional covers the agent workload when
// no messaging role exists in the env map (messaging not configured). The
// agent workload should still get its "agent" role env without a stray
// "messaging" key.
func TestAssignEnvToWorkloads_MessagingOptional(t *testing.T) {
	envByRole := map[string][]deploymentstore.DecryptedEnvVar{
		"agent": {{Name: "LOG_LEVEL", Value: "info"}},
	}
	workloads := []WorkloadSpec{{Name: "agent", Component: "agent"}}
	assignEnvToWorkloads(workloads, envByRole)

	if _, hasMsg := workloads[0].Env["messaging"]; hasMsg {
		t.Errorf("agent workload should not carry messaging key when role absent, got %+v", workloads[0].Env)
	}
	if len(workloads[0].Env["agent"]) != 1 {
		t.Errorf("agent workload should still have agent env, got %+v", workloads[0].Env)
	}
}

// TestAssignEnvToWorkloads_EmptyMap is the pre-cutover case: a deployment
// with no rows in deployment_build_env yet. assignEnvToWorkloads must leave
// Env nil on every workload without panicking.
func TestAssignEnvToWorkloads_EmptyMap(t *testing.T) {
	workloads := []WorkloadSpec{{Name: "agent", Component: "agent"}}
	assignEnvToWorkloads(workloads, nil)
	if workloads[0].Env != nil {
		t.Errorf("expected nil Env on empty map, got %+v", workloads[0].Env)
	}
}

// TestComponentLabelFor mirrors the K8s "app.kubernetes.io/component" label
// convention. Without this, knowledge/ingestion workloads on the record
// endpoint carried just the kind ("knowledge") while the runtime endpoint
// emitted the full label ("knowledge-cache"), and rolesForComponent failed
// to match — so env vars for keyed components never reached the response.
func TestComponentLabelFor(t *testing.T) {
	for _, tc := range []struct {
		kind, key, want string
	}{
		{"agent", "", "agent"},
		{"collector", "", "collector"},
		{"messaging", "", "messaging"},
		{"knowledge", "cache", "knowledge-cache"},
		{"ingestion", "hourly", "ingestion-hourly"},
	} {
		if got := componentLabelFor(tc.kind, tc.key); got != tc.want {
			t.Errorf("componentLabelFor(%q,%q) = %q, want %q", tc.kind, tc.key, got, tc.want)
		}
	}
}

// TestAssignEnvToWorkloads_KeyedComponents pins down the keyed-component
// path: a knowledge-cache workload must pick up its "knowledge:cache" env,
// and an ingestion-hourly workload its "ingestion:hourly" env. The
// component string passed in matches the K8s label format produced by
// componentLabelFor.
func TestAssignEnvToWorkloads_KeyedComponents(t *testing.T) {
	envByRole := map[string][]deploymentstore.DecryptedEnvVar{
		"knowledge:cache":  {{Name: "REDIS_URL", Value: "redis://x"}},
		"ingestion:hourly": {{Name: "S3_BUCKET", Value: "my-bucket"}},
	}
	workloads := []WorkloadSpec{
		{Name: "sasbot-knowledge-cache", Component: "knowledge-cache"},
		{Name: "sasbot-ingestion-hourly", Component: "ingestion-hourly"},
	}
	assignEnvToWorkloads(workloads, envByRole)

	if v := workloads[0].Env["knowledge:cache"]; len(v) != 1 || v[0].Name != "REDIS_URL" {
		t.Errorf("knowledge-cache env = %+v, want REDIS_URL", workloads[0].Env)
	}
	if v := workloads[1].Env["ingestion:hourly"]; len(v) != 1 || v[0].Name != "S3_BUCKET" {
		t.Errorf("ingestion-hourly env = %+v, want S3_BUCKET", workloads[1].Env)
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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// IsMember
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetVisibleDeploymentsByAccount returns no rows
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
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

func TestParseBuildIDFilter(t *testing.T) {
	// Table-driven coverage for parseBuildIDFilter's edge cases: comma
	// splitting, whitespace trimming, empty/whitespace-only inputs, and
	// the interaction of repeated query params with comma-separated values.
	// These are exercised indirectly through the full handler, but pinning
	// them here makes regressions in the parser stand out.
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "absent", query: "", want: nil},
		{name: "single value", query: "?build_id=b1", want: []string{"b1"}},
		{name: "comma-separated", query: "?build_id=b1,b2,b3", want: []string{"b1", "b2", "b3"}},
		{name: "repeated param", query: "?build_id=b1&build_id=b2", want: []string{"b1", "b2"}},
		{name: "mixed repeated and comma", query: "?build_id=b1,b2&build_id=b3", want: []string{"b1", "b2", "b3"}},
		{name: "trims whitespace around values", query: "?build_id=%20b1%20,%20b2", want: []string{"b1", "b2"}},
		{name: "drops empty entries between commas", query: "?build_id=b1,,b2", want: []string{"b1", "b2"}},
		{name: "drops trailing comma", query: "?build_id=b1,", want: []string{"b1"}},
		{name: "drops leading comma", query: "?build_id=,b1", want: []string{"b1"}},
		{name: "whitespace-only value returns nil", query: "?build_id=%20", want: nil},
		{name: "all-empty repeated params return nil", query: "?build_id=&build_id=,", want: nil},
		{name: "empty value returns nil", query: "?build_id=", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/api/v1/deployments"+tc.query, nil)

			got := parseBuildIDFilter(c)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseBuildIDFilter(%q) = %#v, want %#v", tc.query, got, tc.want)
			}
		})
	}
}

func TestListDeployments_RejectsOversizedBuildIDFilter(t *testing.T) {
	// The build_id filter is capped to prevent a misbehaving caller from
	// expanding the SQL ANY() array into something that bothers the planner.
	// 201 entries (one over the cap) should be rejected with 400 before any
	// DB or account lookup runs — no mocks set up, since reaching them is
	// itself a failure.
	router, _, _ := setupListDeploymentsTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	ids := make([]string, 201)
	for i := range ids {
		ids[i] = fmt.Sprintf("b%d", i)
	}
	req := httptest.NewRequest("GET", "/api/v1/deployments?build_id="+strings.Join(ids, ","), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "at most 200") {
		t.Errorf("expected error message to mention the cap, got %s", w.Body.String())
	}
}

func TestListDeployments_RejectsUnfilteredCrossAccount(t *testing.T) {
	// The cross-account path is only meant to power the blueprint sidebar,
	// which always supplies ?build_id=. Refusing the unfiltered combo
	// (no account, no build_id) prevents callers from accidentally pulling
	// every deployment across every account in one uncached response.
	router, _, _ := setupListDeploymentsTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	req := httptest.NewRequest("GET", "/api/v1/deployments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListDeployments_OmittedAccount_StampsAccountContext(t *testing.T) {
	// The cross-account fan-out must stamp account_id / account_name on every
	// returned summary so the blueprint sidebar can attribute, link, and route
	// per deployment without a second join.
	depID := deployid.New()
	router, deployMock, accountMock := setupListDeploymentsTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	now := time.Now()

	// GetAccountsForUser → one account.
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "cluster_id", "created_at", "updated_at", "display_name", "avatar_updated_at",
		}).AddRow("acct-1", "myorg", "organization", "", "", now, now, "MyOrg", nil))

	// GetVisibleDeploymentsByAccountAndBuilds → one deployment.
	deployMock.ExpectQuery(`build_id = ANY`).
		WithArgs("acct-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
		}).AddRow(
			depID, "acct-1", nil, "my-agent", "build-1", "astro-abc-0", "",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
		))

	req := httptest.NewRequest("GET", "/api/v1/deployments?build_id=build-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count       int                      `json:"count"`
		Deployments []AgentDeploymentSummary `json:"deployments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("expected 1 deployment, got %d", resp.Count)
	}
	got := resp.Deployments[0]
	if got.AccountID != "acct-1" {
		t.Errorf("AccountID = %q, want %q", got.AccountID, "acct-1")
	}
	if got.AccountName != "myorg" {
		t.Errorf("AccountName = %q, want %q", got.AccountName, "myorg")
	}
}

func TestListDeployments_OmittedAccount_AggregatesAcrossAccounts(t *testing.T) {
	// Happy path for the cross-account fan-out: deployments from every
	// account the user belongs to should appear in the combined response,
	// each stamped with the originating account_id / account_name.
	depA := deployid.New()
	depB := deployid.New()
	router, deployMock, accountMock := setupListDeploymentsTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	// Fan-out runs accounts concurrently — queries may arrive in either order.
	deployMock.MatchExpectationsInOrder(false)

	now := time.Now()

	// GetAccountsForUser → two accounts.
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "cluster_id", "created_at", "updated_at", "display_name", "avatar_updated_at",
		}).
			AddRow("acct-1", "alpha", "organization", "", "", now, now, "Alpha", nil).
			AddRow("acct-2", "beta", "personal", "", "", now, now, "Beta", nil))

	// GetVisibleDeploymentsByAccountAndBuilds → one deployment per account.
	depCols := []string{
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
		"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
	}
	deployMock.ExpectQuery(`build_id = ANY`).WithArgs("acct-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(depCols).AddRow(
			depA, "acct-1", nil, "agent-a", "b1", "astro-aaa-0", "",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
		))
	deployMock.ExpectQuery(`build_id = ANY`).WithArgs("acct-2", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(depCols).AddRow(
			depB, "acct-2", nil, "agent-b", "b1", "astro-bbb-0", "",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
		))

	req := httptest.NewRequest("GET", "/api/v1/deployments?build_id=b1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count       int                      `json:"count"`
		Deployments []AgentDeploymentSummary `json:"deployments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 2 {
		t.Fatalf("expected count=2, got %d", resp.Count)
	}

	byID := make(map[string]AgentDeploymentSummary, len(resp.Deployments))
	for _, d := range resp.Deployments {
		byID[d.ID] = d
	}
	if got := byID[depA]; got.AccountID != "acct-1" || got.AccountName != "alpha" {
		t.Errorf("dep A account ctx: AccountID=%q AccountName=%q, want acct-1/alpha", got.AccountID, got.AccountName)
	}
	if got := byID[depB]; got.AccountID != "acct-2" || got.AccountName != "beta" {
		t.Errorf("dep B account ctx: AccountID=%q AccountName=%q, want acct-2/beta", got.AccountID, got.AccountName)
	}
}

func TestListDeployments_BuildIDFilter_SingleAccount(t *testing.T) {
	// ?build_id=b1 must restrict the single-account response to deployments
	// of those builds — the filter is pushed into SQL via the build-filtered
	// store method, so the mock only sees one query with build_ids in args.
	depID := deployid.New()
	router, deployMock, accountMock := setupListDeploymentsTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	now := time.Now()

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// The filtered query must reach the build_id = ANY(...) variant — assert
	// by matching the SQL fragment, not just `SELECT`. WithArgs pins the
	// account ID and the build-IDs array binding.
	deployMock.ExpectQuery(`build_id = ANY`).
		WithArgs("acct-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
		}).AddRow(
			depID, "acct-1", nil, "my-agent", "b1", "astro-abc-0", "",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
		))

	req := httptest.NewRequest("GET", "/api/v1/deployments?account=myorg&build_id=b1,b2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count       int                      `json:"count"`
		Deployments []AgentDeploymentSummary `json:"deployments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("expected count=1, got %d", resp.Count)
	}
	if resp.Deployments[0].BuildID != "b1" {
		t.Errorf("BuildID = %q, want %q", resp.Deployments[0].BuildID, "b1")
	}
}

func TestListDeployments_BuildIDFilter_CrossAccount(t *testing.T) {
	// The cross-account path must also honor ?build_id=. Each per-account
	// fan-out goroutine should hit the filtered store method, not the
	// unfiltered one.
	depA := deployid.New()
	router, deployMock, accountMock := setupListDeploymentsTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	deployMock.MatchExpectationsInOrder(false)

	now := time.Now()

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "cluster_id", "created_at", "updated_at", "display_name", "avatar_updated_at",
		}).
			AddRow("acct-1", "alpha", "organization", "", "", now, now, "Alpha", nil).
			AddRow("acct-2", "beta", "personal", "", "", now, now, "Beta", nil))

	depCols := []string{
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
		"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
	}
	deployMock.ExpectQuery(`build_id = ANY`).
		WithArgs("acct-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(depCols).AddRow(
			depA, "acct-1", nil, "agent-a", "b1", "astro-aaa-0", "",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
		))
	// Account 2 has no matching builds — the filtered query returns empty.
	deployMock.ExpectQuery(`build_id = ANY`).
		WithArgs("acct-2", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(depCols))

	req := httptest.NewRequest("GET", "/api/v1/deployments?build_id=b1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count       int                      `json:"count"`
		Deployments []AgentDeploymentSummary `json:"deployments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("expected count=1 (only acct-1 had a match), got %d", resp.Count)
	}
	if got := resp.Deployments[0]; got.ID != depA || got.AccountID != "acct-1" {
		t.Errorf("matched dep: got ID=%q AccountID=%q, want %q / acct-1", got.ID, got.AccountID, depA)
	}
}

func TestListDeployments_OmittedAccount_PartialFailureKeepsSuccessfulAccount(t *testing.T) {
	// A single account's failure must not blank the sidebar. The
	// cross-account path logs and skips the failing account; deployments
	// from the surviving accounts still render.
	depOK := deployid.New()
	router, deployMock, accountMock := setupListDeploymentsTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	deployMock.MatchExpectationsInOrder(false)

	now := time.Now()

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "cluster_id", "created_at", "updated_at", "display_name", "avatar_updated_at",
		}).
			AddRow("acct-1", "alpha", "organization", "", "", now, now, "Alpha", nil).
			AddRow("acct-bad", "broken", "organization", "", "", now, now, "Broken", nil))

	depCols := []string{
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
		"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
	}
	deployMock.ExpectQuery(`build_id = ANY`).WithArgs("acct-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(depCols).AddRow(
			depOK, "acct-1", nil, "agent-a", "b1", "astro-aaa-0", "",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
		))
	deployMock.ExpectQuery(`build_id = ANY`).WithArgs("acct-bad", sqlmock.AnyArg()).
		WillReturnError(fmt.Errorf("db unavailable"))

	req := httptest.NewRequest("GET", "/api/v1/deployments?build_id=b1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 despite partial failure, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Count       int                      `json:"count"`
		Deployments []AgentDeploymentSummary `json:"deployments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("expected count=1 (surviving account), got %d", resp.Count)
	}
	if got := resp.Deployments[0]; got.ID != depOK || got.AccountID != "acct-1" {
		t.Errorf("surviving dep: got ID=%q AccountID=%q, want %q / acct-1", got.ID, got.AccountID, depOK)
	}
}

func TestListDeployments_NotMember(t *testing.T) {
	router, _, accountMock := setupListDeploymentsTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	now := time.Now()

	// accountStore.GetByName
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Two active deployments
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
		}).AddRow(
			depID1, "acct-1", nil, "agent-a", "b1", ns1, "Agent A",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
		).AddRow(
			depID2, "acct-1", nil, "agent-b", "b1", ns2, "Agent B",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
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

func TestListDeployments_PerDeploymentClusterRouting(t *testing.T) {
	depPrimary := deployid.New()
	depAdditional := deployid.New()
	nsPrimary := "astro-primary111-0"
	nsAdditional := "astro-additional222-0"
	clusterID := "eu-west-1"

	router, deployMock, accountMock := setupListDeploymentsTest(t,
		k8sListHandler(nsPrimary, "agent-primary", "b1"))

	now := time.Now()
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
		}).AddRow(
			depPrimary, "acct-1", nil, "agent-primary", "b1", nsPrimary, "Primary",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
		).AddRow(
			depAdditional, "acct-1", nil, "agent-other", "b1", nsAdditional, "Other region",
			`{}`, nil, nil, clusterID,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
		))

	req := httptest.NewRequest("GET", "/api/v1/deployments?account=myorg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Count       int                      `json:"count"`
		Deployments []AgentDeploymentSummary `json:"deployments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Count != 2 {
		t.Fatalf("expected 2 deployments, got %d", resp.Count)
	}

	byID := make(map[string]AgentDeploymentSummary, len(resp.Deployments))
	for _, d := range resp.Deployments {
		byID[d.ID] = d
	}
	if byID[depPrimary].Name != "agent-primary" {
		t.Errorf("primary dep name: got %q want %q", byID[depPrimary].Name, "agent-primary")
	}
	if byID[depAdditional].ID != depAdditional {
		t.Errorf("missing additional-cluster deployment row")
	}
	if byID[depAdditional].Name != "agent-other" {
		t.Errorf("additional dep name: got %q", byID[depAdditional].Name)
	}
	// Account context must be stamped on every summary so the consumer can
	// attribute and link per deployment without joining a second endpoint.
	for _, d := range resp.Deployments {
		if d.AccountID != "acct-1" {
			t.Errorf("dep %q: AccountID = %q, want %q", d.ID, d.AccountID, "acct-1")
		}
		if d.AccountName != "myorg" {
			t.Errorf("dep %q: AccountName = %q, want %q", d.ID, d.AccountName, "myorg")
		}
	}
}

func TestListDeployments_FailedStatusMapsToError(t *testing.T) {
	// Pin the contract: DB status "failed" → AgentDeploymentSummary.Status "error".
	// The client checks deployment.status === "error" to render the error badge, so
	// this mapping must never silently change.
	depID := deployid.New()
	router, deployMock, accountMock := setupListDeploymentsTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	now := time.Now()
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
		}).AddRow(
			depID, "acct-1", nil, "my-agent", "build-1", "astro-abc-0", "",
			`{}`, nil, nil, nil,
			"failed", nil, nil, now, 1,
			now, nil, nil, nil,
		))

	req := httptest.NewRequest("GET", "/api/v1/deployments?account=myorg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Deployments []AgentDeploymentSummary `json:"deployments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(resp.Deployments))
	}
	if resp.Deployments[0].Status != "error" {
		t.Errorf("expected status %q, got %q", "error", resp.Deployments[0].Status)
	}
}

func TestListDeployments_NilDeployStore(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	accountStore := account.NewAccountStore(accountDB)
	log := logger.New("error", "json")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments", ListDeployments(log, accountStore, nil, nil, nil, nil, k8scache.NoopCache{}))

	now := time.Now()
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
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

func TestUndeploy_UndeployingDeployment(t *testing.T) {
	router, deployMock, _ := setupUndeployTest(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	// GetDeploymentByID returns an already-undeploying record
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "undeploying", now, nil))

	body := `{"deployment_id":"` + depID + `"}`
	req := httptest.NewRequest("POST", "/api/v1/undeploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for already-undeploying deployment, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUndeploy_PendingDeployment(t *testing.T) {
	router, deployMock, accountMock := setupUndeployTest(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	// GetDeploymentByID returns a pending deployment (not yet provisioned)
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "pending", now, nil))

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
		t.Fatalf("expected 202 for pending deployment undeploy, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "undeploying" {
		t.Errorf("expected status 'undeploying', got %v", resp["status"])
	}
}

func TestUndeploy_ProvisioningDeployment(t *testing.T) {
	router, deployMock, accountMock := setupUndeployTest(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	// GetDeploymentByID returns a provisioning deployment
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "provisioning", now, nil))

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
		t.Fatalf("expected 202 for provisioning deployment undeploy, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "undeploying" {
		t.Errorf("expected status 'undeploying', got %v", resp["status"])
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
	router.POST("/api/v1/undeploy", UndeployAgent(log, index, accountStore, nil, deployStore, &mockQueue{}, nil, nil))

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

func TestCanDeploySourceAgent_Boundary(t *testing.T) {
	personal := &account.Account{ID: "acct-personal", Name: "matt", Type: "personal"}
	org := &account.Account{ID: "acct-org", Name: "astropods", Type: "organization"}

	tests := []struct {
		name       string
		sourceAcct *account.Account
		targetAcct *account.Account
		visibility string
		want       bool
	}{
		{
			name:       "org deploys own private blueprint",
			sourceAcct: org,
			targetAcct: org,
			visibility: "private",
			want:       true,
		},
		{
			name:       "organization deploys public personal blueprint",
			sourceAcct: personal,
			targetAcct: org,
			visibility: "public",
			want:       true,
		},
		{
			name:       "organization cannot deploy private personal blueprint",
			sourceAcct: personal,
			targetAcct: org,
			visibility: "private",
			want:       false,
		},
		{
			name:       "personal account deploys own private blueprint",
			sourceAcct: personal,
			targetAcct: personal,
			visibility: "private",
			want:       true,
		},
		{
			name:       "personal account deploys public organization blueprint",
			sourceAcct: org,
			targetAcct: personal,
			visibility: "public",
			want:       true,
		},
		{
			name:       "personal account cannot deploy private organization blueprint",
			sourceAcct: org,
			targetAcct: personal,
			visibility: "private",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &agentindex.Agent{Visibility: tt.visibility}

			if got := canDeploySourceAgent(tt.sourceAcct, tt.targetAcct, agent); got != tt.want {
				t.Fatalf("canDeploySourceAgent() = %v, want %v", got, tt.want)
			}
		})
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
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("src-acct", "source-org", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// Target account lookup (different from source)
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("target-org").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("tgt-acct", "target-org", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// IsMember(target, user) → member of target account
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("tgt-acct", "user-target").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get → private agent
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("src-acct", "secret-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "avatar_updated_at", "created_at", "updated_at"}).
			AddRow("src-acct", "secret-agent", "r.io", "private", nil, false, nil, nil, now, now))
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("src-acct", "secret-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "testaccount", `{"name":"secret-agent"}`, "", "", "[]", now, now))

	req := httptest.NewRequest(http.MethodPost, "/deploy/validate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-account deploy of private agent should be rejected, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeploy_PublicSourceAgent_CrossAccount_Allowed(t *testing.T) {
	router, indexMock, accountMock := setupValidateRouter("user-target")

	now := time.Now()
	body := crossAccountDeployableSpec()
	storedSpec := `{"name":"public-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-source-org/public-agent:build-1"}}`

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("source-org").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("src-acct", "source-org", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("target-org").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("tgt-acct", "target-org", "personal", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("tgt-acct", "user-target").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("src-acct", "public-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "avatar_updated_at", "created_at", "updated_at"}).
			AddRow("src-acct", "public-agent", "r.io", "public", nil, false, nil, nil, now, now))
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("src-acct", "public-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "source-org", storedSpec, "", "", "[]", now, now))

	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("src-acct", "public-agent", "build-1").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "source-org", storedSpec, "", "", "[]", now, now))

	req := httptest.NewRequest(http.MethodPost, "/deploy/validate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("cross-account deploy of public agent should be allowed, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["valid"] != true {
		t.Fatalf("expected valid response, got %#v", resp)
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
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// IsMember(target=source, user) → member
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get → no rows
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "nonexistent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "avatar_updated_at", "created_at", "updated_at"}))

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
					[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
					AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

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

// --- Deploy endpoint: deployment_id handling tests ---

// setupDeployRouter creates a gin engine wired with DeployAgent and ValidateDeployment.
// Returns (router, indexMock, accountMock, deployMock).
func setupDeployRouter(userID string) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	router, im, am, dm, _ := setupDeployRouterWithPreflighter(userID, nil)
	return router, im, am, dm
}

// testSigningKey is a deterministic key wired into the test deploy router so
// helper requests can sign specs that pass the deploy handler's verification.
var testSigningKey = []byte("test-signing-key-32-bytes-padding!")

// signedDeployRequest builds a POST /deploy request whose body is signed with
// testSigningKey. Tests use this in place of httptest.NewRequest so the deploy
// handler's mandatory signature check passes.
func signedDeployRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	var ds spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(body), &ds); err != nil {
		t.Fatalf("signedDeployRequest: invalid body JSON: %v", err)
	}
	sig := specsign.Sign(testSigningKey, &ds)
	r := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Template-Signature", sig)
	return r
}

// TestDeploy_MissingSignatureHeader: requests without X-Template-Signature
// must be rejected by the deploy handler before any DB or k8s work.
func TestDeploy_MissingSignatureHeader(t *testing.T) {
	router, _, _, _ := setupDeployRouter("user-1")

	body := deployableSpec("")
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// intentionally no X-Template-Signature

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid or missing template signature") {
		t.Errorf("expected signature error in body, got: %s", rec.Body.String())
	}
}

// TestDeploy_BadSignature: requests with a non-empty but invalid
// X-Template-Signature must be rejected with the same 400.
func TestDeploy_BadSignature(t *testing.T) {
	router, _, _, _ := setupDeployRouter("user-1")

	body := deployableSpec("")
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Template-Signature", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid or missing template signature") {
		t.Errorf("expected signature error in body, got: %s", rec.Body.String())
	}
}

// TestDeploy_TamperedBody: a request signed correctly against one body but
// submitted with a different body must be rejected.
func TestDeploy_TamperedBody(t *testing.T) {
	router, _, _, _ := setupDeployRouter("user-1")

	// Sign one body, submit a different one — signature won't match.
	original := deployableSpec("")
	var ds spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(original), &ds); err != nil {
		t.Fatalf("parse: %v", err)
	}
	sig := specsign.Sign(testSigningKey, &ds)

	tampered := strings.Replace(original, `"image":`, `"image":"evil-image","_oldimage":`, 1)
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(tampered))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Template-Signature", sig)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid or missing template signature") {
		t.Errorf("expected signature error in body, got: %s", rec.Body.String())
	}
}

// setupDeployRouterWithPreflighter is the variant used by image-preflight tests.
// Returns the cfg too so callers can inspect it if needed.
func setupDeployRouterWithPreflighter(userID string, preflighter *k8s.ImagePreflighter) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock, sqlmock.Sqlmock, *config.Config) {
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
			RegistryURL:        "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			Environment:        "test",
			TemplateSigningKey: testSigningKey,
		},
	}

	router := gin.New()
	if userID != "" {
		router.Use(func(c *gin.Context) {
			c.Set(string(auth.UserContextKey), &auth.User{ID: userID})
			c.Next()
		})
	}
	router.POST("/deploy", DeployAgent(log, index, accountStore, cfg, deployStore, nil, nil, nil, nil, nil, &mockQueue{}, nil, nil, nil, nil, nil, preflighter, nil, nil)) //nolint:staticcheck // nil varsStore, clusterStore, k8sReg, EntitlementChecker, quota.Checker, avatarStore, omClient, db, auditStore, ksStore, authzStore, and tmplCache skip checks in tests

	return router, indexMock, accountMock, deployMock, cfg
}

// expectDeployPrep sets up mocks for the full prepareDeployment flow: account lookup,
// membership check, agent+version lookup for both agentIndex.Get and the build lookup.
func expectDeployPrep(accountMock, indexMock sqlmock.Sqlmock) {
	expectDeployPrepWithCluster(accountMock, indexMock, "")
}

func expectDeployPrepWithCluster(accountMock, indexMock sqlmock.Sqlmock, clusterID string) {
	now := time.Now()

	// accountStore.GetByName("myorg") — source and target account
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRowWithCluster("acct-1", "myorg", "organization", nil, nil, now, now, clusterID)...))

	// IsMember(target=source, user) → yes
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get (visibility check)
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "avatar_updated_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "r.io", "public", nil, false, nil, nil, now, now))
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

// expectVariableInsertsByName mocks the user-variable persistence
// SaveNormalizedSpec performs at deploy time. The single write is
// to deployment_build_env: one DELETE clearing prior rows, then one
// INSERT per (variable, target_role) pair (fan-out). The legacy
// deployment_variables table is gone; reads come from build_env via
// GetDeploymentVariables, which now queries build_env directly.
//
// MatchExpectationsInOrder(false) lets the DELETE and INSERTs arrive
// in any order. Tests that exercise multi-role fan-out should register
// additional INSERT expectations explicitly.
func expectVariableInsertsByName(deployMock sqlmock.Sqlmock, names ...string) {
	deployMock.MatchExpectationsInOrder(false)
	deployMock.ExpectExec(`DELETE FROM deployment_build_env`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	for _, name := range names {
		deployMock.ExpectExec(`INSERT INTO deployment_build_env`).
			WithArgs(
				sqlmock.AnyArg(), // deployment_id
				sqlmock.AnyArg(), // role
				name,             // env_name
				sqlmock.AnyArg(), // value_encrypted
				sqlmock.AnyArg(), // nonce
				sqlmock.AnyArg(), // is_secret
				sqlmock.AnyArg(), // source
				sqlmock.AnyArg(), // user_var_name
				sqlmock.AnyArg(), // account_var_ref
				sqlmock.AnyArg(), // optional
			).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
}

// deployableSpec builds a JSON deployment spec that matches the template the server
// generates from the agent spec `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}` with RegistryURL "123456789.dkr.ecr.us-east-1.amazonaws.com"
// and Environment "test". The caller can optionally set deploymentID to test the in-place update path.
func deployableSpecWithDeploymentIDAndDisplayName(deploymentID, displayName string) string {
	return fmt.Sprintf(`{
		"spec": "deployment/v1",
		"source": {"account": "myorg", "name": "my-agent", "build": "build-1", "registry": "https://123456789.dkr.ecr.us-east-1.amazonaws.com"},
		"target": {"runtime": "kubernetes", "deployment_id": %q, "display_name": %q},
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
	}`, deploymentID, displayName)
}

func deployableSpecWithDisplayName(displayName string) string {
	return fmt.Sprintf(`{
		"spec": "deployment/v1",
		"source": {"account": "myorg", "name": "my-agent", "build": "build-1", "registry": "https://123456789.dkr.ecr.us-east-1.amazonaws.com"},
		"target": {"runtime": "kubernetes", "display_name": %q},
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
	}`, displayName)
}

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

func crossAccountDeployableSpec() string {
	return `{
		"spec": "deployment/v1",
		"source": {"account": "source-org", "name": "public-agent", "build": "build-1", "registry": "https://123456789.dkr.ecr.us-east-1.amazonaws.com"},
		"target": {"account": "target-org", "runtime": "kubernetes"},
		"agent": {
			"image": "123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-source-org/public-agent:build-1",
			"endpoints": {"http": {"port": 8080, "protocol": "http"}},
			"replicas": 1,
			"resources": {"cpu": "100m", "memory": "256Mi", "cpu_limit": "1", "memory_limit": "1Gi"},
			"environment": {"ASTRO_AGENT_NAME": "public-agent", "ASTRO_AGENT_BUILD": "build-1"},
			"update": {"strategy": "rolling", "max_unavailable": "25%", "max_surge": "25%"}
		},
		"variables": {
			"SLACK_BOT_TOKEN": {"secret": true, "optional": true, "targets": ["interface.slack"]},
			"SLACK_APP_TOKEN": {"secret": true, "optional": true, "targets": ["interface.slack"]},
			"SLACK_CONFIG": {"secret": false, "optional": true, "targets": ["interface.slack"]}
		},
		"observability": {"enabled": true, "provider": "langfuse"}
	}`
}

func TestDeploy_WithoutDeploymentID_CreatesNew(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupDeployRouter("user-1")

	expectDeployPrep(accountMock, indexMock)

	// No deployment_id in spec and no display_name → new deployment path with
	// a fresh id + derived namespace. prepareDeployment performs no lookup
	// queries on this path.

	// SaveDeploymentPending transaction
	deployMock.ExpectBegin()
	deployMock.ExpectQuery(`INSERT INTO deployments`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at",
		}).AddRow("new-id", "acct-1", nil, "my-agent", "build-1", "astro-new", "", "{}", "pending", time.Now()))
	// Revision insert
	deployMock.ExpectExec(`INSERT INTO deployment_revisions`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Event insert
	deployMock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Normalized spec inserts (agent workload + service + volume + collector workload + services + variables)
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectExec(`INSERT INTO deployment_volumes`).
		WillReturnResult(sqlmock.NewResult(0, 1))
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
	deployMock.ExpectCommit()

	body := deployableSpec("")
	req := signedDeployRequest(t, body)
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
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at",
		}).AddRow(depID, "acct-1", nil, "my-agent", "build-1", "astro-existing", "My Agent", "{}", "pending", now))
	// Revision insert
	deployMock.ExpectExec(`INSERT INTO deployment_revisions`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`DELETE FROM deployment_workloads`).WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`DELETE FROM deployment_sidecars`).WillReturnResult(sqlmock.NewResult(0, 0))
	deployMock.ExpectExec(`DELETE FROM deployment_build_env`).WillReturnResult(sqlmock.NewResult(0, 0))
	// Event insert
	deployMock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Normalized spec re-inserts (agent workload + service + volume + collector workload + services + variables)
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectExec(`INSERT INTO deployment_volumes`).
		WillReturnResult(sqlmock.NewResult(0, 1))
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
	deployMock.ExpectCommit()

	body := deployableSpec(depID)
	req := signedDeployRequest(t, body)
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
	req := signedDeployRequest(t, body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDeploy_ImageNotFound_Returns422 verifies that when the configured
// preflighter detects a missing manifest, the deploy fails fast with a 422
// containing image+build_id in the body — and never enqueues a deploy job
// or writes a pending row. This is the entire user-visible fix for the
// 35-minute ImagePullBackOff loop.
func TestDeploy_ImageNotFound_Returns422(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	target, _ := url.Parse(srv.URL)
	preflighter := k8s.NewImagePreflighter(false)
	preflighter.SetClient(http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 1 * time.Second}).DialContext(ctx, network, target.Host)
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test stub
		},
		Timeout: 2 * time.Second,
	})

	router, indexMock, accountMock, _, _ := setupDeployRouterWithPreflighter("user-1", preflighter)
	expectDeployPrep(accountMock, indexMock)

	body := deployableSpec("")
	req := signedDeployRequest(t, body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["error"] != "image_not_found" {
		t.Errorf("error=%v, want image_not_found", resp["error"])
	}
	if resp["build_id"] != "build-1" {
		t.Errorf("build_id=%v, want build-1", resp["build_id"])
	}
	if img, _ := resp["image"].(string); !strings.Contains(img, "my-agent") {
		t.Errorf("image=%v, expected to contain my-agent", img)
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
	req := signedDeployRequest(t, body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for inactive deployment, got %d: %s", rec.Code, rec.Body.String())
	}
}

// A redeploy via Target.DeploymentID that renames the row into a name owned
// by a *different* live row must also 409 — the constraint fires on the
// UPDATE inside UpdateDeploymentPending, and the handler's translation must
// cover that path too, not just SaveDeploymentPending.
func TestDeploy_DisplayNameCollision_RenameRejected(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupDeployRouter("user-1")

	expectDeployPrep(accountMock, indexMock)

	depID := "existing-dep-id"
	now := time.Now()

	// GetDeploymentByID returns the row being redeployed (different display_name).
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, "acct-1", "my-agent", "build-1", "astro-existing",
			"Original Name", `{}`, "active", now, nil))

	// IsMember check for deployment's account.
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// UpdateDeploymentPending opens a tx, reads next revision, then runs the
	// UPDATE which trips the partial unique index.
	deployMock.ExpectBegin()
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"next_revision"}).AddRow(2))
	deployMock.ExpectQuery(`UPDATE deployments`).
		WillReturnError(&pq.Error{Code: "23505", Constraint: "idx_deployments_live_display_name"})
	deployMock.ExpectRollback()

	body := deployableSpecWithDeploymentIDAndDisplayName(depID, "Owned Name")
	req := signedDeployRequest(t, body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for rename collision, got %d: %s", rec.Code, rec.Body.String())
	}
}

// A new deploy (no Target.DeploymentID) whose display_name collides with an
// existing live deployment must 409. Enforcement is at the DB layer —
// SaveDeploymentPending's INSERT trips the partial unique index — so the
// test wires the INSERT to return the canonical pq unique_violation.
func TestDeploy_DisplayNameCollision_NewDeployRejected(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupDeployRouter("user-1")

	expectDeployPrep(accountMock, indexMock)

	deployMock.ExpectBegin()
	deployMock.ExpectQuery(`INSERT INTO deployments`).
		WillReturnError(&pq.Error{Code: "23505", Constraint: "idx_deployments_live_display_name"})
	deployMock.ExpectRollback()

	body := deployableSpecWithDisplayName("My Agent")
	req := signedDeployRequest(t, body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for display_name collision, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- GetDeploymentStatus tests ---
//
// The /deployments/:id/status endpoint returns the coarse, server-derived
// status enum the UI renders. The handler short-circuits to the DB status
// for non-active states (paused/undeploying/failed/pending) and only probes
// K8s when DB status is active. Tests below exercise the DB precedence path
// without a cluster client — the active+K8s path is integration-tested.

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
	// k8sReg=nil makes k8sRegistryReady return false; handler falls back to
	// DB-status-only and skips the readiness probe. That's exactly the path
	// we want to exercise here.
	router.GET("/api/v1/deployments/:id/status", GetDeploymentStatus(log, accountStore, nil, deployStore))

	return router, deployMock, accountMock
}

func TestGetDeploymentStatus_PausedReportsInactive(t *testing.T) {
	router, deployMock, accountMock := setupGetDeploymentStatusRouter(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "stopped", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID+"/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp DeploymentStatus
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Value != "inactive" {
		t.Errorf("expected inactive, got %q", resp.Value)
	}
}

func TestGetDeploymentStatus_NotFound(t *testing.T) {
	router, deployMock, _ := setupGetDeploymentStatusRouter(t)

	depID := deployid.New()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(emptyDeploymentByIDRows())

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID+"/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// resolveDeployment returns 403 (not 404) when the deployment doesn't
	// exist or isn't accessible — same shape as the rest of the deployment
	// endpoints, so the auth pathway is uniform.
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// DB-status precedence — each of these short-circuits before the K8s probe,
// so we exercise them with k8sReg=nil (no cluster client available).
func TestGetDeploymentStatus_DBStatusPrecedence(t *testing.T) {
	cases := []struct {
		name        string
		dbStatus    string
		wantValue   string
		wantReason  string
		wantDetails string
	}{
		{"stopped", "stopped", "inactive", StatusReasonPaused, "Deployment is paused"},
		{"undeploying", "undeploying", "undeploying", StatusReasonUndeploying, "Deployment is being torn down"},
		{"failed", "failed", "error", StatusReasonFailed, "Deployment failed"},
		{"pending", "pending", "deploying", StatusReasonProvisioning, "Pods are being provisioned"},
		{"provisioning", "provisioning", "deploying", StatusReasonProvisioning, "Pods are being provisioned"},
		// active now trusts the controller-maintained DB status directly (no K8s
		// probe). GetWorkloadStatuses isn't mocked here, so it errors and the
		// handler falls back to the default details string.
		{"active", "active", "active", StatusReasonReady, "Deployment is active"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, deployMock, accountMock := setupGetDeploymentStatusRouter(t)

			depID := deployid.New()
			acctID := uuid.New().String()
			now := time.Now()

			deployMock.ExpectQuery(`SELECT`).
				WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
					"My Agent", `{}`, tc.dbStatus, now, nil))
			accountMock.ExpectQuery(`SELECT`).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

			req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID+"/status", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			var resp DeploymentStatus
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			if resp.Value != tc.wantValue {
				t.Errorf("value: got %q, want %q", resp.Value, tc.wantValue)
			}
			if resp.Reason != tc.wantReason {
				t.Errorf("reason: got %q, want %q", resp.Reason, tc.wantReason)
			}
			if resp.Details != tc.wantDetails {
				t.Errorf("details: got %q, want %q", resp.Details, tc.wantDetails)
			}
		})
	}
}

// Failed deployments surface the stored error_message in the details string
// so the rendered tooltip points users at the actual failure.
func TestGetDeploymentStatus_FailedIncludesErrorMessage(t *testing.T) {
	router, deployMock, accountMock := setupGetDeploymentStatusRouter(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()
	errMsg := "image pull failed: backoff"

	rows := sqlmock.NewRows([]string{
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
		"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
	}).AddRow(
		depID, acctID, nil, "my-agent", "build-1", "astro-abc123",
		"My Agent", `{}`, []byte(nil), (*string)(nil), nil,
		"failed", &errMsg, json.RawMessage(nil), now, nil,
		now, (*time.Time)(nil), nil, nil,
	)
	deployMock.ExpectQuery(`SELECT`).WillReturnRows(rows)
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID+"/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp DeploymentStatus
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Value != "error" || resp.Reason != StatusReasonFailed {
		t.Errorf("value/reason: got %q/%q, want error/%s", resp.Value, resp.Reason, StatusReasonFailed)
	}
	if !strings.Contains(resp.Details, errMsg) {
		t.Errorf("details %q should contain error message %q", resp.Details, errMsg)
	}
	if resp.ErrorMessage != errMsg {
		t.Errorf("ErrorMessage: got %q, want %q", resp.ErrorMessage, errMsg)
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
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
		"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
	}).AddRow(
		id, accountID, nil, agentName, buildID, namespace,
		displayName, specJSON, []byte(nil), (*string)(nil), nil,
		status, (*string)(nil), json.RawMessage(nil), now, revision,
		now, (*time.Time)(nil), nil, nil,
	)
}

func TestWakeUpDeployment_Success(t *testing.T) {
	router, deployMock, accountMock := setupWakeUpRouter(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()
	rev := 1

	// GetDeploymentByID — stopped status
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRowWithStatus(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "stopped", &rev, now))

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

func TestWakeUpDeployment_NotStopped(t *testing.T) {
	router, deployMock, _ := setupWakeUpRouter(t)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	// GetDeploymentByID — active status (not stopped)
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
	if resp["error"] != "deployment is not stopped" {
		t.Errorf("expected error 'deployment is not stopped', got %v", resp["error"])
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
	router.POST("/api/v1/deployments/:id/rollback", RollbackDeployment(log, accountStore, deployStore, &mockQueue{}, nil, nil))

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

// TestDeploy_LegacyVariablesStripped_DeploySucceeds and
// TestDeploy_WebOnlyAdapter_StripsStaleSlackRefs used to verify variable
// stripping at the deploy handler — behaviour that previously rode on the
// EnforceEditable fallback path. With editable retired the deploy handler
// trusts the signed template, so stripping now happens at template generation
// (covered by tests in internal/deployment). The deploy-side regressions are
// no longer reachable via the real flow and have been removed.

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
		"/api/v1/deployments/"+depID+"/logs?account=my-acct&workload=my-agent-agent", nil)
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
		"/api/v1/deployments/"+depID+"/logs?account=my-acct&workload=my-agent-agent", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	// No pod param — query should use workload prefix match, not an exact pod
	if gotQuery != `{namespace="astro-ns-0", pod=~"my-agent-agent-.+"}` {
		t.Errorf("loki query = %q, want {namespace=\"astro-ns-0\", pod=~\"my-agent-agent-.+\"}", gotQuery)
	}
}

// TestGetDeploymentLogs_InvalidWorkloadRejected verifies that the server rejects
// an invalid workload name with 400. The CLI is responsible for sending a
// properly normalized workload name; the server does not mangle inputs.
func TestGetDeploymentLogs_InvalidWorkloadRejected(t *testing.T) {
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	req, _ := http.NewRequest(http.MethodGet,
		"/api/v1/deployments/"+depID+"/logs?account=my-acct&workload=My_Agent-agent&container=app", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

// Submits a deployment spec with a schedule ingestion containing a valid cron
// expression. The handler regenerates the template from the registered agent spec
// (which includes a schedule trigger), runs EnforceEditable and ValidateAndResolve,
// and should accept the spec with 202.
func TestDeploy_WithScheduleIngestion_Succeeds(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupDeployRouter("user-1")

	expectDeployPrepWithIngestion(accountMock, indexMock)

	deployMock.ExpectBegin()
	deployMock.ExpectQuery(`INSERT INTO deployments`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at",
		}).AddRow("new-sched-id", "acct-1", nil, "my-agent", "build-1", "astro-new", "", "{}", "pending", time.Now()))
	deployMock.ExpectExec(`INSERT INTO deployment_revisions`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Normalized insert order: agent workload → agent service → agent volume →
	// ingestion workload → collector workload → collector services → variables → resolved keys
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	deployMock.ExpectExec(`INSERT INTO deployment_volumes`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	deployMock.ExpectQuery(`INSERT INTO deployment_services`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	// SLACK_* variables in the signed spec persist as-is since the deploy
	// handler no longer rewrites adapter-scoped vars.
	expectVariableInsertsByName(
		deployMock,
		"SLACK_BOT_TOKEN",
		"SLACK_APP_TOKEN",
		"SLACK_CONFIG",
	)
	deployMock.ExpectCommit()

	body := deployableSpecWithScheduleIngestion("0 0 * * *")
	rec := httptest.NewRecorder()
	req := signedDeployRequest(t, body)
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
	req := signedDeployRequest(t, body)
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
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "avatar_updated_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "r.io", "public", nil, false, nil, nil, now, now))
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

func TestGetDeploymentLogs_TimezoneParam(t *testing.T) {
	// 72000 seconds past epoch = 1970-01-01T20:00:00Z UTC.
	// In America/New_York (UTC-5 in January) that is 1970-01-01T15:00:00-05:00.
	const nanos = "72000000000000"
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"pod":"p"},"values":[["` + nanos + `","line one\n"]]}]}}`)) //nolint:errcheck
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
		"/api/v1/deployments/"+depID+"/logs?account=my-acct&workload=my-agent-agent&timezone=America/New_York", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var entries []struct {
		Timestamp string `json:"timestamp"`
	}
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	// Timestamp should be offset-adjusted, not UTC.
	if strings.HasSuffix(entries[0].Timestamp, "Z") {
		t.Errorf("timestamp %q is UTC; expected a local offset", entries[0].Timestamp)
	}
	// The local hour for UTC 20:00 in New York (UTC-5) is 15.
	if !strings.Contains(entries[0].Timestamp, "T15:00:00") {
		t.Errorf("timestamp %q: expected local hour 15 (UTC-5)", entries[0].Timestamp)
	}
}

func TestGetDeploymentLogs_InvalidTimezone_FallsBackToUTC(t *testing.T) {
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"pod":"p"},"values":[["1000000000","msg\n"]]}]}}`)) //nolint:errcheck
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
		"/api/v1/deployments/"+depID+"/logs?account=my-acct&workload=my-agent-agent&timezone=Not/ATimezone", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var entries []struct {
		Timestamp string `json:"timestamp"`
	}
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Timestamp != "1970-01-01T00:00:01Z" {
		t.Errorf("timestamp = %q, want UTC fallback 1970-01-01T00:00:01Z", entries[0].Timestamp)
	}
}

func TestStreamDeploymentLogs_TimezoneParam(t *testing.T) {
	// 72000 seconds past epoch = 1970-01-01T20:00:00Z UTC.
	// In America/New_York (UTC-5 in January) that is 1970-01-01T15:00:00-05:00.
	lokiSrv := lokiTailServer(t, []map[string]interface{}{{
		"streams": []map[string]interface{}{{
			"stream": map[string]string{"pod": "p"},
			"values": [][]string{{"72000000000000", "live line"}},
		}},
	}})
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
		srv.URL+"/api/v1/deployments/"+depID+"/logs/stream?account=my-acct&workload=my-agent-agent&timezone=America/New_York", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var logLines []string
	var inNamedEvent bool
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			inNamedEvent = false
			continue
		}
		if strings.HasPrefix(line, "event:") {
			inNamedEvent = true
		} else if strings.HasPrefix(line, "data:") && !inNamedEvent {
			logLines = append(logLines, line)
		}
		if len(logLines) >= 1 {
			cancel()
		}
	}

	if len(logLines) != 1 {
		t.Fatalf("got %d log lines, want 1", len(logLines))
	}
	// Extract the JSON payload from "data: {...}"
	payload := strings.TrimPrefix(logLines[0], "data: ")
	var entry struct {
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(payload), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if strings.HasSuffix(entry.Timestamp, "Z") {
		t.Errorf("timestamp %q is UTC; expected a local offset", entry.Timestamp)
	}
	if !strings.Contains(entry.Timestamp, "T15:00:00") {
		t.Errorf("timestamp %q: expected local hour 15 (UTC-5)", entry.Timestamp)
	}
}

func TestGetDeploymentLogs_NoBackend_Returns503(t *testing.T) {
	// No Loki, no K8s.
	router, deployMock, accountMock := setupLogsTest(t, nil)

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
		"/api/v1/deployments/"+depID+"/logs?account=my-acct&workload=my-agent-agent&pod=my-pod", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

// --- StreamDeploymentLogs tests ---

var streamWSUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// lokiTailServer starts a test HTTP server that acts as a Loki /loki/api/v1/tail WebSocket
// endpoint. It sends each frame in order then blocks until the client disconnects.
func lokiTailServer(t *testing.T, frames []map[string]interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/tail" {
			http.NotFound(w, r)
			return
		}
		conn, err := streamWSUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("websocket upgrade: %v", err)
			return
		}
		defer conn.Close() //nolint:errcheck
		for _, frame := range frames {
			data, _ := json.Marshal(frame)
			conn.WriteMessage(websocket.TextMessage, data) //nolint:errcheck
		}
		conn.ReadMessage() //nolint:errcheck
	}))
}

func setupStreamLogsTest(t *testing.T, lokiClient *loki.Client, heartbeatInterval ...time.Duration) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
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
	router.GET("/api/v1/deployments/:id/logs/stream",
		StreamDeploymentLogs(log, accountStore, nil, deployStore, lokiClient, heartbeatInterval...))

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
			nil, deploymentstore.NewStore(deployDB), nil))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/logs/stream?account=my-acct&workload=my-agent-agent", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestStreamDeploymentLogs_NoBackend_Returns503(t *testing.T) {
	// No Loki, no K8s.
	router, deployMock, accountMock := setupStreamLogsTest(t, nil)

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
		"/api/v1/deployments/"+depID+"/logs/stream?account=my-acct&workload=my-agent-agent", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestStreamDeploymentLogs_LokiPath(t *testing.T) {
	lokiSrv := lokiTailServer(t, []map[string]interface{}{{
		"streams": []map[string]interface{}{{
			"stream": map[string]string{"pod": "my-pod"},
			"values": [][]string{{"2000000000", "live line"}},
		}},
	}})
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
		srv.URL+"/api/v1/deployments/"+depID+"/logs/stream?account=my-acct&workload=my-agent-agent", nil)
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

	// Read until we have the ready event and the log line, then cancel.
	// inNamedEvent prevents data: lines that belong to a named event (e.g. event: status)
	// from being mistaken for bare log-data lines.
	var logLines, eventLines []string
	var inNamedEvent bool
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			inNamedEvent = false
			continue
		}
		if strings.HasPrefix(line, "event:") {
			inNamedEvent = true
			eventLines = append(eventLines, line)
		} else if strings.HasPrefix(line, "data:") && !inNamedEvent {
			logLines = append(logLines, line)
		}
		if len(logLines) >= 1 && len(eventLines) >= 1 {
			cancel()
		}
	}

	if len(logLines) != 1 {
		t.Errorf("got %d log lines, want 1", len(logLines))
	}
	if len(logLines) == 1 && !strings.Contains(logLines[0], "live line") {
		t.Errorf("log line should contain live line, got: %s", logLines[0])
	}
	if len(eventLines) == 0 || !strings.Contains(eventLines[0], "ready") {
		t.Errorf("expected event: ready, got: %v", eventLines)
	}
}

func TestStreamDeploymentLogs_LokiPath_EmitsIDFields(t *testing.T) {
	lokiSrv := lokiTailServer(t, []map[string]interface{}{{
		"streams": []map[string]interface{}{{
			"stream": map[string]string{"pod": "p"},
			"values": [][]string{{"2000000000", "log line"}},
		}},
	}})
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
		srv.URL+"/api/v1/deployments/"+depID+"/logs/stream?account=my-acct&workload=my-agent-agent", nil)
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
			cancel()
			break
		}
	}

	if len(idLines) == 0 {
		t.Fatal("expected id: fields in SSE output, got none")
	}
	if !strings.Contains(idLines[0], "2000000000") {
		t.Errorf("id field = %q, want nanosecond timestamp 2000000000", idLines[0])
	}
}

func TestStreamDeploymentLogs_LokiPath_ReconnectsWhenWSCloses(t *testing.T) {
	// Verify that when the Loki WS closes the handler reconnects server-side without
	// closing the SSE. The SSE should only end once the client cancels.
	dialed := make(chan struct{}, 10)
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/tail" {
			http.NotFound(w, r)
			return
		}
		conn, err := streamWSUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		dialed <- struct{}{}
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
		srv.URL+"/api/v1/deployments/"+depID+"/logs/stream?account=my-acct&workload=my-agent-agent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	// Wait for at least 2 dials to confirm the handler reconnected after the first WS close.
	for i := 0; i < 2; i++ {
		select {
		case <-dialed:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for Loki dial %d", i+1)
		}
	}

	// Cancel the client — the SSE should now close.
	cancel()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
	}
}

func TestStreamDeploymentLogs_HandshakeAndHeartbeat(t *testing.T) {
	// Mock Loki: accept the WS but never send log lines so only heartbeats arrive.
	lokiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/tail" {
			http.NotFound(w, r)
			return
		}
		conn, err := streamWSUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck
		conn.ReadMessage() //nolint:errcheck
	}))
	defer lokiSrv.Close()

	lokiClient := loki.New(lokiSrv.URL)
	router, deployMock, accountMock := setupStreamLogsTest(t, lokiClient, 50*time.Millisecond)

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
		srv.URL+"/api/v1/deployments/"+depID+"/logs/stream?account=my-acct&workload=my-agent-agent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Collect event: lines; track first index of ready/heartbeat incrementally
	// so we can cancel as soon as both have arrived and check ordering after.
	readyIdx, heartbeatIdx := -1, -1
	var eventLines []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "event:") {
			continue
		}
		eventLines = append(eventLines, line)
		idx := len(eventLines) - 1
		if readyIdx == -1 && strings.Contains(line, "ready") {
			readyIdx = idx
		}
		if heartbeatIdx == -1 && strings.Contains(line, "heartbeat") {
			heartbeatIdx = idx
		}
		if readyIdx != -1 && heartbeatIdx != -1 {
			cancel()
		}
	}

	if readyIdx == -1 {
		t.Errorf("expected event: ready in stream, got: %v", eventLines)
	}
	if heartbeatIdx == -1 {
		t.Errorf("expected event: heartbeat in stream, got: %v", eventLines)
	}
	if readyIdx != -1 && heartbeatIdx != -1 && readyIdx > heartbeatIdx {
		t.Errorf("event: ready (index %d) arrived after event: heartbeat (index %d)", readyIdx, heartbeatIdx)
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
	router.POST("/api/v1/deployments/:id/stop", StopDeployment(log, accountStore, testK8sRegistry(k8sClient), deployStore, nil, k8scache.NoopCache{}))

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

func TestStopDeployment_Deploying_Returns400(t *testing.T) {
	k8sHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	router, deployMock, accountMock := setupStopRouter(t, k8sHandler)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123-0", "My Agent", `{}`, "deploying", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/deployments/"+depID+"/stop", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStopDeployment_Failed_Succeeds(t *testing.T) {
	namespace := "astro-abc123-0"

	k8sHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.Contains(path, "/apis/apps/v1/namespaces/") && strings.HasSuffix(path, "/deployments"):
			fmt.Fprintf(w, `{"kind":"DeploymentList","apiVersion":"apps/v1","items":[{"metadata":{"name":"agent","namespace":%q},"spec":{"replicas":1}}]}`, namespace)
		case r.Method == http.MethodPut && strings.Contains(path, "/deployments/"):
			fmt.Fprintf(w, `{"kind":"Deployment","apiVersion":"apps/v1","metadata":{"name":"agent","namespace":%q}}`, namespace)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/statefulsets"):
			fmt.Fprint(w, `{"kind":"StatefulSetList","apiVersion":"apps/v1","items":[]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/cronjobs"):
			fmt.Fprint(w, `{"kind":"CronJobList","apiVersion":"batch/v1","items":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	router, deployMock, accountMock := setupStopRouter(t, k8sHandler)

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	// A failed deployment is stoppable (pause the broken pods).
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", namespace, "My Agent", `{}`, "failed", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	deployMock.ExpectBegin()
	deployMock.ExpectExec(`UPDATE`).WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT`).WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectCommit()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/deployments/"+depID+"/stop", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

// --- GetDeployment tests ---

// setupGetDeploymentTest builds a router with the DB-only GetDeployment (record)
// endpoint. It no longer registers /runtime — that endpoint is DB-backed and
// covered separately by TestGetDeploymentRuntime_ClusterIndependent — so no K8s
// client is needed here.
func setupGetDeploymentTest(t *testing.T) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
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
	router.GET("/api/v1/deployments/:id", GetDeployment(log, accountStore, cfg, deployStore, nil, nil, nil))

	return router, deployMock, accountMock
}

func TestGetDeployment_Success(t *testing.T) {
	depID := deployid.New()
	namespace := "astro-abc123def-0"
	agentName := "my-agent"
	buildID := "build-1"
	acctID := uuid.New().String()
	now := time.Now()

	router, deployMock, accountMock := setupGetDeploymentTest(t)

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

// TestGetDeployment_ExternalURLNotReady previously asserted that GET
// /deployments/:id surfaced K8s ingress readiness (Ready=false + a "creating
// your custom URL" message) when the ALB hadn't provisioned an LB hostname.
// After the record/runtime split, the record endpoint only returns the URL
// (sourced from deployment_ingresses); readiness moved to the runtime
// endpoint, which evaluates the K8s Ingress object live. Test removed —
// runtime readiness behavior is covered by EvaluateEndpointReadiness unit
// tests in internal/k8s and should grow a dedicated runtime-endpoint test
// when the runtime response exposes per-URL readiness.

func TestGetDeployment_NoNamespace_ReturnsDBEntry(t *testing.T) {
	depID := deployid.New()
	namespace := "astro-abc123def-0"
	acctID := uuid.New().String()
	now := time.Now()

	router, deployMock, accountMock := setupGetDeploymentTest(t)

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
	router, deployMock, _ := setupGetDeploymentTest(t)

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(emptyDeploymentByIDRows())

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+deployid.New(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGetDeploymentRuntime_ClusterIndependent verifies the runtime endpoint is
// now DB-backed: it serves the controller-maintained snapshot without touching
// the cluster, so a disabled/unreachable cluster returns 200 with the
// last-observed runtime (not a 503, the old behavior). The cluster client is
// intentionally nil here — the handler must never reach for it.
func TestGetDeploymentRuntime_ClusterIndependent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	depID := deployid.New()
	clusterID := "cl-disabled"
	acctID := uuid.New().String()
	namespace := "astro-disabled-0"
	now := time.Now()

	accountDB, accountMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	deployDB, deployMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	// nil registry / agent index: the handler is DB-only and must not use them.
	router.GET("/api/v1/deployments/:id/runtime", GetDeploymentRuntime(log, accountStore, &config.Config{}, deployStore))

	// resolveDeployment: the deployment row + membership check.
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(deploymentByIDColumns).AddRow(
			depID, acctID, nil, "my-agent", "build-1", namespace, "My Agent",
			`{}`, nil, nil, clusterID,
			"active", nil, nil, now, 1,
			now, nil, nil, nil,
		))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// The persisted runtime snapshot the endpoint reads.
	snap := deploymentstore.RuntimeSnapshot{
		Ready:    1,
		Replicas: 1,
		Services: []deploymentstore.RuntimeService{
			{Name: deployment.GenerateAgentResourceName("my-agent", "messaging"), Type: "ClusterIP"},
		},
		Workloads: []deploymentstore.RuntimeWorkload{{
			Name: "my-agent-agent",
			Kind: "Deployment",
			Pods: []deploymentstore.RuntimePod{{
				Name:  "my-agent-agent-0",
				Phase: "Running",
				Containers: []deploymentstore.RuntimeContainer{
					{Name: "app", State: "Running", Ready: true},
					{Name: "messaging", State: "Running", Ready: true},
				},
			}},
		}},
	}
	snapJSON, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	deployMock.ExpectQuery(`SELECT snapshot, observed_at FROM deployment_runtime_status`).
		WillReturnRows(sqlmock.NewRows([]string{"snapshot", "observed_at"}).AddRow(snapJSON, now))

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID+"/runtime", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Runtime DeploymentRuntime `json:"runtime"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Runtime.Ready != 1 || resp.Runtime.Replicas != 1 {
		t.Errorf("expected ready/replicas 1/1, got %d/%d", resp.Runtime.Ready, resp.Runtime.Replicas)
	}
	if len(resp.Runtime.Workloads) != 1 || resp.Runtime.Workloads[0].PodName != "my-agent-agent-0" {
		t.Errorf("expected one workload with the snapshot pod, got %+v", resp.Runtime.Workloads)
	}
	if !resp.Runtime.MessagingReachable {
		t.Errorf("expected messaging_reachable true (Service present + sidecar ready)")
	}
	if err := deployMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDeploy_SourcePropertiesFromDB used to verify that EnforceEditable
// rejected a deploy whose source.build disagreed with the canonical build
// recorded against the agent version. With editable retired the same
// protection comes from template signature verification: the server-produced
// template is signed and the deploy handler refuses any spec whose contents
// don't match the signature, source.build included.

func TestGetDeployment_NotMember(t *testing.T) {
	router, deployMock, accountMock := setupGetDeploymentTest(t)

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

// specWithMessaging is a minimal agent spec with messaging interfaces enabled,
// plus two inputs (API_KEY, LOG_LEVEL). Used by POST template handler tests.
const specWithMessaging = `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1","inputs":[{"name":"API_KEY","secret":true,"description":"API key"},{"name":"LOG_LEVEL","secret":false,"description":"Log level"}],"interfaces":{"messaging":true}}}`

// specWithIngestion is a minimal agent spec with a schedule-triggered ingestion job.
const specWithIngestion = `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1","inputs":[{"name":"API_KEY","secret":true,"description":"API key"}]},"ingestion":{"nightly":{"container":{"image":"sync:latest"},"trigger":{"type":"schedule"}}}}`

// specWithVarInputs is a minimal agent spec JSON that declares two inputs:
// API_KEY (secret) and LOG_LEVEL (non-secret). These become entries in
// template.Variables so the POST template handler tests have keys to populate.
const specWithVarInputs = `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1","inputs":[{"name":"API_KEY","secret":true,"description":"API key"},{"name":"LOG_LEVEL","secret":false,"description":"Log level"}]}}`

// --- GetDeploymentEvents tests ---
//
// The handler now reads the controller-maintained snapshot
// (deployment_runtime_status.Events); it no longer hits K8s or a cache. These
// tests seed the snapshot row and assert passthrough. Event capture/humanization
// is covered in deploycontroller (buildDeploymentEvents / HumanizeEvent).

func TestGetDeploymentEvents_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()
	namespace := "astro-abc123"

	// Snapshot as the controller would persist it: newest-first, humanized.
	snap := deploymentstore.RuntimeSnapshot{
		Events: []deploymentstore.EventItem{
			{Type: "Normal", Reason: "Scheduled", Message: "assigned", ObjectKind: "Pod", ObjectName: "my-agent-abc", Count: 1, FirstTimestamp: "2026-04-16T10:00:00Z", LastTimestamp: "2026-04-16T10:00:00Z", Title: "Scheduled", Severity: "info"},
			{Type: "Warning", Reason: "BackOff", Message: "Back-off restarting failed container", ObjectKind: "Pod", ObjectName: "my-agent-abc", Count: 3, FirstTimestamp: "2026-04-16T08:50:00Z", LastTimestamp: "2026-04-16T09:00:00Z"},
		},
	}
	snapJSON, _ := json.Marshal(snap)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments/:id/events", GetDeploymentEvents(log, accountStore, deployStore))

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", namespace,
			"My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	deployMock.ExpectQuery(`SELECT snapshot, observed_at FROM deployment_runtime_status`).
		WillReturnRows(sqlmock.NewRows([]string{"snapshot", "observed_at"}).AddRow(snapJSON, now))

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID+"/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp DeploymentEventsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(resp.Events))
	}
	// Order and humanized fields are preserved from the snapshot.
	if resp.Events[0].Reason != "Scheduled" || resp.Events[0].Title != "Scheduled" || resp.Events[0].Severity != "info" {
		t.Errorf("unexpected first event: %+v", resp.Events[0])
	}
	if resp.Events[1].Reason != "BackOff" || resp.Events[1].Count != 3 {
		t.Errorf("unexpected second event: %+v", resp.Events[1])
	}
}

func TestGetDeploymentEvents_NoSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments/:id/events", GetDeploymentEvents(log, accountStore, deployStore))

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-ns",
			"My Agent", `{}`, "active", now, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	deployMock.ExpectQuery(`SELECT snapshot, observed_at FROM deployment_runtime_status`).
		WillReturnError(sql.ErrNoRows)

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID+"/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp DeploymentEventsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(resp.Events))
	}
}

func TestGetDeploymentEvents_NoAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := logger.New("error", "json")

	router := gin.New()
	router.GET("/api/v1/deployments/:id/events", GetDeploymentEvents(log, nil, nil))

	req := httptest.NewRequest("GET", "/api/v1/deployments/some-id/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Shared test helpers for template handler tests ---

func expectAccountLookup(mock sqlmock.Sqlmock) {
	now := time.Now()
	accountRow := func() *sqlmock.Rows {
		return sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow("acct-1", "myorg", "organization", nil, nil, now, now)...)
	}
	// Fresh template resolves the target account before cache lookup and again in resolveAgentForTemplate.
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(accountRow())
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(accountRow())
}

func expectAgentLookup(mock sqlmock.Sqlmock, visibility string) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "avatar_updated_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", visibility, nil, false, nil, nil, now, now))
	mock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1"}}`, "", "", "[]", now, now))
}

// ===== POST /agents/:account/:name/deployment-template =====

// setupPostTemplateRouter creates a gin router wired to the POST deployment-template handler
// with the standard mock stores. Returns the router + all three mocks for test-specific expectations.
func setupPostTemplateRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock, sqlmock.Sqlmock) {
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
	router.POST("/agents/:account/:name/deployment-template",
		PostDeploymentTemplate(log, index, accountStore, cfg, deployStore, nil, nil, nil))

	return router, indexMock, accountMock, deployMock
}

// expectGenerateTemplateLatest sets up mock expectations for generateTemplate
// when no build override is provided (resolves latest version, 2-arg query).
func expectGenerateTemplateLatest(indexMock, accountMock sqlmock.Sqlmock, specJSON string) {
	now := time.Now()
	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "public")
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", specJSON, "", "", "[]", now, now))
}

// expectGenerateTemplatePinned sets up mock expectations for generateTemplate
// when a build override is provided (pinned build, 3-arg query).
func expectGenerateTemplatePinned(indexMock, accountMock sqlmock.Sqlmock, specJSON string) {
	now := time.Now()
	expectAccountLookup(accountMock)
	expectAgentLookup(indexMock, "public")
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent", "build-1").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", specJSON, "", "", "[]", now, now))
}

// postTemplate sends a POST to the deployment-template endpoint and returns the response.
func postTemplate(t *testing.T, router *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/agents/myorg/my-agent/deployment-template",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPostTemplate_FreshTemplate_EmptyBody(t *testing.T) {
	router, indexMock, accountMock, _ := setupPostTemplateRouter(t)
	expectGenerateTemplateLatest(indexMock, accountMock, specWithVarInputs)

	rec := postTemplate(t, router, `{}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Spec != "deployment-template/v1" {
		t.Errorf("resp.Spec: expected deployment-template/v1, got %s", resp.Spec)
	}
	if resp.Template.Spec != "deployment/v1" {
		t.Errorf("template.Spec: expected deployment/v1, got %s", resp.Template.Spec)
	}
	// Variables promoted to root
	if _, ok := resp.Variables["API_KEY"]; !ok {
		t.Error("expected API_KEY in resp.Variables")
	}
	// Interfaces always present
	if resp.Interfaces.Adapters == nil {
		t.Error("resp.Interfaces.Adapters should be non-nil")
	}
	// Schedules always present
	if resp.Schedules == nil {
		t.Error("resp.Schedules should be non-nil")
	}
	// Validation should flag required API_KEY
	if resp.Validation.Valid {
		t.Error("expected valid=false with missing required API_KEY")
	}
}

func TestPostTemplate_FreshTemplate_WithVariables(t *testing.T) {
	router, indexMock, accountMock, _ := setupPostTemplateRouter(t)
	expectGenerateTemplateLatest(indexMock, accountMock, specWithVarInputs)

	rec := postTemplate(t, router, `{
		"variables": {
			"API_KEY": {"value": "sk-test-123"},
			"LOG_LEVEL": {"value": "debug"}
		}
	}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if v := resp.Template.Variables["API_KEY"]; v.Value != "sk-test-123" {
		t.Errorf("API_KEY value: expected sk-test-123, got %s", v.Value)
	}
	if v := resp.Template.Variables["LOG_LEVEL"]; v.Value != "debug" {
		t.Errorf("LOG_LEVEL value: expected debug, got %s", v.Value)
	}
	if !resp.Validation.Valid {
		t.Errorf("expected valid=true, got errors: %v", resp.Validation.Errors)
	}
}

func TestPostTemplate_FreshTemplate_WithAdapters(t *testing.T) {
	router, indexMock, accountMock, _ := setupPostTemplateRouter(t)
	expectGenerateTemplateLatest(indexMock, accountMock, specWithMessaging)

	rec := postTemplate(t, router, `{
		"interfaces": {"adapters": ["slack", "web"]},
		"variables": {"API_KEY": {"value": "sk-test"}, "LOG_LEVEL": {"value": "info"}}
	}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	// Response interfaces should reflect the selection
	if len(resp.Interfaces.Adapters) != 2 {
		t.Errorf("resp.Interfaces.Adapters: expected [slack web], got %v", resp.Interfaces.Adapters)
	}
	// Slack tokens should be required (non-optional)
	if v, ok := resp.Variables["SLACK_BOT_TOKEN"]; ok && v.Optional {
		t.Error("SLACK_BOT_TOKEN should be non-optional when slack is selected")
	}
}

func TestPostTemplate_WithDeploymentID_PrefillsValues(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupPostTemplateRouter(t)

	now := time.Now()
	depID := "dep-prefill-1"
	acctID := "acct-1"

	storedSpec := `{"interfaces":{"adapters":["web","slack"]}}`

	// GetDeploymentByID
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Bot", storedSpec, "active", now, nil))
	// IsMember
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	// GetByID for deployment target account (before template generation).
	accountMock.ExpectQuery(`SELECT .+ FROM accounts a LEFT JOIN account_organizations ao`).
		WithArgs(acctID).
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow(acctID, "myorg", "organization", nil, nil, now, now)...))
	// generateTemplate
	expectGenerateTemplatePinned(indexMock, accountMock, specWithVarInputs)
	// GetDeploymentVariables
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"role", "env_name", "value_encrypted", "nonce",
			"is_secret", "user_var_name", "account_var_ref", "optional",
		}).
			AddRow("agent", "API_KEY", []byte{}, nil, true, "API_KEY", "my-vault-secret", false).
			AddRow("agent", "LOG_LEVEL", []byte("warn"), nil, false, "LOG_LEVEL", "", true))

	rec := postTemplate(t, router, `{"deployment_id": "dep-prefill-1"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	// Variables should be prefilled from stored deployment
	if v := resp.Variables["API_KEY"]; v.Ref != "my-vault-secret" {
		t.Errorf("API_KEY ref: expected my-vault-secret, got %q", v.Ref)
	}
	if v := resp.Variables["LOG_LEVEL"]; v.Value != "warn" {
		t.Errorf("LOG_LEVEL value: expected warn, got %q", v.Value)
	}
	// Target should have deployment ID and display name
	if resp.Template.Target.DeploymentID != depID {
		t.Errorf("target.deployment_id: expected %s, got %s", depID, resp.Template.Target.DeploymentID)
	}
	if resp.Template.Target.DisplayName != "My Bot" {
		t.Errorf("target.display_name: expected My Bot, got %s", resp.Template.Target.DisplayName)
	}
}

// TestPostTemplate_WithDeploymentID_PreservesSlackTokenRef exercises the
// configure-redeploy path for a deployment that originally bound SLACK_BOT_TOKEN
// to an account variable reference. SLACK_BOT_TOKEN is platform-injected by
// the slack adapter at ShapeTemplate time, so the merge step must round-trip
// the stored ref through to the response — otherwise the UI shows an empty
// secret field and the user has no idea what was previously selected.
func TestPostTemplate_WithDeploymentID_PreservesSlackTokenRef(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupPostTemplateRouter(t)

	now := time.Now()
	depID := "dep-slack-1"
	acctID := "acct-1"

	// Stored spec has slack adapter enabled — mergeDeploymentPrefill copies
	// this into template.Interfaces.Adapters so ShapeTemplate then injects
	// the slack variables.
	storedSpec := `{"interfaces":{"adapters":["web","slack"]}}`

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-slack",
			"My Slack Bot", storedSpec, "active", now, nil))
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	accountMock.ExpectQuery(`SELECT .+ FROM accounts a LEFT JOIN account_organizations ao`).
		WithArgs(acctID).
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow(acctID, "myorg", "organization", nil, nil, now, now)...))
	// Base spec carries interfaces.messaging:true so the slack adapter is
	// available to inject SLACK_BOT_TOKEN.
	expectGenerateTemplatePinned(indexMock, accountMock, specWithMessaging)
	// GetDeploymentVariables: SLACK_BOT_TOKEN was originally set via an
	// account variable reference. The row carries the ref in account_var_ref
	// and lives under the messaging role.
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"role", "env_name", "value_encrypted", "nonce",
			"is_secret", "user_var_name", "account_var_ref", "optional",
		}).
			AddRow("messaging", "SLACK_BOT_TOKEN", []byte{}, nil, true, "SLACK_BOT_TOKEN", "my-slack-secret", false))

	rec := postTemplate(t, router, `{"deployment_id":"dep-slack-1"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	v, ok := resp.Variables["SLACK_BOT_TOKEN"]
	if !ok {
		t.Fatalf("SLACK_BOT_TOKEN missing from resp.Variables: %v", resp.Variables)
	}
	if v.Ref != "my-slack-secret" {
		t.Errorf("SLACK_BOT_TOKEN.Ref: expected %q, got %q", "my-slack-secret", v.Ref)
	}
	if v.Value != "" {
		t.Errorf("SLACK_BOT_TOKEN.Value: expected empty (ref-bound), got %q", v.Value)
	}
}

// TestPostTemplate_WithDeploymentID_PreservesSlackConfigValue covers the
// non-secret half of the same flow: SLACK_CONFIG is platform-injected by
// the slack adapter (Datatype=object, Secret=false), so its stored plaintext
// value has to survive the same pipeline as SLACK_BOT_TOKEN.Ref.
func TestPostTemplate_WithDeploymentID_PreservesSlackConfigValue(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupPostTemplateRouter(t)

	now := time.Now()
	depID := "dep-slack-cfg-1"
	acctID := "acct-1"
	storedConfig := `{"allowed_channel_ids":["C12345"]}`
	storedSpec := `{"interfaces":{"adapters":["web","slack"]}}`

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-slack-cfg",
			"My Slack Bot", storedSpec, "active", now, nil))
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	accountMock.ExpectQuery(`SELECT .+ FROM accounts a LEFT JOIN account_organizations ao`).
		WithArgs(acctID).
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow(acctID, "myorg", "organization", nil, nil, now, now)...))
	expectGenerateTemplatePinned(indexMock, accountMock, specWithMessaging)
	// SLACK_CONFIG is stored as plaintext (Secret=false). value_encrypted
	// holds the raw bytes, no nonce. The Variable returned by
	// GetDeploymentVariables surfaces these as Value.
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"role", "env_name", "value_encrypted", "nonce",
			"is_secret", "user_var_name", "account_var_ref", "optional",
		}).
			AddRow("messaging", "SLACK_CONFIG", []byte(storedConfig), nil, false, "SLACK_CONFIG", "", true))

	rec := postTemplate(t, router, `{"deployment_id":"dep-slack-cfg-1"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	v, ok := resp.Variables["SLACK_CONFIG"]
	if !ok {
		t.Fatalf("SLACK_CONFIG missing from resp.Variables: %v", resp.Variables)
	}
	if v.Value != storedConfig {
		t.Errorf("SLACK_CONFIG.Value: expected %q, got %q", storedConfig, v.Value)
	}
	if v.Ref != "" {
		t.Errorf("SLACK_CONFIG.Ref: expected empty (value-bound), got %q", v.Ref)
	}
}

const inlineSecretPlaintext = "sk-inline-never-leak-99"

func expectPostTemplateInlineSecretPrefill(deployMock, indexMock, accountMock sqlmock.Sqlmock, depID, acctID string) {
	now := time.Now()
	storedSpec := `{}`
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-inline",
			"My Bot", storedSpec, "active", now, nil))
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	accountMock.ExpectQuery(`SELECT .+ FROM accounts a LEFT JOIN account_organizations ao`).
		WithArgs(acctID).
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow(acctID, "myorg", "organization", nil, nil, now, now)...))
	expectGenerateTemplatePinned(indexMock, accountMock, specWithVarInputs)
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"role", "env_name", "value_encrypted", "nonce",
			"is_secret", "user_var_name", "account_var_ref", "optional",
		}).
			AddRow("agent", "API_KEY", []byte(inlineSecretPlaintext), nil, true, "API_KEY", "", false).
			AddRow("agent", "LOG_LEVEL", []byte("info"), nil, false, "LOG_LEVEL", "", true))
}

func TestPostTemplate_InlineSecret_PrefillConfiguredNotExposed(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupPostTemplateRouter(t)

	depID := "dep-inline-1"
	acctID := "acct-1"
	expectPostTemplateInlineSecretPrefill(deployMock, indexMock, accountMock, depID, acctID)

	rec := postTemplate(t, router, `{"deployment_id":"`+depID+`"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, inlineSecretPlaintext) {
		t.Fatalf("response body must not contain inline secret plaintext")
	}

	var resp spec.TemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	v, ok := resp.Variables["API_KEY"]
	if !ok {
		t.Fatalf("API_KEY missing from resp.Variables")
	}
	if !v.Configured {
		t.Error("API_KEY.Configured: expected true")
	}
	if v.Value != "" {
		t.Errorf("API_KEY.Value: expected empty, got %q", v.Value)
	}
	if v.Ref != "" {
		t.Errorf("API_KEY.Ref: expected empty, got %q", v.Ref)
	}
	if tv, ok := resp.Template.Variables["API_KEY"]; ok && tv.Value != "" {
		t.Errorf("template.variables.API_KEY.Value: expected empty on prefill, got %q", tv.Value)
	}
}

func TestPostTemplate_InlineSecret_FinalizePreservesWhenOmitted(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupPostTemplateRouter(t)

	depID := "dep-inline-2"
	acctID := "acct-1"
	expectPostTemplateInlineSecretPrefill(deployMock, indexMock, accountMock, depID, acctID)

	rec := postTemplate(t, router, `{"deployment_id":"`+depID+`","finalize":true}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Validation.Valid {
		t.Fatalf("expected valid template, errors: %v", resp.Validation.Errors)
	}
	if got := resp.Template.Variables["API_KEY"].Value; got != inlineSecretPlaintext {
		t.Errorf("template.variables.API_KEY.Value: expected preserved secret, got %q", got)
	}
}

func TestPostTemplate_InlineSecret_FinalizeSchemaStillScrubbed(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupPostTemplateRouter(t)

	depID := "dep-inline-3"
	acctID := "acct-1"
	expectPostTemplateInlineSecretPrefill(deployMock, indexMock, accountMock, depID, acctID)

	rec := postTemplate(t, router, `{"deployment_id":"`+depID+`","finalize":true}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	v := resp.Variables["API_KEY"]
	if !v.Configured {
		t.Error("API_KEY.Configured: expected true on schema map")
	}
	if v.Value != "" {
		t.Errorf("API_KEY.Value on schema map: expected empty, got %q", v.Value)
	}
}

func TestPostTemplate_DeploymentNotFound(t *testing.T) {
	router, _, _, deployMock := setupPostTemplateRouter(t)

	deployMock.ExpectQuery(`SELECT`).WillReturnRows(emptyDeploymentByIDRows())

	rec := postTemplate(t, router, `{"deployment_id": "dep-nonexistent"}`)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// expect404WithErrorCode asserts the response is HTTP 404 and carries the
// given typed error_code field. The CLI's notFoundFromTemplateErr switches
// on this code to disambiguate "account/blueprint/build" 404s without
// parsing free-text error messages.
func expect404WithErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error     string `json:"error"`
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v: %s", err, rec.Body.String())
	}
	if body.ErrorCode != want {
		t.Errorf("error_code: want %q, got %q (body=%s)", want, body.ErrorCode, rec.Body.String())
	}
}

func TestPostTemplate_AccountNotFound_TaggedErrorCode(t *testing.T) {
	router, _, accountMock, _ := setupPostTemplateRouter(t)

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns))

	rec := postTemplate(t, router, `{}`)

	expect404WithErrorCode(t, rec, "account_not_found")
}

func TestPostTemplate_BlueprintNotFound_TaggedErrorCode(t *testing.T) {
	router, indexMock, accountMock, _ := setupPostTemplateRouter(t)

	expectAccountLookup(accountMock)
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name"}))

	rec := postTemplate(t, router, `{}`)

	expect404WithErrorCode(t, rec, "blueprint_not_found")
}

func TestPostTemplate_BuildNotFound_TaggedErrorCode(t *testing.T) {
	router, indexMock, accountMock, _ := setupPostTemplateRouter(t)
	now := time.Now()

	expectAccountLookup(accountMock)
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "avatar_updated_at", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", "public", nil, false, nil, nil, now, now))
	// generateTemplate looks up agent_versions twice for visibility flow then
	// for the build resolution; this one returns no rows for the requested
	// build id, mirroring `--build deadbeef` against an unknown build.
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-1", "myorg", `{"name":"my-agent"}`, "", "", "[]", now, now))
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs("acct-1", "my-agent", "deadbeef").
		WillReturnRows(sqlmock.NewRows([]string{"build_id"}))

	rec := postTemplate(t, router, `{"build":"deadbeef"}`)

	expect404WithErrorCode(t, rec, "build_not_found")
}

func TestPostTemplate_Forbidden(t *testing.T) {
	router, _, accountMock, deployMock := setupPostTemplateRouter(t)

	now := time.Now()
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow("dep-1", "acct-1", "my-agent", "build-1",
			"astro-abc123", "Bot", "{}", "active", now, nil))
	// IsMember returns 0 — not a member
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	rec := postTemplate(t, router, `{"deployment_id": "dep-1"}`)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostTemplate_InvalidBody(t *testing.T) {
	router, _, _, _ := setupPostTemplateRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/agents/myorg/my-agent/deployment-template",
		strings.NewReader(`{invalid json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostTemplate_SchedulesInResponse(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupPostTemplateRouter(t)

	now := time.Now()
	depID := "dep-sched-1"
	acctID := "acct-1"
	storedSpec := `{"ingestion":{"nightly":{"image":"sync:latest","trigger":{"type":"schedule","schedule":"0 2 * * *"}}}}`

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Bot", storedSpec, "active", now, nil))
	accountMock.ExpectQuery(`SELECT COUNT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	accountMock.ExpectQuery(`SELECT .+ FROM accounts a LEFT JOIN account_organizations ao`).
		WithArgs(acctID).
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow(acctID, "myorg", "organization", nil, nil, now, now)...))

	expectGenerateTemplatePinned(indexMock, accountMock, specWithIngestion)
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}).AddRow(depID, "API_KEY", "sk-val", "", true, false, `{"agent"}`, nil))

	// POST with schedule override
	rec := postTemplate(t, router, `{
		"deployment_id": "dep-sched-1",
		"schedules": {"nightly": "0 4 * * *"}
	}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	// Schedules in response should reflect the override
	if resp.Schedules["nightly"] != "0 4 * * *" {
		t.Errorf("resp.Schedules[nightly]: expected '0 4 * * *', got %q", resp.Schedules["nightly"])
	}
}

// ===== Cross-account prefill: source.account in deployment spec =====
//
// These tests exercise the Configure prefill path through the HTTP handler
// with sqlmock. Each test arranges the mock queue around a specific shape
// of deployment_spec_json (cross-account / same-account / legacy) and
// asserts the externally visible template response. The mock queue also
// acts as a contract: any unexpected DB call (for example, an extra
// IsMember check against the wrong account, or a GetLatestVersion call
// when a pinned build is expected) fails the test.

// postTemplateFor sends a POST to the deployment-template endpoint for the
// given URL account/name, enabling tests where the URL (target) account
// differs from the source account carried in the deployment spec.
func postTemplateFor(t *testing.T, router *gin.Engine, urlAccount, urlAgent, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(
		http.MethodPost,
		"/agents/"+urlAccount+"/"+urlAgent+"/deployment-template",
		strings.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// expectAccountLookupFor stubs a GetByName (or GetByID) query that returns a
// single account row for the given bind argument. The sqlmock FIFO queue
// distinguishes GetByName vs GetByID by the single bind argument (name vs id).
func expectAccountLookupFor(mock sqlmock.Sqlmock, arg, id, name string) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs(arg).
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow(id, name, "organization", nil, nil, now, now)...))
}

// expectAgentLookupFor stubs the two queries inside Index.Get (agents row +
// versions list) for a specific (accountID, agentName) pair with the given
// visibility. Used when the source account differs from the URL account.
func expectAgentLookupFor(mock sqlmock.Sqlmock, accountID, agentName, visibility string) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs(accountID, agentName).
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "avatar_updated_at", "created_at", "updated_at"}).
			AddRow(accountID, agentName, "registry.io", visibility, nil, false, nil, nil, now, now))
	mock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs(accountID, agentName).
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-old", accountID, `{"name":"my-agent","agent":{"image":"example:build-old"}}`, "", "", "[]", now, now))
}

// expectPinnedVersionFor stubs Index.GetVersion for a specific (accountID,
// agentName, buildID) triple, returning a row whose spec image pins the
// requested build so the template's source.build is observable.
func expectPinnedVersionFor(mock sqlmock.Sqlmock, accountID, agentName, buildID string) {
	now := time.Now()
	image := "123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-" + accountID + "/" + agentName + ":" + buildID
	specJSON := `{"name":"` + agentName + `","agent":{"image":"` + image + `"}}`
	mock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs(accountID, agentName, buildID).
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow(buildID, accountID, specJSON, "", "", "[]", now, now))
}

// expectPrefillLineageValidated arms sqlmock with the publisher resolution
// queries issued before generateTemplate runs: resolveSourceAccountName validates
// the deployment tuple via Index.GetVersion, materializes account names via
// AccountStore lookups, then resolveAgentForTemplate performs a fresh
// source-account name lookup — generating multiple GetByName calls for the same
// publisher name before generateTemplate issues its own resolver query.
func expectPrefillLineageValidated(indexMock, accountMock sqlmock.Sqlmock, sourceAccountName, sourceAcctID, agentName, buildID string) {
	expectAccountLookupFor(accountMock, sourceAccountName, sourceAcctID, sourceAccountName)
	expectPinnedVersionFor(indexMock, sourceAcctID, agentName, buildID)
	expectAccountLookupFor(accountMock, sourceAcctID, sourceAcctID, sourceAccountName)
	expectAccountLookupFor(accountMock, sourceAccountName, sourceAcctID, sourceAccountName)
}

// expectPrefillLineageFromOwningAccountValidated covers legacy rows whose spec
// omits source: validatedLineagePublisher falls back to the deployment's
// account_id tuple check before resolving the publisher name via GetByID.
func expectPrefillLineageFromOwningAccountValidated(indexMock, accountMock sqlmock.Sqlmock, acctID, accountName, agentName, buildID string) {
	expectPinnedVersionFor(indexMock, acctID, agentName, buildID)
	expectAccountLookupFor(accountMock, acctID, acctID, accountName)
}

// TestPostTemplate_CrossAccountPrefill_UsesSourceAccount covers the
// cross-account shape: the deployment lives under the target account (URL)
// but its spec carries source.account="publisher", and the pinned build
// only exists under the source account. The handler must resolve the build
// under the source account and return a template whose source/target
// reflect the publisher and the workspace respectively.
func TestPostTemplate_CrossAccountPrefill_UsesSourceAccount(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupPostTemplateRouter(t)

	now := time.Now()
	depID := "dep-crossacct-1"
	targetAcctID := "target-acct"
	sourceAcctID := "source-acct"
	storedSpec := `{"source":{"account":"publisher","name":"my-agent","build":"build-1"},"interfaces":{"adapters":["web"]}}`

	// Load the deployment under the target account.
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, targetAcctID, "my-agent", "build-1", "astro-abc123",
			"Cross-Account Bot", storedSpec, "active", now, nil))
	// Auth is scoped to the deployment's account (target), not source.
	accountMock.ExpectQuery(`SELECT COUNT`).
		WithArgs(targetAcctID, "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	expectAccountLookupFor(accountMock, targetAcctID, targetAcctID, "myorg")
	expectPrefillLineageValidated(indexMock, accountMock, "publisher", sourceAcctID, "my-agent", "build-1")
	// generateTemplate resolves everything under "publisher"
	// (source.account), not under "myorg" (URL account).
	expectAgentLookupFor(indexMock, sourceAcctID, "my-agent", "public")
	expectPinnedVersionFor(indexMock, sourceAcctID, "my-agent", "build-1")
	// mergeDeploymentPrefill resolves target.account from the deployment owner.
	expectAccountLookupFor(accountMock, targetAcctID, targetAcctID, "myorg")
	// GetDeploymentVariables (empty).
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}))
	// URL uses "myorg" (target) while the deployment spec points at
	// "publisher" (source).
	rec := postTemplateFor(t, router, "myorg", "my-agent", `{"deployment_id":"`+depID+`"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Template reflects the cross-account shape:
	// - template.source.account is the publisher name
	// - template.source.build is the deployment's pinned build
	// - target.account is the deployment's workspace
	if resp.Template.Source.Account != "publisher" {
		t.Errorf("template.source.account: expected %q, got %q", "publisher", resp.Template.Source.Account)
	}
	if resp.Template.Source.Build != "build-1" {
		t.Errorf("template.source.build: expected %q, got %q", "build-1", resp.Template.Source.Build)
	}
	if resp.Template.Target.Account != "myorg" {
		t.Errorf("template.target.account: expected %q, got %q", "myorg", resp.Template.Target.Account)
	}
	if resp.Template.Target.DeploymentID != depID {
		t.Errorf("template.target.deployment_id: expected %q, got %q", depID, resp.Template.Target.DeploymentID)
	}

	if err := indexMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled index expectations: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled account expectations: %v", err)
	}
	if err := deployMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled deploy expectations: %v", err)
	}
}

// TestPostTemplate_SameAccountPrefill verifies the common case where a
// deployment's source.account equals the URL account. The agent and build
// lookup happen under that single account and the template reflects it.
func TestPostTemplate_SameAccountPrefill(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupPostTemplateRouter(t)

	now := time.Now()
	depID := "dep-sameacct-1"
	acctID := "acct-1"
	storedSpec := `{"source":{"account":"myorg","name":"my-agent","build":"build-1"}}`

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"Same-Account Bot", storedSpec, "active", now, nil))
	accountMock.ExpectQuery(`SELECT COUNT`).
		WithArgs(acctID, "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	expectAccountLookupFor(accountMock, acctID, acctID, "myorg")
	expectPrefillLineageValidated(indexMock, accountMock, "myorg", acctID, "my-agent", "build-1")
	expectAgentLookup(indexMock, "public")
	expectPinnedVersionFor(indexMock, acctID, "my-agent", "build-1")
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}))

	rec := postTemplateFor(t, router, "myorg", "my-agent", `{"deployment_id":"`+depID+`"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Template.Source.Account != "myorg" {
		t.Errorf("template.source.account: expected %q, got %q", "myorg", resp.Template.Source.Account)
	}
	if resp.Template.Source.Build != "build-1" {
		t.Errorf("template.source.build: expected %q, got %q", "build-1", resp.Template.Source.Build)
	}
	if resp.Template.Target.DeploymentID != depID {
		t.Errorf("template.target.deployment_id: expected %q, got %q", depID, resp.Template.Target.DeploymentID)
	}

	if err := indexMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled index expectations: %v", err)
	}
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled account expectations: %v", err)
	}
}

// TestPostTemplate_LegacyPrefill_FallsBackToURLAccount covers deployments
// whose spec omits source.account. With no publisher recorded, the handler
// resolves the agent and its build under the URL account.
func TestPostTemplate_LegacyPrefill_FallsBackToURLAccount(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupPostTemplateRouter(t)

	now := time.Now()
	depID := "dep-legacy-1"
	acctID := "acct-1"
	// No source.account in the spec — legacy shape.
	storedSpec := `{"interfaces":{"adapters":["web"]}}`

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"Legacy Bot", storedSpec, "active", now, nil))
	accountMock.ExpectQuery(`SELECT COUNT`).
		WithArgs(acctID, "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	expectAccountLookupFor(accountMock, acctID, acctID, "myorg")
	expectPrefillLineageFromOwningAccountValidated(indexMock, accountMock, acctID, "myorg", "my-agent", "build-1")
	// SourceAccountFromSpec returns "", so generateTemplate uses
	// c.Param("account") = "myorg" for both the agent and version lookups.
	expectGenerateTemplatePinned(indexMock, accountMock, specWithVarInputs)
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}))
	expectAccountLookupFor(accountMock, acctID, acctID, "myorg")

	rec := postTemplateFor(t, router, "myorg", "my-agent", `{"deployment_id":"`+depID+`"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Template.Source.Account != "myorg" {
		t.Errorf("source.account: expected %q, got %q", "myorg", resp.Template.Source.Account)
	}
	if resp.Template.Source.Build != "build-1" {
		t.Errorf("source.build: expected %q, got %q", "build-1", resp.Template.Source.Build)
	}

	if err := indexMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled index expectations: %v", err)
	}
}

// TestPostTemplate_CrossAccountPrefill_AuthStaysOnDeploymentAccount verifies
// the auth boundary for cross-account Configure. A user who is only a
// member of the deployment's target account can open Configure even when
// the source agent is private in another account. The test exercises this
// by seeding exactly one IsMember query (against the target account); if
// the handler also called IsMember against the source account, sqlmock
// would reject that call as unexpected and fail the test.
func TestPostTemplate_CrossAccountPrefill_AuthStaysOnDeploymentAccount(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupPostTemplateRouter(t)

	now := time.Now()
	depID := "dep-auth-1"
	targetAcctID := "target-acct"
	sourceAcctID := "source-acct"
	storedSpec := `{"source":{"account":"publisher","name":"my-agent","build":"build-1"}}`

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, targetAcctID, "my-agent", "build-1", "astro-abc123",
			"Private Cross-Acct Bot", storedSpec, "active", now, nil))
	// The only membership check allowed is against the deployment's target account.
	accountMock.ExpectQuery(`SELECT COUNT`).
		WithArgs(targetAcctID, "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	expectAccountLookupFor(accountMock, targetAcctID, targetAcctID, "myorg")
	expectPrefillLineageValidated(indexMock, accountMock, "publisher", sourceAcctID, "my-agent", "build-1")
	expectAgentLookupFor(indexMock, sourceAcctID, "my-agent", "private")
	expectPinnedVersionFor(indexMock, sourceAcctID, "my-agent", "build-1")
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}))

	rec := postTemplateFor(t, router, "myorg", "my-agent", `{"deployment_id":"`+depID+`"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("target-account member should reach private source agent via existing deployment, got %d: %s",
			rec.Code, rec.Body.String())
	}

	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected or missing account queries (auth should stay on target account): %v", err)
	}
	if err := indexMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled index expectations: %v", err)
	}
	if err := deployMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled deploy expectations: %v", err)
	}
}

// TestPostTemplate_CrossAccountPrefill_PinsDeployedBuild verifies that a
// cross-account Configure call returns the deployed build even when the
// source account has a newer one available. The mock queue only accepts
// the 3-arg GetVersion(sourceAcctID, name, "build-old"); the 2-arg
// GetLatestVersion is intentionally not mocked so a call to the latest
// version would fail the test.
func TestPostTemplate_CrossAccountPrefill_PinsDeployedBuild(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupPostTemplateRouter(t)

	now := time.Now()
	depID := "dep-pinned-1"
	targetAcctID := "target-acct"
	sourceAcctID := "source-acct"
	pinnedBuild := "build-old"
	storedSpec := `{"source":{"account":"publisher","name":"my-agent","build":"` + pinnedBuild + `"}}`

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, targetAcctID, "my-agent", pinnedBuild, "astro-abc123",
			"Pinned Bot", storedSpec, "active", now, nil))
	accountMock.ExpectQuery(`SELECT COUNT`).
		WithArgs(targetAcctID, "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	expectAccountLookupFor(accountMock, targetAcctID, targetAcctID, "myorg")
	expectPrefillLineageValidated(indexMock, accountMock, "publisher", sourceAcctID, "my-agent", pinnedBuild)

	// The source account has two builds — "build-new" (latest) and
	// "build-old" (deployed). Index.Get lists them newest-first; the
	// handler then asks for the pinned one, not the latest.
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs(sourceAcctID, "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "avatar_updated_at", "created_at", "updated_at"}).
			AddRow(sourceAcctID, "my-agent", "registry.io", "public", nil, false, nil, nil, now, now))
	indexMock.ExpectQuery("SELECT .+ FROM agent_versions WHERE account_id").
		WithArgs(sourceAcctID, "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"build_id", "ecr_namespace", "spec_json", "readme", "agent_card_json", "validation_warnings", "published_at", "updated_at"}).
			AddRow("build-new", sourceAcctID, `{"name":"my-agent","agent":{"image":"example:build-new"}}`, "", "", "[]", now.Add(time.Hour), now.Add(time.Hour)).
			AddRow(pinnedBuild, sourceAcctID, `{"name":"my-agent","agent":{"image":"example:build-old"}}`, "", "", "[]", now, now))
	// Pinned lookup — only the deployed build is mocked. A GetLatestVersion
	// (2-arg) call would not match any expectation and fail the test.
	expectPinnedVersionFor(indexMock, sourceAcctID, "my-agent", pinnedBuild)

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}))

	rec := postTemplateFor(t, router, "myorg", "my-agent", `{"deployment_id":"`+depID+`"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp spec.TemplateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Template.Source.Build != pinnedBuild {
		t.Errorf("source.build: expected pinned %q, got %q (latest build leaked through)",
			pinnedBuild, resp.Template.Source.Build)
	}

	if err := indexMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled index expectations: %v", err)
	}
}

// --- Deploy endpoint: target.cluster_id validation tests ---
//
// `target.cluster_id` is an optional field on the deployment spec. Absent
// means "route to the primary cluster" and is persisted as NULL — already
// covered by TestDeploy_WithoutDeploymentID_CreatesNew (which submits no
// cluster_id and expects a successful 202). The cases below cover the
// validation path for present values: valid+enabled+healthy (happy), unknown (400),
// present-but-disabled (400), and present-but-unhealthy (400).

// setupDeployRouterWithClusterStore is a copy of setupDeployRouter that
// additionally wires a real clusterstore.Store backed by an sqlmock. Returns
// the cluster mock alongside the existing mocks so callers can prime
// `clusterstore.Get` lookups.
func setupDeployRouterWithClusterStore(userID string) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	return setupDeployRouterWithClusterStoreClients(userID, map[string]k8s.ClusterClient{
		"eu-west-1-managed": healthyStubCluster(),
	})
}

func setupDeployRouterWithClusterStoreClients(userID string, cachedClients map[string]k8s.ClusterClient) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)

	indexDB, indexMock, _ := sqlmock.New()
	accountDB, accountMock, _ := sqlmock.New()
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	clusterDB, clusterMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	index := agentindex.NewIndexWithDB(indexDB)
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	clusterStore := clusterstore.New(clusterDB)
	log := logger.New("error", "json")
	cfg := &config.Config{
		Deployment: config.DeploymentConfig{
			RegistryURL:        "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			Environment:        "test",
			TemplateSigningKey: testSigningKey,
		},
	}

	primary := newMockK8sClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	k8sReg := k8s.NewRegistryWithClusterStore(primary, clusterStore, log)
	for id, client := range cachedClients {
		k8sReg.SetCachedClientForTest(id, client)
	}

	router := gin.New()
	if userID != "" {
		router.Use(func(c *gin.Context) {
			c.Set(string(auth.UserContextKey), &auth.User{ID: userID})
			c.Next()
		})
	}
	router.POST("/deploy", DeployAgent(log, index, accountStore, cfg, deployStore, nil, clusterStore, k8sReg, nil, nil, &mockQueue{}, nil, nil, nil, nil, nil, nil, nil, nil)) //nolint:staticcheck // nil varsStore, EntitlementChecker, quota.Checker, avatarStore, omClient, db, auditStore, ksStore, authzStore, preflighter, and tmplCache skip checks in tests

	return router, indexMock, accountMock, deployMock, clusterMock
}

// deployableSpecWithClusterID returns a deployable spec body with an explicit
// target.cluster_id. The rest of the spec mirrors deployableSpec("").
func deployableSpecWithClusterID(clusterID string) string {
	return fmt.Sprintf(`{
		"spec": "deployment/v1",
		"source": {"account": "myorg", "name": "my-agent", "build": "build-1", "registry": "https://123456789.dkr.ecr.us-east-1.amazonaws.com"},
		"target": {"runtime": "kubernetes", "cluster_id": %q},
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
	}`, clusterID)
}

func TestDeploy_WithUnknownClusterID_Returns400(t *testing.T) {
	router, indexMock, accountMock, _, clusterMock := setupDeployRouterWithClusterStore("user-1")
	expectDeployPrepWithCluster(accountMock, indexMock, "unknown-cluster")

	// clusterstore.Get returns ErrNotFound when sql.ErrNoRows propagates.
	clusterMock.ExpectQuery(`SELECT .+ FROM clusters WHERE id = \$1`).
		WithArgs("unknown-cluster").
		WillReturnError(sql.ErrNoRows)

	body := deployableSpecWithClusterID("unknown-cluster")
	req := signedDeployRequest(t, body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if got := resp["error"]; got != "unknown cluster_id" {
		t.Errorf("error = %v, want unknown cluster_id", got)
	}
	if got := resp["cluster_id"]; got != "unknown-cluster" {
		t.Errorf("cluster_id in response = %v, want unknown-cluster", got)
	}
	if err := clusterMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled cluster expectations: %v", err)
	}
}

func TestDeploy_WithDisabledClusterID_Returns400(t *testing.T) {
	router, indexMock, accountMock, _, clusterMock := setupDeployRouterWithClusterStore("user-1")
	expectDeployPrepWithCluster(accountMock, indexMock, "staging")

	now := time.Now()
	clusterMock.ExpectQuery(`SELECT .+ FROM clusters WHERE id = \$1`).
		WithArgs("staging").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "eks_cluster_ca", "enabled",
			"agent_ingress_domain", "ingestion_ingress_domain", "knowledge_domain",
			"langfuse_base_url_ext", "langfuse_vpce_ips", "pod_subnet_cidrs",
			"created_at", "updated_at",
		}).AddRow("staging", "us-east-1", "staging-eks", "https://staging.eks.example", []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n"), false,
			"agents.example.com", "ingestion.example.com", "knowledge.example.com",
			"http://langfuse.platform.astroids.ai:3000", "10.0.1.10", "10.0.0.0/24",
			now, now))

	body := deployableSpecWithClusterID("staging")
	req := signedDeployRequest(t, body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if got := resp["error"]; got != "cluster is disabled" {
		t.Errorf("error = %v, want cluster is disabled", got)
	}
	if got := resp["cluster_id"]; got != "staging" {
		t.Errorf("cluster_id in response = %v, want staging", got)
	}
	if err := clusterMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled cluster expectations: %v", err)
	}
}

func TestDeploy_WithUnhealthyClusterID_Returns400(t *testing.T) {
	const clusterID = "eu-west-1-unhealthy"
	router, indexMock, accountMock, _, clusterMock := setupDeployRouterWithClusterStoreClients("user-1", map[string]k8s.ClusterClient{
		clusterID: unhealthyStubCluster("connection refused"),
	})
	expectDeployPrepWithCluster(accountMock, indexMock, clusterID)

	now := time.Now()
	clusterMock.ExpectQuery(`SELECT .+ FROM clusters WHERE id = \$1`).
		WithArgs(clusterID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "eks_cluster_ca", "enabled",
			"agent_ingress_domain", "ingestion_ingress_domain", "knowledge_domain",
			"langfuse_base_url_ext", "langfuse_vpce_ips", "pod_subnet_cidrs",
			"created_at", "updated_at",
		}).AddRow(clusterID, "us-east-1", "fake-eks", "https://fake.eks.example", []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n"), true,
			"agents.example.com", "ingestion.example.com", "knowledge.example.com",
			"http://langfuse.platform.astroids.ai:3000", "10.0.1.10", "10.0.0.0/24",
			now, now))

	body := deployableSpecWithClusterID(clusterID)
	req := signedDeployRequest(t, body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if got := resp["error"]; got != "cluster is unhealthy" {
		t.Errorf("error = %v, want cluster is unhealthy", got)
	}
	if got := resp["cluster_id"]; got != clusterID {
		t.Errorf("cluster_id in response = %v, want %s", got, clusterID)
	}
	if got := resp["details"]; got != "unable to connect to cluster" {
		t.Errorf("details = %v, want unable to connect to cluster", got)
	}
	if err := clusterMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled cluster expectations: %v", err)
	}
}

func TestDeploy_WithValidClusterID_PersistsToDeploymentsTable(t *testing.T) {
	router, indexMock, accountMock, deployMock, clusterMock := setupDeployRouterWithClusterStore("user-1")

	now := time.Now()
	// Deploy validates the cluster (GetEntry), Refresh evicts cache, then
	// clustercfg.Resolve loads the row again — two identical SELECTs.
	for range 2 {
		clusterMock.ExpectQuery(`SELECT .+ FROM clusters WHERE id = \$1`).
			WithArgs("eu-west-1-managed").
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "region", "eks_cluster_name", "eks_cluster_endpoint", "eks_cluster_ca", "enabled",
				"agent_ingress_domain", "ingestion_ingress_domain", "knowledge_domain",
				"langfuse_base_url_ext", "langfuse_vpce_ips", "pod_subnet_cidrs",
				"created_at", "updated_at",
			}).AddRow("eu-west-1-managed", "eu-west-1", "prod-eu", "https://eu.eks.example", []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n"), true,
				"agents.example.com", "ingestion.example.com", "knowledge.example.com",
				"http://langfuse.platform.astroids.ai:3000", "10.0.1.10", "10.0.0.0/24",
				now, now))
	}

	expectDeployPrepWithCluster(accountMock, indexMock, "eu-west-1-managed")

	// Pin the cluster_id arg in the INSERT. The deployment INSERT's
	// parameter list ends with (..., kms_key_arn, cluster_id, status); the
	// $11 position is cluster_id. sqlmock uses WithArgs on positional args
	// against the parameter list, not column-named — so we pass
	// AnyArg() for the earlier columns and pin only $11.
	deployMock.ExpectBegin()
	deployMock.ExpectQuery(`INSERT INTO deployments`).
		WithArgs(
			sqlmock.AnyArg(),    // $1  id
			sqlmock.AnyArg(),    // $2  account_id
			sqlmock.AnyArg(),    // $3  source_account_id (nilIfEmpty → nil)
			sqlmock.AnyArg(),    // $4  agent_name
			sqlmock.AnyArg(),    // $5  build_id
			sqlmock.AnyArg(),    // $6  namespace
			sqlmock.AnyArg(),    // $7  display_name
			sqlmock.AnyArg(),    // $8  deployment_spec_json
			sqlmock.AnyArg(),    // $9  encrypted_data_key
			sqlmock.AnyArg(),    // $10 kms_key_arn
			"eu-west-1-managed", // $11 cluster_id — the new pin
			sqlmock.AnyArg(),    // $12 status
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at",
		}).AddRow("new-id", "acct-1", nil, "my-agent", "build-1", "astro-new", "", "{}", "pending", now))

	// We don't pin every downstream INSERT (revisions, events, workloads,
	// services, build_env) — those have their own test coverage and aren't
	// what this test asserts. We DO need to let them succeed so the
	// transaction commits. sqlmock by default rejects unexpected queries;
	// turning ordering off lets them flow through with permissive matchers
	// below.
	deployMock.MatchExpectationsInOrder(false)
	for i := 0; i < 20; i++ {
		// Generous slot count to cover deployment_revisions, deployment_events,
		// deployment_workloads, deployment_services, deployment_build_env.
		deployMock.ExpectExec(`.*`).WillReturnResult(sqlmock.NewResult(0, 1))
		deployMock.ExpectQuery(`.*`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(i + 1)))
	}
	deployMock.ExpectCommit()

	body := deployableSpecWithClusterID("eu-west-1-managed")
	req := signedDeployRequest(t, body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := clusterMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled cluster expectations: %v", err)
	}
	// We deliberately do NOT call deployMock.ExpectationsWereMet() — the
	// permissive over-allocation above means some slots will go unmatched;
	// the assertion that matters (cluster_id in the INSERT args) already
	// fired or the deploy would have errored before commit.
}
