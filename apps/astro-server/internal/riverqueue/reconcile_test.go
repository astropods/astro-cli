package riverqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type testClusterClient struct {
	clientset *kubernetes.Clientset
}

func (c *testClusterClient) Clientset() *kubernetes.Clientset      { return c.clientset }
func (c *testClusterClient) Config() *rest.Config                  { return nil }
func (c *testClusterClient) CheckHealth() error                    { return nil }
func (c *testClusterClient) GetServerVersion() (string, error)     { return "v1.30.0", nil }
func (c *testClusterClient) DiagnoseConnection() map[string]string { return nil }

func newTestK8sClient(handler http.Handler) k8s.ClusterClient {
	srv := httptest.NewServer(handler)
	cs, _ := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	return &testClusterClient{clientset: cs}
}

var testDeployColumns = []string{
	"id", "account_id", "agent_name", "build_id", "namespace", "display_name",
	"deployment_spec_json", "encrypted_data_key", "kms_key_arn",
	"status", "error_message", "error_details", "status_changed_at", "current_revision",
	"deployed_at", "undeployed_at", "avatar_colors",
}

func k8sNamespaceListHandler(namespaces ...string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/namespaces", func(w http.ResponseWriter, r *http.Request) {
		var items []string
		for _, ns := range namespaces {
			items = append(items, fmt.Sprintf(`{"metadata":{"name":%q,"labels":{"app.kubernetes.io/managed-by":"astro-server","astro.dev/account-id":"acct-1","astro.dev/agent":"agent"}}}`, ns))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"apiVersion":"v1","kind":"NamespaceList","items":[%s]}`, strings.Join(items, ","))
	})
	return mux
}

func addDeployRow(rows *sqlmock.Rows, id, namespace, status string) {
	now := time.Now()
	rows.AddRow(id, "acct-1", "agent", "build-1", namespace, "agent",
		"{}", nil, nil,
		status, nil, nil, now, nil,
		now, nil, nil)
}

func TestMaintainNamespaceOwnership_PendingNotOrphaned(t *testing.T) {
	k8sClient := newTestK8sClient(k8sNamespaceListHandler("astro-abc-0"))

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	rows := sqlmock.NewRows(testDeployColumns)
	addDeployRow(rows, "dep-1", "astro-abc-0", "pending")
	mock.ExpectQuery("SELECT .+ FROM deployments").WillReturnRows(rows)

	mock.ExpectExec("INSERT INTO namespace_ownership").
		WithArgs("astro-abc-0", "acct-1", "agent", "dep-1", "", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		db:    db,
		log:   logger.New("error", "json"),
	}

	w.maintainNamespaceOwnership(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

func TestMaintainNamespaceOwnership_OrphanRecovered(t *testing.T) {
	k8sClient := newTestK8sClient(k8sNamespaceListHandler("astro-orphan-0"))

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	mock.ExpectQuery("SELECT .+ FROM deployments").
		WillReturnRows(sqlmock.NewRows(testDeployColumns))

	// Expect the recovery transaction: BEGIN, INSERT deployment, INSERT event, COMMIT
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO deployments").
		WithArgs(sqlmock.AnyArg(), "acct-1", "agent", "", "astro-orphan-0", "failed", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deployment_events").
		WithArgs(sqlmock.AnyArg(), "failed", "Deployment recovered from orphaned K8s namespace").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		db:    db,
		log:   logger.New("warn", "json"),
	}

	w.maintainNamespaceOwnership(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

func TestMaintainNamespaceOwnership_AllLiveStatusesIncluded(t *testing.T) {
	statuses := []struct {
		status    string
		namespace string
	}{
		{"active", "astro-active-0"},
		{"scaled_down", "astro-scaled-0"},
		{"pending", "astro-pending-0"},
		{"provisioning", "astro-prov-0"},
		{"failed", "astro-failed-0"},
		{"undeploying", "astro-undep-0"},
	}

	var nsList []string
	for _, s := range statuses {
		nsList = append(nsList, s.namespace)
	}
	k8sClient := newTestK8sClient(k8sNamespaceListHandler(nsList...))

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	rows := sqlmock.NewRows(testDeployColumns)
	for i, s := range statuses {
		addDeployRow(rows, fmt.Sprintf("dep-%d", i), s.namespace, s.status)
	}
	mock.ExpectQuery("SELECT .+ FROM deployments").WillReturnRows(rows)

	for range statuses {
		mock.ExpectExec("INSERT INTO namespace_ownership").
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		db:    db,
		log:   logger.New("error", "json"),
	}

	w.maintainNamespaceOwnership(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

func TestSourceAccountFromSpec(t *testing.T) {
	tests := []struct {
		name     string
		specJSON string
		want     string
	}{
		{"full spec", `{"source":{"account":"team-a","name":"bot","build":"b1"}}`, "team-a"},
		{"empty spec", `{}`, ""},
		{"empty string", "", ""},
		{"no source", `{"target":{"account":"team-b"}}`, ""},
		{"invalid json", `{broken`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deploymentstore.SourceAccountFromSpec(tt.specJSON)
			if got != tt.want {
				t.Errorf("SourceAccountFromSpec(%q) = %q, want %q", tt.specJSON, got, tt.want)
			}
		})
	}
}

func TestMaintainNamespaceOwnership_SourceAccountPopulated(t *testing.T) {
	k8sClient := newTestK8sClient(k8sNamespaceListHandler("astro-abc-0"))

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	now := time.Now()
	rows := sqlmock.NewRows(testDeployColumns)
	// Row with a spec that has source.account set
	rows.AddRow("dep-1", "acct-1", "agent", "build-1", "astro-abc-0", "agent",
		`{"source":{"account":"source-team","name":"agent","build":"build-1"}}`, nil, nil,
		"active", nil, nil, now, nil,
		now, nil, nil)
	mock.ExpectQuery("SELECT .+ FROM deployments").WillReturnRows(rows)

	// Expect source_account = "source-team" in the upsert
	mock.ExpectExec("INSERT INTO namespace_ownership").
		WithArgs("astro-abc-0", "acct-1", "agent", "dep-1", "source-team", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		db:    db,
		log:   logger.New("error", "json"),
	}

	w.maintainNamespaceOwnership(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

func TestNamespaceOrphanLogic(t *testing.T) {
	dbNamespaces := map[string]bool{
		"astro-active-0":  true,
		"astro-pending-0": true,
		"astro-failed-0":  true,
	}

	k8sNamespaces := []string{
		"astro-active-0",
		"astro-pending-0",
		"astro-failed-0",
		"astro-gone-0",
	}

	var orphaned []string
	for _, ns := range k8sNamespaces {
		if !dbNamespaces[ns] {
			orphaned = append(orphaned, ns)
		}
	}

	if len(orphaned) != 1 {
		t.Fatalf("expected 1 orphan, got %d: %v", len(orphaned), orphaned)
	}
	if orphaned[0] != "astro-gone-0" {
		t.Errorf("expected orphan 'astro-gone-0', got %q", orphaned[0])
	}
}

// ---------------------------------------------------------------------------
// Drift report tests
// ---------------------------------------------------------------------------

// fakeK8sResources defines the resources that exist in the fake cluster.
type fakeK8sResources struct {
	deployments  map[string]fakeDeployment
	statefulsets map[string]fakeStatefulSet
	cronjobs     map[string]fakeCronJob
	services     map[string]bool
}

type fakeDeployment struct {
	Image    string
	Replicas int32
}

type fakeStatefulSet struct {
	Image    string
	Replicas int32
}

type fakeCronJob struct {
	Schedule string
}

// newFakeDriftK8s creates an HTTP server that simulates the K8s API for drift checks.
// Supports both GET (single resource) and LIST (all resources in namespace).
func newFakeDriftK8s(t *testing.T, ns string, res fakeK8sResources) *kubernetes.Clientset {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		// LIST deployments
		case strings.HasSuffix(path, "/deployments") || strings.HasSuffix(path, "/deployments/"):
			var items []map[string]any
			for name, d := range res.deployments {
				items = append(items, map[string]any{
					"kind": "Deployment", "apiVersion": "apps/v1",
					"metadata": map[string]any{"name": name, "namespace": ns},
					"spec": map[string]any{
						"replicas": d.Replicas,
						"template": map[string]any{"spec": map[string]any{
							"containers": []map[string]any{{"name": name, "image": d.Image}},
						}},
					},
					"status": map[string]any{"readyReplicas": d.Replicas},
				})
			}
			writeDriftJSON(w, map[string]any{"kind": "DeploymentList", "apiVersion": "apps/v1", "items": items})

		// GET deployment
		case strings.Contains(path, "/deployments/"):
			name := path[strings.LastIndex(path, "/")+1:]
			if d, ok := res.deployments[name]; ok {
				writeDriftJSON(w, map[string]any{
					"kind": "Deployment", "apiVersion": "apps/v1",
					"metadata": map[string]any{"name": name, "namespace": ns},
					"spec": map[string]any{
						"replicas": d.Replicas,
						"template": map[string]any{"spec": map[string]any{
							"containers": []map[string]any{{"name": name, "image": d.Image}},
						}},
					},
					"status": map[string]any{"readyReplicas": d.Replicas},
				})
			} else {
				writeDriftNotFound(w, "deployments", name)
			}

		// LIST statefulsets
		case strings.HasSuffix(path, "/statefulsets") || strings.HasSuffix(path, "/statefulsets/"):
			var items []map[string]any
			for name, ss := range res.statefulsets {
				items = append(items, map[string]any{
					"kind": "StatefulSet", "apiVersion": "apps/v1",
					"metadata": map[string]any{"name": name, "namespace": ns},
					"spec": map[string]any{
						"replicas": ss.Replicas,
						"template": map[string]any{"spec": map[string]any{
							"containers": []map[string]any{{"name": name, "image": ss.Image}},
						}},
					},
					"status": map[string]any{"readyReplicas": ss.Replicas},
				})
			}
			writeDriftJSON(w, map[string]any{"kind": "StatefulSetList", "apiVersion": "apps/v1", "items": items})

		// GET statefulset
		case strings.Contains(path, "/statefulsets/"):
			name := path[strings.LastIndex(path, "/")+1:]
			if ss, ok := res.statefulsets[name]; ok {
				writeDriftJSON(w, map[string]any{
					"kind": "StatefulSet", "apiVersion": "apps/v1",
					"metadata": map[string]any{"name": name, "namespace": ns},
					"spec": map[string]any{
						"replicas": ss.Replicas,
						"template": map[string]any{"spec": map[string]any{
							"containers": []map[string]any{{"name": name, "image": ss.Image}},
						}},
					},
					"status": map[string]any{"readyReplicas": ss.Replicas},
				})
			} else {
				writeDriftNotFound(w, "statefulsets", name)
			}

		// GET cronjob
		case strings.Contains(path, "/cronjobs/"):
			name := path[strings.LastIndex(path, "/")+1:]
			if cj, ok := res.cronjobs[name]; ok {
				writeDriftJSON(w, map[string]any{
					"kind": "CronJob", "apiVersion": "batch/v1",
					"metadata": map[string]any{"name": name, "namespace": ns},
					"spec":     map[string]any{"schedule": cj.Schedule},
				})
			} else {
				writeDriftNotFound(w, "cronjobs", name)
			}

		// LIST services
		case strings.HasSuffix(path, "/services") || strings.HasSuffix(path, "/services/"):
			var items []map[string]any
			for name := range res.services {
				items = append(items, map[string]any{
					"kind": "Service", "apiVersion": "v1",
					"metadata": map[string]any{"name": name, "namespace": ns},
					"spec":     map[string]any{},
				})
			}
			writeDriftJSON(w, map[string]any{"kind": "ServiceList", "apiVersion": "v1", "items": items})

		// GET service
		case strings.Contains(path, "/services/"):
			name := path[strings.LastIndex(path, "/")+1:]
			if res.services[name] {
				writeDriftJSON(w, map[string]any{
					"kind": "Service", "apiVersion": "v1",
					"metadata": map[string]any{"name": name, "namespace": ns},
					"spec":     map[string]any{},
				})
			} else {
				writeDriftNotFound(w, "services", name)
			}

		// LIST ingresses
		case strings.HasSuffix(path, "/ingresses") || strings.HasSuffix(path, "/ingresses/"):
			writeDriftJSON(w, map[string]any{"kind": "IngressList", "apiVersion": "networking.k8s.io/v1", "items": []any{}})

		default:
			writeDriftNotFound(w, "unknown", path)
		}
	}))
	t.Cleanup(srv.Close)

	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("NewForConfig: %v", err)
	}
	return cs
}

func writeDriftJSON(w http.ResponseWriter, v any) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeDriftNotFound(w http.ResponseWriter, kind, name string) {
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"kind":       "Status",
		"apiVersion": "v1",
		"status":     "Failure",
		"message":    fmt.Sprintf("%s %q not found", kind, name),
		"reason":     "NotFound",
		"code":       404,
	})
}

func strPtr(s string) *string { return &s }

// testBuildReport is a helper that calls BuildDriftReport with minimal params.
func testBuildReport(ctx context.Context, cs *kubernetes.Clientset, ns string, workloads []*deploymentstore.Workload, services []*deploymentstore.Service) *deploymentstore.DriftReport {
	return BuildDriftReport(ctx, cs, ns, "test-agent", "v1", workloads, services, nil, nil, nil, nil)
}

// countStatus returns the number of items with the given status in a report.
func countStatus(items []deploymentstore.DriftResourceItem, status string) int {
	n := 0
	for _, item := range items {
		if item.Status == status {
			n++
		}
	}
	return n
}

// findItem finds a drift item by name.
func findItem(items []deploymentstore.DriftResourceItem, name string) *deploymentstore.DriftResourceItem {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

// --- Real sasbot deployment data ---

// sasbotWorkloads returns the normalized workloads for sasbot.
// Sidecars (messaging, collector) are now in a separate table and not included here.
func sasbotWorkloads() []*deploymentstore.Workload {
	return []*deploymentstore.Workload{
		{Name: "sasbot-agent", WorkloadType: "deployment", Image: "sasbot:14f4c4dd", Replicas: 1},
		{Name: "sasbot-knowledge-cache", WorkloadType: "deployment", Image: "redis:7-alpine", Replicas: 1},
		{Name: "sasbot-knowledge-graph", WorkloadType: "deployment", Image: "neo4j:5-community", Replicas: 1},
		{Name: "sasbot-ingestion-webhook", WorkloadType: "deployment", Image: "sasbot-ingestion-webhook:14f4c4dd", Replicas: 1},
		{Name: "sasbot-model-ollama", WorkloadType: "statefulset", Image: "ollama:latest", Replicas: 1},
		{Name: "sasbot-knowledge-docs", WorkloadType: "statefulset", Image: "qdrant:latest", Replicas: 1},
	}
}

// sasbotServices returns the FIXED normalized services with WorkloadName set.
func sasbotServices() []*deploymentstore.Service {
	return []*deploymentstore.Service{
		{Name: "http", Port: 8080, TargetPort: 8080, Protocol: "http", WorkloadName: "sasbot-agent"},
		{Name: "grpc", Port: 9090, TargetPort: 9090, Protocol: "grpc", WorkloadName: "sasbot-messaging"},
		{Name: "http", Port: 8090, TargetPort: 8090, Protocol: "http", WorkloadName: "sasbot-messaging"},
		{Name: "http", Port: 6379, TargetPort: 6379, Protocol: "http", WorkloadName: "sasbot-knowledge-cache"},
		{Name: "bolt", Port: 7687, TargetPort: 7687, Protocol: "tcp", WorkloadName: "sasbot-knowledge-graph"},
		{Name: "http", Port: 7474, TargetPort: 7474, Protocol: "http", WorkloadName: "sasbot-knowledge-graph"},
		{Name: "http", Port: 6333, TargetPort: 6333, Protocol: "http", WorkloadName: "sasbot-knowledge-docs"},
		{Name: "http", Port: 11434, TargetPort: 11434, Protocol: "http", WorkloadName: "sasbot-model-ollama"},
		{Name: "http", Port: 3001, TargetPort: 3001, Protocol: "http", WorkloadName: "sasbot-ingestion-webhook"},
		{Name: "otlp-grpc", Port: 4317, TargetPort: 4317, Protocol: "grpc", WorkloadName: "sasbot-collector"},
		{Name: "otlp-http", Port: 4318, TargetPort: 4318, Protocol: "http", WorkloadName: "sasbot-collector"},
	}
}

// sasbotK8sResources returns what actually exists in K8s for sasbot.
func sasbotK8sResources() fakeK8sResources {
	return fakeK8sResources{
		deployments: map[string]fakeDeployment{
			"sasbot-agent":             {Image: "sasbot:14f4c4dd", Replicas: 1},
			"sasbot-knowledge-cache":   {Image: "redis:7-alpine", Replicas: 1},
			"sasbot-knowledge-graph":   {Image: "neo4j:5-community", Replicas: 1},
			"sasbot-ingestion-webhook": {Image: "sasbot-ingestion-webhook:14f4c4dd", Replicas: 1},
			// Note: messaging and collector are NOT standalone deployments (they're sidecars).
		},
		statefulsets: map[string]fakeStatefulSet{
			"sasbot-model-ollama":   {Image: "ollama:latest", Replicas: 1},
			"sasbot-knowledge-docs": {Image: "qdrant:latest", Replicas: 1},
		},
		services: map[string]bool{
			"sasbot-agent":             true,
			"sasbot-messaging":         true,
			"sasbot-collector":         true,
			"sasbot-knowledge-cache":   true,
			"sasbot-knowledge-graph":   true,
			"sasbot-knowledge-docs":    true,
			"sasbot-model-ollama":      true,
			"sasbot-ingestion-webhook": true,
		},
	}
}

// TestBuildDriftReport_Sasbot_NoDrift verifies that a healthy sasbot deployment
// reports zero drift with the fixed data model.
func TestBuildDriftReport_Sasbot_NoDrift(t *testing.T) {
	ns := "sasbot-ns"
	cs := newFakeDriftK8s(t, ns, sasbotK8sResources())

	report := testBuildReport(context.Background(), cs, ns, sasbotWorkloads(), sasbotServices())
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Summary.Missing+report.Summary.Drift > 0 {
		t.Errorf("expected zero drift for healthy sasbot, got missing=%d drift=%d", report.Summary.Missing, report.Summary.Drift)
		for _, item := range report.Workloads {
			if item.Status != "match" {
				t.Logf("  workload %s: %s", item.Name, item.Status)
			}
		}
		for _, item := range report.Services {
			if item.Status != "match" {
				t.Logf("  service %s: %s", item.Name, item.Status)
			}
		}
	}
}

// TestBuildDriftReport_Sasbot_OldBugReproduction reproduces the exact bug from
// production: storing messaging/collector as "deployment" and services by
// endpoint name caused false drifts on a healthy cluster.
func TestBuildDriftReport_Sasbot_OldBugReproduction(t *testing.T) {
	ns := "sasbot-ns"
	cs := newFakeDriftK8s(t, ns, sasbotK8sResources())

	oldWorkloads := []*deploymentstore.Workload{
		{Name: "sasbot-agent", WorkloadType: "deployment", Image: "sasbot:14f4c4dd", Replicas: 1},
		{Name: "sasbot-knowledge-cache", WorkloadType: "deployment", Image: "redis:7-alpine", Replicas: 1},
		{Name: "sasbot-knowledge-graph", WorkloadType: "deployment", Image: "neo4j:5-community", Replicas: 1},
		{Name: "sasbot-ingestion-webhook", WorkloadType: "deployment", Image: "sasbot-ingestion-webhook:14f4c4dd", Replicas: 1},
		{Name: "sasbot-model-ollama", WorkloadType: "statefulset", Image: "ollama:latest", Replicas: 1},
		{Name: "sasbot-knowledge-docs", WorkloadType: "statefulset", Image: "qdrant:latest", Replicas: 1},
		// BUG: these were "deployment" — drift checker looks for standalone K8s Deployments
		{Name: "sasbot-messaging", WorkloadType: "deployment", Image: "messaging:latest", Replicas: 1},
		{Name: "sasbot-collector", WorkloadType: "deployment", Image: "astropods/collector:latest", Replicas: 1},
	}

	oldServices := []*deploymentstore.Service{
		{Name: "http", Port: 8080},
		{Name: "http", Port: 8090},
		{Name: "http", Port: 6379},
		{Name: "http", Port: 7474},
		{Name: "http", Port: 6333},
		{Name: "http", Port: 11434},
		{Name: "http", Port: 3001},
		{Name: "grpc", Port: 9090},
		{Name: "bolt", Port: 7687},
		{Name: "otlp-grpc", Port: 4317},
		{Name: "otlp-http", Port: 4318},
	}

	report := testBuildReport(context.Background(), cs, ns, oldWorkloads, oldServices)

	deployMissing := countStatus(report.Workloads, "missing")
	svcMissing := countStatus(report.Services, "missing")

	// Reproduces: 2 deployment missing (messaging, collector) + service missing (endpoint names)
	if deployMissing != 2 {
		t.Errorf("expected 2 false deployment-missing, got %d", deployMissing)
	}
	if svcMissing == 0 {
		t.Error("expected service-missing from endpoint-named data")
	}

	t.Logf("Old bug reproduced: %d total items, %d missing, %d drift", report.Summary.Total, report.Summary.Missing, report.Summary.Drift)
}

// TestBuildDriftReport_EmptyWorkloads verifies zero workloads produces an all-match report.
func TestBuildDriftReport_EmptyWorkloads(t *testing.T) {
	ns := "test-ns"
	cs := newFakeDriftK8s(t, ns, fakeK8sResources{})

	report := testBuildReport(context.Background(), cs, ns, nil, nil)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Summary.Missing+report.Summary.Drift > 0 {
		t.Errorf("empty workloads should not produce drift, got missing=%d drift=%d", report.Summary.Missing, report.Summary.Drift)
	}
}

// TestBuildDriftReport_ServiceDedup verifies multiple endpoints sharing one K8s
// Service only result in a single check (no duplicate "missing" reports).
func TestBuildDriftReport_ServiceDedup(t *testing.T) {
	ns := "test-ns"
	cs := newFakeDriftK8s(t, ns, fakeK8sResources{
		services: map[string]bool{"my-svc": true},
	})

	services := []*deploymentstore.Service{
		{Name: "http", Port: 8080, WorkloadName: "my-svc"},
		{Name: "grpc", Port: 9090, WorkloadName: "my-svc"},
		{Name: "metrics", Port: 9191, WorkloadName: "my-svc"},
	}

	report := testBuildReport(context.Background(), cs, ns, nil, services)
	if len(report.Services) != 1 {
		t.Fatalf("expected 1 service entry (deduped), got %d", len(report.Services))
	}
	if report.Services[0].Status != "match" {
		t.Errorf("expected match, got %s", report.Services[0].Status)
	}
}

// TestBuildDriftReport_ServiceMissing_ReportedOnce verifies a missing service is
// reported exactly once even with multiple endpoints.
func TestBuildDriftReport_ServiceMissing_ReportedOnce(t *testing.T) {
	ns := "test-ns"
	cs := newFakeDriftK8s(t, ns, fakeK8sResources{})

	services := []*deploymentstore.Service{
		{Name: "grpc", Port: 9090, WorkloadName: "my-svc"},
		{Name: "http", Port: 8090, WorkloadName: "my-svc"},
	}

	report := testBuildReport(context.Background(), cs, ns, nil, services)
	missing := countStatus(report.Services, "missing")
	if missing != 1 {
		t.Fatalf("expected 1 missing service, got %d", missing)
	}
	if report.Services[0].Name != "my-svc" {
		t.Errorf("expected service name 'my-svc', got %q", report.Services[0].Name)
	}
}

// TestBuildDriftReport_ServiceUsesWorkloadName verifies services are looked up by
// WorkloadName (K8s resource name), not endpoint Name.
func TestBuildDriftReport_ServiceUsesWorkloadName(t *testing.T) {
	ns := "test-ns"
	cs := newFakeDriftK8s(t, ns, fakeK8sResources{
		services: map[string]bool{"agent-svc": true},
	})

	services := []*deploymentstore.Service{
		{Name: "http", Port: 8080, WorkloadName: "agent-svc"},
	}

	report := testBuildReport(context.Background(), cs, ns, nil, services)
	if report.Summary.Missing > 0 {
		t.Errorf("should match on WorkloadName not Name, got missing=%d", report.Summary.Missing)
	}
}

// TestBuildDriftReport_ServiceFallbackToName verifies legacy data (no WorkloadName)
// falls back to using the endpoint Name for lookup.
func TestBuildDriftReport_ServiceFallbackToName(t *testing.T) {
	ns := "test-ns"
	cs := newFakeDriftK8s(t, ns, fakeK8sResources{
		services: map[string]bool{"old-svc": true},
	})

	services := []*deploymentstore.Service{
		{Name: "old-svc", Port: 8080, WorkloadName: ""},
	}

	report := testBuildReport(context.Background(), cs, ns, nil, services)
	if report.Summary.Missing > 0 {
		t.Errorf("fallback to Name should work, got missing=%d", report.Summary.Missing)
	}
}

// TestBuildDriftReport_DeploymentImageMismatch detects image drift.
func TestBuildDriftReport_DeploymentImageMismatch(t *testing.T) {
	ns := "test-ns"
	cs := newFakeDriftK8s(t, ns, fakeK8sResources{
		deployments: map[string]fakeDeployment{
			"my-agent": {Image: "agent:old-tag", Replicas: 1},
		},
	})

	workloads := []*deploymentstore.Workload{
		{Name: "my-agent", WorkloadType: "deployment", Image: "agent:new-tag", Replicas: 1},
	}

	report := testBuildReport(context.Background(), cs, ns, workloads, nil)
	item := findItem(report.Workloads, "my-agent")
	if item == nil || item.Status != "drift" {
		t.Errorf("expected drift for image mismatch, got %v", item)
	}
	if item != nil && item.Actual["Image"] != "agent:old-tag" {
		t.Errorf("expected actual image 'agent:old-tag', got %q", item.Actual["Image"])
	}
}

// TestBuildDriftReport_ReplicaMismatch detects replica count drift.
func TestBuildDriftReport_ReplicaMismatch(t *testing.T) {
	ns := "test-ns"
	cs := newFakeDriftK8s(t, ns, fakeK8sResources{
		deployments: map[string]fakeDeployment{
			"my-agent": {Image: "agent:v1", Replicas: 3},
		},
	})

	workloads := []*deploymentstore.Workload{
		{Name: "my-agent", WorkloadType: "deployment", Image: "agent:v1", Replicas: 1},
	}

	report := testBuildReport(context.Background(), cs, ns, workloads, nil)
	item := findItem(report.Workloads, "my-agent")
	if item == nil || item.Status != "drift" {
		t.Errorf("expected drift for replica mismatch, got %v", item)
	}
}

// TestBuildDriftReport_CronJobScheduleMismatch detects schedule drift.
func TestBuildDriftReport_CronJobScheduleMismatch(t *testing.T) {
	ns := "test-ns"
	cs := newFakeDriftK8s(t, ns, fakeK8sResources{
		cronjobs: map[string]fakeCronJob{
			"my-cron": {Schedule: "0 */6 * * *"},
		},
	})

	workloads := []*deploymentstore.Workload{
		{Name: "my-cron", WorkloadType: "cronjob", TriggerSchedule: strPtr("0 * * * *")},
	}

	report := testBuildReport(context.Background(), cs, ns, workloads, nil)
	item := findItem(report.Workloads, "my-cron")
	if item == nil || item.Status != "drift" {
		t.Errorf("expected drift for schedule mismatch, got %v", item)
	}
}

// TestBuildDriftReport_MixedWorkloadTypes tests all workload types together.
func TestBuildDriftReport_MixedWorkloadTypes(t *testing.T) {
	ns := "mixed-ns"
	cs := newFakeDriftK8s(t, ns, fakeK8sResources{
		deployments:  map[string]fakeDeployment{"web": {Image: "web:v1", Replicas: 2}},
		statefulsets: map[string]fakeStatefulSet{"db": {Image: "pg:15", Replicas: 1}},
		cronjobs:     map[string]fakeCronJob{"cleanup": {Schedule: "0 3 * * *"}},
		services:     map[string]bool{"web": true, "db": true},
	})

	workloads := []*deploymentstore.Workload{
		{Name: "web", WorkloadType: "deployment", Image: "web:v1", Replicas: 2},
		{Name: "db", WorkloadType: "statefulset", Image: "pg:15", Replicas: 1},
		{Name: "cleanup", WorkloadType: "cronjob", TriggerSchedule: strPtr("0 3 * * *")},
	}
	services := []*deploymentstore.Service{
		{Name: "http", Port: 8080, WorkloadName: "web"},
		{Name: "tcp", Port: 5432, WorkloadName: "db"},
	}

	report := testBuildReport(context.Background(), cs, ns, workloads, services)
	if report.Summary.Missing+report.Summary.Drift > 0 {
		t.Errorf("all matching, should produce zero drift, got missing=%d drift=%d", report.Summary.Missing, report.Summary.Drift)
	}
}

// TestBuildDriftReport_MultipleServicesMissing tests dedup across multiple missing services.
func TestBuildDriftReport_MultipleServicesMissing(t *testing.T) {
	ns := "test-ns"
	cs := newFakeDriftK8s(t, ns, fakeK8sResources{
		services: map[string]bool{"svc-a": true},
	})

	services := []*deploymentstore.Service{
		{Name: "http", Port: 80, WorkloadName: "svc-a"},
		{Name: "grpc", Port: 9090, WorkloadName: "svc-b"},
		{Name: "http", Port: 8080, WorkloadName: "svc-b"},
		{Name: "tcp", Port: 5432, WorkloadName: "svc-c"},
	}

	report := testBuildReport(context.Background(), cs, ns, nil, services)
	missing := countStatus(report.Services, "missing")
	if missing != 2 {
		t.Fatalf("expected 2 missing services (svc-b, svc-c), got %d", missing)
	}
}

// TestBuildDriftReport_StatefulSetMissing verifies StatefulSet drift detection.
func TestBuildDriftReport_StatefulSetMissing(t *testing.T) {
	ns := "test-ns"
	cs := newFakeDriftK8s(t, ns, fakeK8sResources{})

	workloads := []*deploymentstore.Workload{
		{Name: "my-db", WorkloadType: "statefulset", Image: "pg:15", Replicas: 1},
	}

	report := testBuildReport(context.Background(), cs, ns, workloads, nil)
	item := findItem(report.Workloads, "my-db")
	if item == nil || item.Status != "missing" {
		t.Errorf("expected StatefulSet missing, got %v", item)
	}
}

// TestBuildDriftReport_Summary verifies summary counts are correct.
func TestBuildDriftReport_Summary(t *testing.T) {
	ns := "test-ns"
	cs := newFakeDriftK8s(t, ns, fakeK8sResources{
		deployments: map[string]fakeDeployment{
			"web": {Image: "web:v1", Replicas: 1},
		},
	})

	workloads := []*deploymentstore.Workload{
		{Name: "web", WorkloadType: "deployment", Image: "web:v1", Replicas: 1},
		{Name: "api", WorkloadType: "deployment", Image: "api:v1", Replicas: 1}, // missing in K8s
	}

	report := testBuildReport(context.Background(), cs, ns, workloads, nil)
	if report.Summary.Match != 1 {
		t.Errorf("expected 1 match, got %d", report.Summary.Match)
	}
	if report.Summary.Missing != 1 {
		t.Errorf("expected 1 missing, got %d", report.Summary.Missing)
	}
	if report.Summary.Total < 2 {
		t.Errorf("expected total >= 2, got %d", report.Summary.Total)
	}
}
