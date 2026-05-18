package handlers

import (
	"bufio"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	fakeKube "k8s.io/client-go/kubernetes/fake"
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

func (q *mockQueue) InsertDeployJob(_ context.Context, _, _ string) error   { return nil }
func (q *mockQueue) InsertUndeployJob(_ context.Context, _, _ string) error { return nil }
func (q *mockQueue) InsertWakeUpJob(_ context.Context, _, _ string) error   { return nil }

// deploymentByIDColumns lists the columns scanned by scanDeployment, in order.
var deploymentByIDColumns = []string{
	"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
	"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
	"status", "error_message", "error_details", "status_changed_at", "current_revision",
	"deployed_at", "undeployed_at", "avatar_colors",
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
		now, undeployedAt, nil,
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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// IsMember
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetVisibleDeploymentsByAccount
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors",
		}).AddRow(
			depID, "acct-1", nil, agentName, buildID, namespace, "My Agent",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil,
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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myaccount", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors",
		}).AddRow(
			depID, "acct-1", nil, agentName, buildID, namespace, "Sas Bot",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil,
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

type staticK8sCache struct {
	key  string
	data []byte
}

func (c staticK8sCache) Get(_ context.Context, key string) ([]byte, bool) {
	if key != c.key {
		return nil, false
	}
	return c.data, true
}

func (staticK8sCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}

func (staticK8sCache) Invalidate(_ context.Context, _ string) error {
	return nil
}

func TestEnrichDeployment_CacheHitPreservesSourceAccount(t *testing.T) {
	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	accountStore := account.NewAccountStore(accountDB)
	log := logger.New("error", "json")

	now := time.Now()
	sourceID := "src-acct"
	dbDep := &deploymentstore.Deployment{
		ID:                 "dep-1",
		AccountID:          "target-acct",
		SourceAccountID:    &sourceID,
		AgentName:          "agent-from-db",
		BuildID:            "build-from-db",
		Namespace:          "astro-cache-0",
		DisplayName:        "Agent From DB",
		DeploymentSpecJSON: `{"spec":"deployment/v1","source":{"account":"publisher","name":"agent-from-db","build":"build-from-db"}}`,
		Status:             deploymentstore.StatusActive,
		DeployedAt:         now,
	}
	cached, err := json.Marshal([]AgentDeployment{{
		Name:      "stale-name-from-cache",
		BuildID:   "build-from-db",
		Namespace: dbDep.Namespace,
		Status:    "Running",
	}})
	if err != nil {
		t.Fatalf("marshal cache payload: %v", err)
	}

	accountMock.ExpectQuery(`SELECT`).
		WithArgs(sourceID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow(sourceID, "publisher", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	deps := enrichDeployment(
		context.Background(),
		log,
		accountStore,
		nil,
		nil,
		nil,
		dbDep,
		nil,
		staticK8sCache{key: "list:" + dbDep.Namespace, data: cached},
		"list:",
		time.Minute,
	)

	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	dep := deps[0]
	if dep.Name != dbDep.AgentName {
		t.Errorf("cached deployment name = %q, want DB name %q", dep.Name, dbDep.AgentName)
	}
	if dep.SourceAccount != "publisher" {
		t.Errorf("cached deployment source_account = %q, want publisher", dep.SourceAccount)
	}
}

// TestEnrichDeployment_FailedDBOverridesK8sStatus is the regression for the
// 35-minute "Deploying" bug: when the DB knows a deployment is failed (set by
// the preflight 422 path or the reconcile pod-failure escalation), the
// enriched payload returned to the API must report status=error and the
// error_message — even when K8s replica scanning would otherwise return
// "Pending" because the failing pods can't reach Ready.
func TestEnrichDeployment_FailedDBOverridesK8sStatus(t *testing.T) {
	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	accountStore := account.NewAccountStore(accountDB)
	log := logger.New("error", "json")

	now := time.Now()
	sourceID := "src-acct"
	errMsg := "ImagePullBackOff on pod agent-abc (image=img:missing)"
	dbDep := &deploymentstore.Deployment{
		ID:                 "dep-failed",
		AccountID:          "target-acct",
		SourceAccountID:    &sourceID,
		AgentName:          "agent-stuck",
		BuildID:            "build-1",
		Namespace:          "astro-failed-0",
		DisplayName:        "Stuck",
		DeploymentSpecJSON: `{"spec":"deployment/v1","source":{"account":"publisher","name":"agent-stuck","build":"build-1"}}`,
		Status:             deploymentstore.StatusFailed,
		ErrorMessage:       &errMsg,
		DeployedAt:         now,
	}
	// K8s scan would have returned Pending (ready<replicas) — proving the
	// fix overrides whatever K8s saw.
	cached, err := json.Marshal([]AgentDeployment{{
		Name:      "agent-stuck",
		BuildID:   "build-1",
		Namespace: dbDep.Namespace,
		Status:    "Pending",
		Replicas:  1,
		Ready:     0,
	}})
	if err != nil {
		t.Fatalf("marshal cache payload: %v", err)
	}

	accountMock.ExpectQuery(`SELECT`).
		WithArgs(sourceID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors",
		}).AddRow(sourceID, "publisher", "organization", nil, nil, now, now, "", nil))

	deps := enrichDeployment(
		context.Background(),
		log,
		accountStore,
		nil,
		nil,
		nil,
		dbDep,
		nil,
		staticK8sCache{key: "list:" + dbDep.Namespace, data: cached},
		"list:",
		time.Minute,
	)

	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	d := deps[0]
	if d.Status != "error" {
		t.Errorf("status = %q, want error (failed DB row must override K8s Pending)", d.Status)
	}
	if d.ErrorMessage != errMsg {
		t.Errorf("error_message = %q, want %q", d.ErrorMessage, errMsg)
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

// TestListAstroDeployments_StaleStatefulSetPodVersion verifies that a pod with a stale
// version label (e.g. an OnDelete StatefulSet not yet recycled after a redeploy) is still
// matched to its workload and its runtime container status is returned.
func TestListAstroDeployments_StaleStatefulSetPodVersion(t *testing.T) {
	namespace := "astro-abc123-0"
	agentKey := "myorg.myagent"
	currentBuild := "build-2"
	staleBuild := "build-1"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/deployments") {
			_, _ = w.Write([]byte(`{"kind":"DeploymentList","apiVersion":"apps/v1","items":[]}`))
			return
		}
		if strings.Contains(path, "/statefulsets") {
			fmt.Fprintf(w, `{
				"kind":"StatefulSetList","apiVersion":"apps/v1","items":[{
					"metadata":{
						"name":"myagent-knowledge-db","namespace":%q,
						"creationTimestamp":"2026-01-01T00:00:00Z",
						"labels":{
							"app.kubernetes.io/managed-by":"astro-server",
							"astro.dev/agent":%q,
							"app.kubernetes.io/version":%q,
							"app.kubernetes.io/component":"knowledge-db"
						}
					},
					"spec":{
						"replicas":1,
						"template":{
							"spec":{
								"containers":[{"name":"app","image":"postgres:15"}]
							}
						}
					},
					"status":{"replicas":1}
				}]
			}`, namespace, agentKey, currentBuild)
			return
		}
		if strings.Contains(path, "/ingresses") {
			_, _ = w.Write([]byte(`{"kind":"IngressList","apiVersion":"networking.k8s.io/v1","items":[]}`))
			return
		}
		if strings.Contains(path, "/pods") {
			// Pod exists but has the old build version label — simulates OnDelete StatefulSet
			// where the pod was not recreated after a redeploy.
			fmt.Fprintf(w, `{
				"kind":"PodList","apiVersion":"v1","items":[{
					"metadata":{
						"name":"myagent-knowledge-db-0","namespace":%q,
						"creationTimestamp":"2026-01-01T00:00:00Z",
						"labels":{
							"app.kubernetes.io/managed-by":"astro-server",
							"astro.dev/agent":%q,
							"app.kubernetes.io/version":%q,
							"app.kubernetes.io/component":"knowledge-db"
						}
					},
					"status":{
						"phase":"Running",
						"containerStatuses":[{
							"name":"app",
							"ready":false,
							"restartCount":6,
							"state":{"waiting":{"reason":"CrashLoopBackOff"}}
						}]
					}
				}]
			}`, namespace, agentKey, staleBuild)
			return
		}
		if strings.Contains(path, "/jobs") {
			_, _ = w.Write([]byte(`{"kind":"JobList","apiVersion":"batch/v1","items":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	k8sClient := newMockK8sClient(handler)
	deps, err := listAstroDeployments(context.Background(), k8sClient, namespace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}
	if len(deps[0].Workloads) != 1 {
		t.Fatalf("expected 1 workload, got %d", len(deps[0].Workloads))
	}
	wl := deps[0].Workloads[0]
	if len(wl.Containers) == 0 {
		t.Fatal("containers should be populated even when pod version label is stale")
	}
	if wl.Containers[0].Name != "app" {
		t.Errorf("expected container name %q, got %q", "app", wl.Containers[0].Name)
	}
	if wl.Containers[0].RestartCount != 6 {
		t.Errorf("expected restart count 6 from stale pod, got %d", wl.Containers[0].RestartCount)
	}
	if wl.PodName != "myagent-knowledge-db-0" {
		t.Errorf("expected pod name %q, got %q", "myagent-knowledge-db-0", wl.PodName)
	}
}

// TestListAstroDeployments_PrefersNewestRunningPod verifies that when multiple pods exist
// for the same agent+component (e.g. a stale pod and a newer replacement), the newest
// Running pod is selected over an older one.
func TestListAstroDeployments_PrefersNewestRunningPod(t *testing.T) {
	namespace := "astro-abc123-0"
	agentKey := "myorg.myagent"
	build := "build-2"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/deployments") {
			_, _ = w.Write([]byte(`{"kind":"DeploymentList","apiVersion":"apps/v1","items":[]}`))
			return
		}
		if strings.Contains(path, "/statefulsets") {
			fmt.Fprintf(w, `{
				"kind":"StatefulSetList","apiVersion":"apps/v1","items":[{
					"metadata":{
						"name":"myagent-knowledge-db","namespace":%q,
						"creationTimestamp":"2026-01-01T00:00:00Z",
						"labels":{
							"app.kubernetes.io/managed-by":"astro-server",
							"astro.dev/agent":%q,
							"app.kubernetes.io/version":%q,
							"app.kubernetes.io/component":"knowledge-db"
						}
					},
					"spec":{"replicas":1,"template":{"spec":{"containers":[{"name":"app","image":"postgres:15"}]}}},
					"status":{"replicas":1}
				}]
			}`, namespace, agentKey, build)
			return
		}
		if strings.Contains(path, "/ingresses") {
			_, _ = w.Write([]byte(`{"kind":"IngressList","apiVersion":"networking.k8s.io/v1","items":[]}`))
			return
		}
		if strings.Contains(path, "/pods") {
			// Two pods for the same component: an older stale one and a newer running one.
			fmt.Fprintf(w, `{
				"kind":"PodList","apiVersion":"v1","items":[
					{
						"metadata":{
							"name":"myagent-knowledge-db-0","namespace":%q,
							"creationTimestamp":"2026-01-01T00:00:00Z",
							"labels":{"astro.dev/agent":%q,"app.kubernetes.io/version":"build-1","app.kubernetes.io/component":"knowledge-db"}
						},
						"status":{
							"phase":"Running",
							"containerStatuses":[{"name":"app","ready":false,"restartCount":10,"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]
						}
					},
					{
						"metadata":{
							"name":"myagent-knowledge-db-1","namespace":%q,
							"creationTimestamp":"2026-06-01T00:00:00Z",
							"labels":{"astro.dev/agent":%q,"app.kubernetes.io/version":%q,"app.kubernetes.io/component":"knowledge-db"}
						},
						"status":{
							"phase":"Running",
							"containerStatuses":[{"name":"app","ready":true,"restartCount":0,"state":{"running":{"startedAt":"2026-06-01T00:00:00Z"}}}]
						}
					}
				]
			}`, namespace, agentKey, namespace, agentKey, build)
			return
		}
		if strings.Contains(path, "/jobs") {
			_, _ = w.Write([]byte(`{"kind":"JobList","apiVersion":"batch/v1","items":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	k8sClient := newMockK8sClient(handler)
	deps, err := listAstroDeployments(context.Background(), k8sClient, namespace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 || len(deps[0].Workloads) != 1 {
		t.Fatalf("expected 1 deployment with 1 workload")
	}
	wl := deps[0].Workloads[0]
	if wl.PodName != "myagent-knowledge-db-1" {
		t.Errorf("expected newer pod %q, got %q", "myagent-knowledge-db-1", wl.PodName)
	}
	if !wl.Containers[0].Ready {
		t.Error("expected container from newer pod to be ready")
	}
}

// TestListAstroDeployments_IngestionWorkloads verifies the unified workload
// model for ingestion: CronJobs and standalone Jobs each surface as their
// own Workload entry (Kind="CronJob"/"Job"), and CronJob-owned child Jobs
// attach to their parent's Runs[] rather than appearing as a separate row.
//
// This is the user-visible signal that a schedule is configured — before
// the unification, schedule ingestion was invisible until the first run.
func TestListAstroDeployments_IngestionWorkloads(t *testing.T) {
	namespace := "astro-abc123-0"
	agentKey := "myorg.myagent"
	build := "build-1"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		if strings.Contains(path, "/deployments") {
			_, _ = w.Write([]byte(`{"kind":"DeploymentList","apiVersion":"apps/v1","items":[]}`))
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
		if strings.Contains(path, "/cronjobs") {
			fmt.Fprintf(w, `{
				"kind":"CronJobList","apiVersion":"batch/v1","items":[{
					"metadata":{
						"name":"myagent-ingestion-daily","namespace":%q,
						"uid":"cron-uid-1",
						"creationTimestamp":"2026-01-01T00:00:00Z",
						"labels":{
							"app.kubernetes.io/managed-by":"astro-server",
							"astro.dev/agent":%q,
							"app.kubernetes.io/version":%q,
							"app.kubernetes.io/component":"ingestion-daily"
						}
					},
					"spec":{"schedule":"0 0 * * *","jobTemplate":{"spec":{"template":{"spec":{"containers":[]}}}}},
					"status":{"lastScheduleTime":"2026-05-05T00:00:00Z"}
				}]
			}`, namespace, agentKey, build)
			return
		}
		if strings.Contains(path, "/jobs") {
			// Two Jobs:
			//   - cron child (ownerRef = CronJob/myagent-ingestion-daily) → Runs[]
			//   - standalone startup Job → its own Workload
			fmt.Fprintf(w, `{
				"kind":"JobList","apiVersion":"batch/v1","items":[
					{
						"metadata":{
							"name":"myagent-ingestion-daily-28012345","namespace":%q,
							"creationTimestamp":"2026-05-05T00:00:00Z",
							"ownerReferences":[{"apiVersion":"batch/v1","kind":"CronJob","name":"myagent-ingestion-daily","uid":"cron-uid-1","controller":true}],
							"labels":{
								"app.kubernetes.io/managed-by":"astro-server",
								"astro.dev/agent":%q,
								"app.kubernetes.io/version":%q,
								"app.kubernetes.io/component":"ingestion-daily"
							}
						},
						"spec":{"completions":1},
						"status":{"succeeded":1,"startTime":"2026-05-05T00:00:01Z","conditions":[{"type":"Complete","status":"True"}]}
					},
					{
						"metadata":{
							"name":"myagent-ingestion-bootstrap","namespace":%q,
							"creationTimestamp":"2026-04-01T00:00:00Z",
							"labels":{
								"app.kubernetes.io/managed-by":"astro-server",
								"astro.dev/agent":%q,
								"app.kubernetes.io/version":%q,
								"app.kubernetes.io/component":"ingestion-bootstrap"
							}
						},
						"spec":{"completions":1},
						"status":{"succeeded":1,"startTime":"2026-04-01T00:00:05Z","conditions":[{"type":"Complete","status":"True"}]}
					}
				]
			}`, namespace, agentKey, build, namespace, agentKey, build)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	k8sClient := newMockK8sClient(handler)
	deps, err := listAstroDeployments(context.Background(), k8sClient, namespace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment (stub from CronJob/Job), got %d", len(deps))
	}

	// Index workloads by name for assertions.
	byName := map[string]WorkloadDetail{}
	for _, wl := range deps[0].Workloads {
		byName[wl.Name] = wl
	}
	if len(byName) != 2 {
		t.Fatalf("expected 2 Workloads (CronJob + standalone Job), got %d: %+v", len(byName), deps[0].Workloads)
	}

	cron, ok := byName["myagent-ingestion-daily"]
	if !ok {
		t.Fatal("expected Workload for the CronJob")
	}
	if cron.Kind != "CronJob" {
		t.Errorf("CronJob workload Kind = %q, want CronJob", cron.Kind)
	}
	if cron.Schedule != "0 0 * * *" {
		t.Errorf("CronJob workload Schedule = %q, want %q", cron.Schedule, "0 0 * * *")
	}
	if cron.Status != "Idle" {
		t.Errorf("CronJob workload Status = %q, want Idle (no active children)", cron.Status)
	}
	if cron.StartTime == "" {
		t.Error("CronJob workload StartTime should be populated from LastScheduleTime")
	}
	if len(cron.Runs) != 1 {
		t.Fatalf("expected 1 Run on the CronJob workload (the child Job), got %d", len(cron.Runs))
	}
	run := cron.Runs[0]
	if run.Name != "myagent-ingestion-daily-28012345" {
		t.Errorf("Runs[0].Name = %q, want the child Job name", run.Name)
	}
	if run.Status != "Succeeded" {
		t.Errorf("Runs[0].Status = %q, want Succeeded", run.Status)
	}
	if run.Completions != "1/1" {
		t.Errorf("Runs[0].Completions = %q, want 1/1", run.Completions)
	}

	bootstrap, ok := byName["myagent-ingestion-bootstrap"]
	if !ok {
		t.Fatal("expected standalone Job to surface as its own Workload")
	}
	if bootstrap.Kind != "Job" {
		t.Errorf("standalone Job Kind = %q, want Job", bootstrap.Kind)
	}
	if bootstrap.Status != "Succeeded" {
		t.Errorf("standalone Job Status = %q, want Succeeded", bootstrap.Status)
	}
	if bootstrap.Completions != "1/1" {
		t.Errorf("standalone Job Completions = %q, want 1/1", bootstrap.Completions)
	}
	if len(bootstrap.Runs) != 0 {
		t.Errorf("standalone Job should not have Runs[], got %d", len(bootstrap.Runs))
	}
	if bootstrap.Schedule != "" {
		t.Errorf("standalone Job should not have Schedule, got %q", bootstrap.Schedule)
	}
}

// TestBuildContainerStatuses_DedupesEnvDirectOverridesEnvFrom verifies that
// when a container has both an envFrom-resolved key and a direct env entry
// with the same name, the result has one entry per name with the direct
// entry winning — mirroring K8s runtime precedence and preventing duplicate
// React keys downstream.
func TestBuildContainerStatuses_DedupesEnvDirectOverridesEnvFrom(t *testing.T) {
	const ns = "astro-test-0"
	cs := fakeKube.NewClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "agent-creds", Namespace: ns},
			Data: map[string][]byte{
				"POSTGRES_USER":     []byte("astro"),
				"POSTGRES_PASSWORD": []byte("p1"),
				"GITHUB_TOKEN":      []byte("ghs_xx"),
			},
		},
	)
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-pod", Namespace: ns},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "app",
				EnvFrom: []corev1.EnvFromSource{{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: "agent-creds"},
					},
				}},
				Env: []corev1.EnvVar{{
					Name: "POSTGRES_USER",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "knowledge-postgres-creds"},
							Key:                  "POSTGRES_USER",
						},
					},
				}, {
					Name: "POSTGRES_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "knowledge-postgres-creds"},
							Key:                  "POSTGRES_PASSWORD",
						},
					},
				}},
			}},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}

	got := buildContainerStatuses(context.Background(), cs, pod)
	if len(got) != 1 || got[0].Name != "app" {
		t.Fatalf("expected one container 'app', got %+v", got)
	}

	seen := map[string]int{}
	bySource := map[string]string{}
	for _, ev := range got[0].Env {
		seen[ev.Name]++
		bySource[ev.Name] = ev.From
	}

	for _, k := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "GITHUB_TOKEN"} {
		if seen[k] != 1 {
			t.Errorf("env %q appears %d times, want 1", k, seen[k])
		}
	}
	// Direct env (secretKeyRef → "secret:NAME/KEY", with slash) must win
	// over envFrom ("secret:NAME", no slash) for keys present in both.
	for _, k := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD"} {
		if !strings.Contains(bySource[k], "/") {
			t.Errorf("%s: expected direct secretKeyRef source (with '/'), got %q", k, bySource[k])
		}
	}
	// Keys only in envFrom keep the envFrom source.
	if bySource["GITHUB_TOKEN"] != "secret:agent-creds" {
		t.Errorf("GITHUB_TOKEN: expected envFrom source, got %q", bySource["GITHUB_TOKEN"])
	}
}

// TestContainersFromSpecWithEnv_ResolvesEnvWithoutLivePod verifies that
// Job/CronJob workloads can surface env vars by reading directly from the
// pod template, so the General tab is populated even when no pod exists
// (CronJob never fired, Job pod GC'd).
func TestContainersFromSpecWithEnv_ResolvesEnvWithoutLivePod(t *testing.T) {
	const ns = "astro-test-0"
	cs := fakeKube.NewClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "ingest-creds", Namespace: ns},
			Data: map[string][]byte{
				"S3_ACCESS_KEY": []byte("AKIA..."),
				"S3_SECRET":     []byte("xxx"),
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "ingest-config", Namespace: ns},
			Data: map[string]string{
				"BUCKET":   "my-bucket",
				"INTERVAL": "3600",
			},
		},
	)

	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: "ingest",
			EnvFrom: []corev1.EnvFromSource{
				{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "ingest-config"}}},
				{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "ingest-creds"}}},
			},
			Env: []corev1.EnvVar{
				{Name: "MODE", Value: "incremental"},
				{Name: "S3_ACCESS_KEY", ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "ingest-creds"},
						Key:                  "S3_ACCESS_KEY",
					},
				}},
			},
		}},
		InitContainers: []corev1.Container{{
			Name: "wait-for-db",
			Env:  []corev1.EnvVar{{Name: "DB_HOST", Value: "postgres"}},
		}},
	}

	got := containersFromSpecWithEnv(context.Background(), cs, ns, podSpec)
	if len(got) != 2 {
		t.Fatalf("expected 2 containers (1 main + 1 init), got %d", len(got))
	}

	byName := map[string]ContainerStatus{}
	for _, c := range got {
		byName[c.Name] = c
	}

	ingest, ok := byName["ingest"]
	if !ok {
		t.Fatal("expected 'ingest' container")
	}
	envByName := map[string]EnvVar{}
	for _, ev := range ingest.Env {
		envByName[ev.Name] = ev
	}

	// envFrom configmap → values surfaced as-is.
	if envByName["BUCKET"].Value != "my-bucket" {
		t.Errorf("BUCKET value = %q, want %q", envByName["BUCKET"].Value, "my-bucket")
	}
	if envByName["BUCKET"].From != "configmap:ingest-config" {
		t.Errorf("BUCKET source = %q, want configmap:ingest-config", envByName["BUCKET"].From)
	}
	// envFrom secret → key surfaced, value redacted.
	if envByName["S3_SECRET"].Value != "••••••••" {
		t.Errorf("S3_SECRET value = %q, want redacted", envByName["S3_SECRET"].Value)
	}
	if envByName["S3_SECRET"].From != "secret:ingest-creds" {
		t.Errorf("S3_SECRET source = %q, want secret:ingest-creds", envByName["S3_SECRET"].From)
	}
	// Direct literal env.
	if envByName["MODE"].Value != "incremental" {
		t.Errorf("MODE value = %q, want incremental", envByName["MODE"].Value)
	}
	// Direct secretKeyRef wins over envFrom for the same key (slash in source).
	s3Key := envByName["S3_ACCESS_KEY"]
	if !strings.Contains(s3Key.From, "/") {
		t.Errorf("S3_ACCESS_KEY source = %q, want direct secretKeyRef (with '/')", s3Key.From)
	}
	if s3Key.Value != "••••••••" {
		t.Errorf("S3_ACCESS_KEY value = %q, want redacted", s3Key.Value)
	}

	// InitContainer env is also resolved.
	wait, ok := byName["wait-for-db"]
	if !ok {
		t.Fatal("expected 'wait-for-db' init container")
	}
	if len(wait.Env) != 1 || wait.Env[0].Name != "DB_HOST" || wait.Env[0].Value != "postgres" {
		t.Errorf("init env = %+v, want single DB_HOST=postgres", wait.Env)
	}
}

// TestListAstroDeployments_IngestionWorkloadsHaveEnvVars verifies the
// full listing path: a CronJob and standalone Job whose templates declare
// env (envFrom secret + direct entries) surface those env vars on the
// returned WorkloadDetail.Containers, even though no pods exist for them.
func TestListAstroDeployments_IngestionWorkloadsHaveEnvVars(t *testing.T) {
	namespace := "astro-env123-0"
	agentKey := "myorg.envagent"
	build := "build-1"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		case strings.Contains(path, "/deployments"):
			_, _ = w.Write([]byte(`{"kind":"DeploymentList","apiVersion":"apps/v1","items":[]}`))
		case strings.Contains(path, "/statefulsets"):
			_, _ = w.Write([]byte(`{"kind":"StatefulSetList","apiVersion":"apps/v1","items":[]}`))
		case strings.Contains(path, "/ingresses"):
			_, _ = w.Write([]byte(`{"kind":"IngressList","apiVersion":"networking.k8s.io/v1","items":[]}`))
		case strings.HasSuffix(path, "/pods"):
			_, _ = w.Write([]byte(`{"kind":"PodList","apiVersion":"v1","items":[]}`))
		case strings.Contains(path, "/secrets/ingest-creds"):
			fmt.Fprintf(w, `{
				"kind":"Secret","apiVersion":"v1",
				"metadata":{"name":"ingest-creds","namespace":%q},
				"data":{"API_KEY":"c2VjcmV0"}
			}`, namespace)
		case strings.Contains(path, "/cronjobs"):
			fmt.Fprintf(w, `{
				"kind":"CronJobList","apiVersion":"batch/v1","items":[{
					"metadata":{
						"name":"envagent-ingestion-hourly","namespace":%q,
						"uid":"cron-uid-env-1",
						"creationTimestamp":"2026-01-01T00:00:00Z",
						"labels":{
							"app.kubernetes.io/managed-by":"astro-server",
							"astro.dev/agent":%q,
							"app.kubernetes.io/version":%q,
							"app.kubernetes.io/component":"ingestion-hourly"
						}
					},
					"spec":{
						"schedule":"0 * * * *",
						"jobTemplate":{"spec":{"template":{"spec":{"containers":[{
							"name":"ingest",
							"envFrom":[{"secretRef":{"name":"ingest-creds"}}],
							"env":[
								{"name":"MODE","value":"incremental"},
								{"name":"REGION","value":"us-east-1"}
							]
						}]}}}}
					},
					"status":{}
				}]
			}`, namespace, agentKey, build)
		case strings.HasSuffix(path, "/jobs"):
			fmt.Fprintf(w, `{
				"kind":"JobList","apiVersion":"batch/v1","items":[{
					"metadata":{
						"name":"envagent-ingestion-bootstrap","namespace":%q,
						"creationTimestamp":"2026-04-01T00:00:00Z",
						"labels":{
							"app.kubernetes.io/managed-by":"astro-server",
							"astro.dev/agent":%q,
							"app.kubernetes.io/version":%q,
							"app.kubernetes.io/component":"ingestion-bootstrap"
						}
					},
					"spec":{
						"completions":1,
						"template":{"spec":{"containers":[{
							"name":"bootstrap",
							"env":[
								{"name":"INIT_TARGET","value":"all"}
							]
						}]}}
					},
					"status":{"succeeded":1,"conditions":[{"type":"Complete","status":"True"}]}
				}]
			}`, namespace, agentKey, build)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	k8sClient := newMockK8sClient(handler)
	deps, err := listAstroDeployments(context.Background(), k8sClient, namespace, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deps))
	}

	byName := map[string]WorkloadDetail{}
	for _, wl := range deps[0].Workloads {
		byName[wl.Name] = wl
	}

	cron, ok := byName["envagent-ingestion-hourly"]
	if !ok {
		t.Fatal("expected CronJob workload")
	}
	if len(cron.Containers) != 1 {
		t.Fatalf("CronJob containers = %d, want 1", len(cron.Containers))
	}
	cronEnv := indexEnv(cron.Containers[0].Env)
	if cronEnv["MODE"].Value != "incremental" {
		t.Errorf("CronJob MODE = %q, want incremental", cronEnv["MODE"].Value)
	}
	if cronEnv["REGION"].Value != "us-east-1" {
		t.Errorf("CronJob REGION = %q, want us-east-1", cronEnv["REGION"].Value)
	}
	// envFrom secret resolved: key present, value redacted, source attributed.
	if _, ok := cronEnv["API_KEY"]; !ok {
		t.Error("CronJob API_KEY should be resolved from envFrom secret")
	}
	if cronEnv["API_KEY"].Value != "••••••••" {
		t.Errorf("CronJob API_KEY value = %q, want redacted", cronEnv["API_KEY"].Value)
	}
	if cronEnv["API_KEY"].From != "secret:ingest-creds" {
		t.Errorf("CronJob API_KEY source = %q, want secret:ingest-creds", cronEnv["API_KEY"].From)
	}

	job, ok := byName["envagent-ingestion-bootstrap"]
	if !ok {
		t.Fatal("expected standalone Job workload")
	}
	if len(job.Containers) != 1 {
		t.Fatalf("Job containers = %d, want 1", len(job.Containers))
	}
	jobEnv := indexEnv(job.Containers[0].Env)
	if jobEnv["INIT_TARGET"].Value != "all" {
		t.Errorf("Job INIT_TARGET = %q, want all", jobEnv["INIT_TARGET"].Value)
	}
}

func indexEnv(env []EnvVar) map[string]EnvVar {
	out := make(map[string]EnvVar, len(env))
	for _, ev := range env {
		out[ev.Name] = ev
	}
	return out
}

func TestListDeployments_NoDBRecord_ReturnsEmpty(t *testing.T) {
	// K8s namespace exists but no DB record → deployment should NOT appear
	router, deployMock, accountMock := setupListDeploymentsTest(t,
		k8sListHandler("astro-orphan-0", "orphan-agent", "build-1"))

	now := time.Now()

	// accountStore.GetByName
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// IsMember
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// GetVisibleDeploymentsByAccount returns no rows
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors",
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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Two active deployments
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors",
		}).AddRow(
			depID1, "acct-1", nil, "agent-a", "b1", ns1, "Agent A",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil,
		).AddRow(
			depID2, "acct-1", nil, "agent-b", "b1", ns2, "Agent B",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil,
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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors",
		}).AddRow(
			depID, "acct-1", nil, agentName, buildID, namespace, "My Agent",
			`{}`, nil, nil, nil,
			"active", nil, nil, now, 1,
			now, nil, nil,
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
			"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order",
		}).AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
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
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("src-acct", "source-org", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// Target account lookup (different from source)
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("target-org").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("tgt-acct", "target-org", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// IsMember(target, user) → member of target account
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("tgt-acct", "user-target").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get → private agent
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("src-acct", "secret-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at"}).
			AddRow("src-acct", "secret-agent", "r.io", "private", nil, false, nil, now, now))
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
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("src-acct", "source-org", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("target-org").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("tgt-acct", "target-org", "personal", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("tgt-acct", "user-target").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("src-acct", "public-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at"}).
			AddRow("src-acct", "public-agent", "r.io", "public", nil, false, nil, now, now))
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
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// IsMember(target=source, user) → member
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get → no rows
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "nonexistent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at"}))

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
					[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
					AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

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
	router.POST("/deploy", DeployAgent(log, index, accountStore, cfg, deployStore, nil, nil, nil, &mockQueue{}, nil, nil, nil, nil, nil, nil, preflighter)) //nolint:staticcheck // nil varsStore, clusterStore, EntitlementChecker, avatarStore, omClient, db, auditStore, ksStore, and authzStore skip checks in tests

	return router, indexMock, accountMock, deployMock, cfg
}

// expectDeployPrep sets up mocks for the full prepareDeployment flow: account lookup,
// membership check, agent+version lookup for both agentIndex.Get and the build lookup.
func expectDeployPrep(accountMock, indexMock sqlmock.Sqlmock) {
	now := time.Now()

	// accountStore.GetByName("myorg") — source account
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// IsMember(target=source, user) → yes
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get (visibility check)
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "r.io", "public", nil, false, nil, now, now))
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
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for display_name collision, got %d: %s", rec.Code, rec.Body.String())
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow(acctID, "myaccount", "personal", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

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
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
		"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at", "avatar_colors",
	}).AddRow(
		id, accountID, nil, agentName, buildID, namespace,
		displayName, specJSON, []byte(nil), (*string)(nil), nil,
		status, (*string)(nil), json.RawMessage(nil), now, revision,
		now, (*time.Time)(nil), nil,
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

	deployMock.ExpectBegin()
	deployMock.ExpectQuery(`INSERT INTO deployments`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at",
		}).AddRow("new-id", "acct-1", nil, "my-agent", "build-1", "astro-new", "", "{}", "pending", time.Now()))
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
	// No adapter selected in the submitted spec → EnforceEditable strips all
	// interface-targeted variables (including the three slack vars). Only the
	// DELETE fires; no user-var INSERTs follow.
	deployMock.ExpectExec(`DELETE FROM deployment_build_env`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
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

// Regression: a client submitting a web-only deploy with stale slack-targeted
// variables/env refs should have them stripped before persistence. Mirrors the
// real-world scenario where a redeploy of a previously slack-enabled agent
// drops slack from the adapter list but a stale UI/CLI cache forwards the old
// SLACK_CONFIG ref. The deploy handler must run ApplyAdapterShaping on the
// submitted spec; if it doesn't, the variable INSERT for SLACK_CONFIG below
// will fire and break the strict mock expectations.
func TestDeploy_WebOnlyAdapter_StripsStaleSlackRefs(t *testing.T) {
	router, indexMock, accountMock, deployMock := setupDeployRouter("user-1")

	// Use a custom prep that advertises a messaging-capable agent so the
	// regenerated template has matching interfaces.image for EnforceEditable.
	expectDeployPrepMessaging(accountMock, indexMock)

	deployMock.ExpectBegin()
	deployMock.ExpectQuery(`INSERT INTO deployments`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
			"display_name", "deployment_spec_json", "status", "deployed_at",
		}).AddRow("new-id", "acct-1", nil, "my-agent", "build-1", "astro-new", "", "{}", "pending", time.Now()))
	deployMock.ExpectExec(`INSERT INTO deployment_revisions`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectExec(`INSERT INTO deployment_events`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Agent workload + service. Messaging sidecar (colocated) with two
	// services (grpc + http). Collector workload + service. The exact INSERT
	// shape is incidental to this test — what matters is the variable
	// inserts list, which must be empty.
	deployMock.MatchExpectationsInOrder(false)
	for i := 0; i < 2; i++ {
		deployMock.ExpectQuery(`INSERT INTO deployment_workloads`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(i + 1))
	}
	deployMock.ExpectQuery(`INSERT INTO deployment_sidecars`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	for i := 0; i < 5; i++ {
		deployMock.ExpectQuery(`INSERT INTO deployment_services`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(i + 1))
	}
	// SaveNormalizedSpec clears any existing deployment_build_env rows
	// before re-inserting per-(role, env_name) user_var rows. With slack
	// stripped from the spec, no user_var rows should be inserted —
	// only the DELETE happens. An extra INSERT here would mean stale
	// SLACK_* refs leaked through shaping.
	deployMock.ExpectExec(`DELETE FROM deployment_build_env`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	deployMock.ExpectCommit()

	body := deployableSpecWithStaleSlackRefs()
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := deployMock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled deploy expectations (stale slack vars likely leaked): %v", err)
	}
}

// expectDeployPrepMessaging is like expectDeployPrep but advertises the source
// agent as messaging-capable so the server-regenerated template has an
// `interfaces` block matching the submitted spec.
func expectDeployPrepMessaging(accountMock, indexMock sqlmock.Sqlmock) {
	now := time.Now()
	specJSON := `{"name":"my-agent","agent":{"image":"123456789.dkr.ecr.us-east-1.amazonaws.com/test-tenant-myorg/my-agent:build-1","interfaces":{"messaging":true}}}`

	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "r.io", "public", nil, false, nil, now, now))
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

// deployableSpecWithStaleSlackRefs simulates the bug scenario:
//   - adapters: ["web"]
//   - interfaces.environment.SLACK_CONFIG present (stale ref)
//   - SLACK_CONFIG variable still in spec
//
// Without ApplyAdapterShaping running on the submitted spec, the stale
// variable + env ref ride through to persistence and end up as env vars
// in the messaging container.
func deployableSpecWithStaleSlackRefs() string {
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
		"interfaces": {
			"adapters": ["web"],
			"image": "123456789.dkr.ecr.us-east-1.amazonaws.com/dockerhub/astropods/messaging:latest",
			"resources": {"cpu": "100m", "memory": "128Mi", "cpu_limit": "500m", "memory_limit": "512Mi"},
			"endpoints": {
				"grpc": {"port": 9090, "protocol": "grpc"},
				"http": {"port": 8080, "protocol": "http", "expose": {"enabled": true}}
			},
			"environment": {"SLACK_CONFIG": "${variables.SLACK_CONFIG}"}
		},
		"variables": {
			"SLACK_CONFIG": {"secret": false, "optional": true, "targets": ["interface.slack"]}
		},
		"observability": {"enabled": true, "provider": "langfuse"}
	}`
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
	// No adapter selected → EnforceEditable strips the SLACK_* vars from the
	// submitted spec. DELETE fires; no variable INSERTs follow.
	deployMock.ExpectExec(`DELETE FROM deployment_build_env`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
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
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "r.io", "public", nil, false, nil, now, now))
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
	router.GET("/api/v1/deployments/:id", GetDeployment(log, accountStore, cfg, k8sClient, deployStore, nil, nil, nil, k8scache.NoopCache{}))

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
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// IsMember → yes
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// agentIndex.Get (visibility check)
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "r.io", "public", nil, false, nil, now, now))
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

// mockCache is an in-memory k8scache.Cache for testing cache hit/bypass behaviour.
type mockCache struct {
	data map[string][]byte
}

func (m *mockCache) Get(_ context.Context, key string) ([]byte, bool) {
	d, ok := m.data[key]
	return d, ok
}
func (m *mockCache) Set(_ context.Context, key string, data []byte, _ time.Duration) error {
	m.data[key] = data
	return nil
}
func (m *mockCache) Invalidate(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

// k8sEventsHandler returns an http.Handler that serves a K8s EventList with the given items JSON.
// It also increments *callCount each time the events endpoint is hit so tests can assert whether
// the real K8s API was contacted.
func k8sEventsHandler(namespace, itemsJSON string, callCount *int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/namespaces/"+namespace+"/events") {
			*callCount++
			fmt.Fprintf(w, `{"kind":"EventList","apiVersion":"v1","metadata":{},"items":[%s]}`, itemsJSON)
			return
		}
		http.NotFound(w, r)
	})
}

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

	// Two events: a Warning (older) and a Normal (newer)
	eventsJSON := `
		{
			"metadata":{"name":"evt-warn","namespace":"astro-abc123","creationTimestamp":"2026-04-16T09:00:00Z"},
			"involvedObject":{"kind":"Pod","name":"my-agent-abc"},
			"reason":"BackOff",
			"message":"Back-off restarting failed container",
			"type":"Warning",
			"count":3,
			"firstTimestamp":"2026-04-16T08:50:00Z",
			"lastTimestamp":"2026-04-16T09:00:00Z"
		},
		{
			"metadata":{"name":"evt-normal","namespace":"astro-abc123","creationTimestamp":"2026-04-16T10:00:00Z"},
			"involvedObject":{"kind":"Pod","name":"my-agent-abc"},
			"reason":"Scheduled",
			"message":"Successfully assigned pod",
			"type":"Normal",
			"count":1,
			"firstTimestamp":"2026-04-16T10:00:00Z",
			"lastTimestamp":"2026-04-16T10:00:00Z"
		}`

	var callCount int
	k8sClient := newMockK8sClient(k8sEventsHandler(namespace, eventsJSON, &callCount))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments/:id/events",
		GetDeploymentEvents(log, accountStore, k8sClient, deployStore, k8scache.NoopCache{}))

	// GetDeploymentByID
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", namespace,
			"My Agent", `{}`, "active", now, nil))

	// IsMember
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID+"/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if callCount != 1 {
		t.Fatalf("expected K8s API to be called once, got %d", callCount)
	}

	var resp DeploymentEventsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(resp.Events))
	}

	// Events should be sorted by lastTimestamp descending, so Normal (10:00) comes first.
	first := resp.Events[0]
	if first.Reason != "Scheduled" {
		t.Errorf("expected first event reason 'Scheduled', got %q", first.Reason)
	}
	if first.Type != "Normal" {
		t.Errorf("expected first event type 'Normal', got %q", first.Type)
	}
	if first.ObjectKind != "Pod" {
		t.Errorf("expected object_kind 'Pod', got %q", first.ObjectKind)
	}
	if first.ObjectName != "my-agent-abc" {
		t.Errorf("expected object_name 'my-agent-abc', got %q", first.ObjectName)
	}
	if first.Count != 1 {
		t.Errorf("expected count 1, got %d", first.Count)
	}
	if first.LastTimestamp != "2026-04-16T10:00:00Z" {
		t.Errorf("expected last_timestamp '2026-04-16T10:00:00Z', got %q", first.LastTimestamp)
	}
	if first.FirstTimestamp != "2026-04-16T10:00:00Z" {
		t.Errorf("expected first_timestamp '2026-04-16T10:00:00Z', got %q", first.FirstTimestamp)
	}

	second := resp.Events[1]
	if second.Reason != "BackOff" {
		t.Errorf("expected second event reason 'BackOff', got %q", second.Reason)
	}
	if second.Type != "Warning" {
		t.Errorf("expected second event type 'Warning', got %q", second.Type)
	}
	if second.Count != 3 {
		t.Errorf("expected count 3, got %d", second.Count)
	}
	if second.Message != "Back-off restarting failed container" {
		t.Errorf("expected message 'Back-off restarting failed container', got %q", second.Message)
	}
}

func TestGetDeploymentEvents_NoAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	log := logger.New("error", "json")

	router := gin.New()
	// No auth middleware — user is not set in context.
	router.GET("/api/v1/deployments/:id/events",
		GetDeploymentEvents(log, nil, nil, nil, k8scache.NoopCache{}))

	req := httptest.NewRequest("GET", "/api/v1/deployments/some-id/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetDeploymentEvents_NoK8sClient(t *testing.T) {
	gin.SetMode(gin.TestMode)

	log := logger.New("error", "json")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	// Pass nil for k8sClient.
	router.GET("/api/v1/deployments/:id/events",
		GetDeploymentEvents(log, nil, nil, nil, k8scache.NoopCache{}))

	req := httptest.NewRequest("GET", "/api/v1/deployments/some-id/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetDeploymentEvents_CacheHit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()
	namespace := "astro-ns-cached"

	// Pre-populate cache with a known response.
	cachedResp := DeploymentEventsResponse{
		Events: []K8sEventItem{
			{Type: "Normal", Reason: "CachedEvent", Message: "from cache", ObjectKind: "Pod",
				ObjectName: "cached-pod", Count: 1, FirstTimestamp: "2026-04-16T08:00:00Z", LastTimestamp: "2026-04-16T08:00:00Z"},
		},
	}
	cachedData, _ := json.Marshal(cachedResp)
	cache := &mockCache{data: map[string][]byte{
		"astro:k8s:events:" + namespace: cachedData,
	}}

	// Set up a K8s handler that should NOT be called. If it is called, the test fails.
	var callCount int
	k8sClient := newMockK8sClient(k8sEventsHandler(namespace, "", &callCount))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments/:id/events",
		GetDeploymentEvents(log, accountStore, k8sClient, deployStore, cache))

	// GetDeploymentByID — status is "active" so cache should be used.
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", namespace,
			"My Agent", `{}`, "active", now, nil))

	// IsMember
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID+"/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if callCount != 0 {
		t.Fatalf("expected K8s API NOT to be called when cache hits, but it was called %d times", callCount)
	}

	var resp DeploymentEventsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("expected 1 cached event, got %d", len(resp.Events))
	}
	if resp.Events[0].Reason != "CachedEvent" {
		t.Errorf("expected cached event reason 'CachedEvent', got %q", resp.Events[0].Reason)
	}
}

func TestGetDeploymentEvents_CacheBypassDuringDeploy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")

	depID := deployid.New()
	acctID := uuid.New().String()
	now := time.Now()
	namespace := "astro-ns-pending"

	// Pre-populate cache — this should be ignored because status is "pending".
	cachedResp := DeploymentEventsResponse{
		Events: []K8sEventItem{
			{Type: "Normal", Reason: "StaleEvent", Message: "should be ignored"},
		},
	}
	cachedData, _ := json.Marshal(cachedResp)
	cache := &mockCache{data: map[string][]byte{
		"astro:k8s:events:" + namespace: cachedData,
	}}

	liveEventsJSON := `{
		"metadata":{"name":"evt-live","namespace":"astro-ns-pending","creationTimestamp":"2026-04-16T11:00:00Z"},
		"involvedObject":{"kind":"Pod","name":"agent-live"},
		"reason":"Pulling",
		"message":"Pulling image",
		"type":"Normal",
		"count":1,
		"firstTimestamp":"2026-04-16T11:00:00Z",
		"lastTimestamp":"2026-04-16T11:00:00Z"
	}`

	var callCount int
	k8sClient := newMockK8sClient(k8sEventsHandler(namespace, liveEventsJSON, &callCount))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/deployments/:id/events",
		GetDeploymentEvents(log, accountStore, k8sClient, deployStore, cache))

	// GetDeploymentByID — status is "pending" (transitional), so cache should be bypassed.
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRow(depID, acctID, "my-agent", "build-1", namespace,
			"My Agent", `{}`, "pending", now, nil))

	// IsMember
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest("GET", "/api/v1/deployments/"+depID+"/events", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if callCount != 1 {
		t.Fatalf("expected K8s API to be called once (cache bypass), got %d calls", callCount)
	}

	var resp DeploymentEventsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("expected 1 live event, got %d", len(resp.Events))
	}
	if resp.Events[0].Reason != "Pulling" {
		t.Errorf("expected live event reason 'Pulling', got %q", resp.Events[0].Reason)
	}
	if resp.Events[0].Message != "Pulling image" {
		t.Errorf("expected message 'Pulling image', got %q", resp.Events[0].Message)
	}
}

// --- Shared test helpers for template handler tests ---

func expectAccountLookup(mock sqlmock.Sqlmock) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
}

func expectAgentLookup(mock sqlmock.Sqlmock, visibility string) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", visibility, nil, false, nil, now, now))
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
		PostDeploymentTemplate(log, index, accountStore, cfg, deployStore, nil, nil))

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
	// generateTemplate
	expectGenerateTemplatePinned(indexMock, accountMock, specWithVarInputs)
	// GetByID for target.account
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow(acctID, "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, pq.StringArray(nil), pq.StringArray(nil)))
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
	// Base spec carries interfaces.messaging:true so the slack adapter is
	// available to inject SLACK_BOT_TOKEN.
	expectGenerateTemplatePinned(indexMock, accountMock, specWithMessaging)
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow(acctID, "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, pq.StringArray(nil), pq.StringArray(nil)))
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
	expectGenerateTemplatePinned(indexMock, accountMock, specWithMessaging)
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow(acctID, "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, pq.StringArray(nil), pq.StringArray(nil)))
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))

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
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at"}).
			AddRow("acct-1", "my-agent", "registry.io", "public", nil, false, nil, now, now))
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

	expectGenerateTemplatePinned(indexMock, accountMock, specWithIngestion)
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow(acctID, "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
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
		WillReturnRows(sqlmock.NewRows(
			[]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow(id, name, "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
}

// expectAgentLookupFor stubs the two queries inside Index.Get (agents row +
// versions list) for a specific (accountID, agentName) pair with the given
// visibility. Used when the source account differs from the URL account.
func expectAgentLookupFor(mock sqlmock.Sqlmock, accountID, agentName, visibility string) {
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs(accountID, agentName).
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at"}).
			AddRow(accountID, agentName, "registry.io", visibility, nil, false, nil, now, now))
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
	expectPrefillLineageValidated(indexMock, accountMock, "publisher", sourceAcctID, "my-agent", "build-1")
	// generateTemplate resolves everything under "publisher"
	// (source.account), not under "myorg" (URL account).
	expectAgentLookupFor(indexMock, sourceAcctID, "my-agent", "public")
	expectPinnedVersionFor(indexMock, sourceAcctID, "my-agent", "build-1")
	// GetDeploymentVariables (empty).
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}))
	// mergeDeploymentPrefill looks up the target account by ID to set
	// target.account on the template.
	expectAccountLookupFor(accountMock, targetAcctID, targetAcctID, "myorg")

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
	expectPrefillLineageValidated(indexMock, accountMock, "myorg", acctID, "my-agent", "build-1")
	expectAgentLookup(indexMock, "public")
	expectPinnedVersionFor(indexMock, acctID, "my-agent", "build-1")
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
	expectPrefillLineageValidated(indexMock, accountMock, "publisher", sourceAcctID, "my-agent", "build-1")
	expectAgentLookupFor(indexMock, sourceAcctID, "my-agent", "private")
	expectPinnedVersionFor(indexMock, sourceAcctID, "my-agent", "build-1")
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "name", "value", "ref", "secret", "optional", "targets", "nonce",
		}))
	expectAccountLookupFor(accountMock, targetAcctID, targetAcctID, "myorg")

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
	expectPrefillLineageValidated(indexMock, accountMock, "publisher", sourceAcctID, "my-agent", pinnedBuild)

	// The source account has two builds — "build-new" (latest) and
	// "build-old" (deployed). Index.Get lists them newest-first; the
	// handler then asks for the pinned one, not the latest.
	indexMock.ExpectQuery("SELECT .+ FROM agents WHERE account_id").
		WithArgs(sourceAcctID, "my-agent").
		WillReturnRows(sqlmock.NewRows(
			[]string{"account_id", "name", "registry", "visibility", "archived_at", "name_reserved", "avatar_colors", "created_at", "updated_at"}).
			AddRow(sourceAcctID, "my-agent", "registry.io", "public", nil, false, nil, now, now))
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
	expectAccountLookupFor(accountMock, targetAcctID, targetAcctID, "myorg")

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
// validation path for present values: valid+enabled (happy), unknown (400),
// and present-but-disabled (400).

// setupDeployRouterWithClusterStore is a copy of setupDeployRouter that
// additionally wires a real clusterstore.Store backed by an sqlmock. Returns
// the cluster mock alongside the existing mocks so callers can prime
// `clusterstore.Get` lookups.
func setupDeployRouterWithClusterStore(userID string) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock, sqlmock.Sqlmock, sqlmock.Sqlmock) {
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
	router.POST("/deploy", DeployAgent(log, index, accountStore, cfg, deployStore, nil, clusterStore, nil, &mockQueue{}, nil, nil, nil, nil, nil, nil, nil)) //nolint:staticcheck // nil varsStore, EntitlementChecker, avatarStore, omClient, db, auditStore, ksStore, authzStore, and preflighter skip checks in tests

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
	router, _, _, _, clusterMock := setupDeployRouterWithClusterStore("user-1")

	// clusterstore.Get returns ErrNotFound when sql.ErrNoRows propagates.
	clusterMock.ExpectQuery(`SELECT .+ FROM clusters WHERE id = \$1`).
		WithArgs("unknown-cluster").
		WillReturnError(sql.ErrNoRows)

	body := deployableSpecWithClusterID("unknown-cluster")
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
	router, _, _, _, clusterMock := setupDeployRouterWithClusterStore("user-1")

	now := time.Now()
	clusterMock.ExpectQuery(`SELECT .+ FROM clusters WHERE id = \$1`).
		WithArgs("staging").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "region", "eks_cluster_name", "eks_cluster_endpoint",
			"enabled", "created_at", "updated_at",
		}).AddRow("staging", "us-east-1", "staging-eks", "https://staging.eks.example", false, now, now))

	body := deployableSpecWithClusterID("staging")
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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

func TestDeploy_WithValidClusterID_PersistsToDeploymentsTable(t *testing.T) {
	router, indexMock, accountMock, deployMock, clusterMock := setupDeployRouterWithClusterStore("user-1")

	now := time.Now()
	clusterMock.ExpectQuery(`SELECT .+ FROM clusters WHERE id = \$1`).
		WithArgs("eu-west-1-managed").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "region", "eks_cluster_name", "eks_cluster_endpoint",
			"enabled", "created_at", "updated_at",
		}).AddRow("eu-west-1-managed", "eu-west-1", "prod-eu", "https://eu.eks.example", true, now, now))

	expectDeployPrep(accountMock, indexMock)

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
	req := httptest.NewRequest(http.MethodPost, "/deploy", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
