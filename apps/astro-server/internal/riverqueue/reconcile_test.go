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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
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
	"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
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

// k8sNS describes one namespace returned by the labelled list handler.
// labels override the default set; account-id and agent default to the
// "acct-1"/"agent" pair the existing simple helper uses.
type k8sNS struct {
	name   string
	labels map[string]string
}

// k8sNamespaceListLabeledHandler is the per-namespace-label sibling of
// k8sNamespaceListHandler. Use it when a test asserts on something the
// reconciler reads off labels (e.g. astro.dev/source-account-id) that
// the simple helper hardcodes.
func k8sNamespaceListLabeledHandler(items ...k8sNS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/namespaces", func(w http.ResponseWriter, r *http.Request) {
		var encoded []string
		for _, ns := range items {
			labels := map[string]string{
				"app.kubernetes.io/managed-by": "astro-server",
				"astro.dev/account-id":         "acct-1",
				"astro.dev/agent":              "agent",
			}
			for k, v := range ns.labels {
				labels[k] = v
			}
			labelJSON, _ := json.Marshal(labels)
			encoded = append(encoded, fmt.Sprintf(`{"metadata":{"name":%q,"labels":%s}}`, ns.name, labelJSON))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"apiVersion":"v1","kind":"NamespaceList","items":[%s]}`, strings.Join(encoded, ","))
	})
	return mux
}

func addDeployRow(rows *sqlmock.Rows, id, namespace, status string) {
	now := time.Now()
	rows.AddRow(id, "acct-1", nil, "agent", "build-1", namespace, "agent",
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

	// No orphan recovery expected — the namespace matches a pending deployment.
	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		log:   logger.New("error", "json"),
	}

	w.maintainNamespaceOwnership(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

// Without astro.dev/source-account-id on the namespace, the reconciler
// must default source_account_id to the deployer account. This test
// pins both that fallback and the INSERT arg shape that lands the value
// at write time (no NULL-then-backfill round trip).
func TestMaintainNamespaceOwnership_OrphanRecovered(t *testing.T) {
	k8sClient := newTestK8sClient(k8sNamespaceListHandler("astro-orphan-0"))

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	mock.ExpectQuery("SELECT .+ FROM deployments").
		WillReturnRows(sqlmock.NewRows(testDeployColumns))

	// INSERT args: (id, accountID, sourceAccountID, agentName, buildID,
	// namespace, status, errorMessage). Source defaults to accountID
	// because the simple list handler does not stamp the new label.
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO deployments").
		WithArgs(sqlmock.AnyArg(), "acct-1", "acct-1", "agent", "", "astro-orphan-0", "failed", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deployment_events").
		WithArgs(sqlmock.AnyArg(), "failed", "Deployment recovered from orphaned K8s namespace").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		log:   logger.New("warn", "json"),
	}

	w.maintainNamespaceOwnership(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

// With astro.dev/source-account-id stamped on the namespace, the
// reconciler must thread that value through to the INSERT verbatim
// rather than defaulting to the deployer account.
func TestMaintainNamespaceOwnership_OrphanRecovered_LabeledSource(t *testing.T) {
	k8sClient := newTestK8sClient(k8sNamespaceListLabeledHandler(
		k8sNS{
			name: "astro-orphan-0",
			labels: map[string]string{
				"astro.dev/source-account-id": "src-1",
				"astro.dev/build":             "build-9",
			},
		},
	))

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	mock.ExpectQuery("SELECT .+ FROM deployments").
		WillReturnRows(sqlmock.NewRows(testDeployColumns))

	mock.ExpectBegin()
	// source_account_id (3rd positional) must be "src-1", NOT "acct-1".
	mock.ExpectExec("INSERT INTO deployments").
		WithArgs(sqlmock.AnyArg(), "acct-1", "src-1", "agent", "build-9", "astro-orphan-0", "failed", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deployment_events").
		WithArgs(sqlmock.AnyArg(), "failed", "Deployment recovered from orphaned K8s namespace").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
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

	// No orphan recovery expected — all K8s namespaces are in the DB.
	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		log:   logger.New("error", "json"),
	}

	w.maintainNamespaceOwnership(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// OIDC issuer reconciliation tests
// ---------------------------------------------------------------------------

func TestSpecHasOIDCAuth(t *testing.T) {
	tests := []struct {
		name     string
		specJSON string
		want     bool
	}{
		{"oidc enabled", `{"interfaces":{"adapters":["web"],"image":"img","resources":{},"auth":{"web":{"type":"oidc"}}}}`, true},
		{"no auth", `{"interfaces":{"adapters":["web"],"image":"img","resources":{}}}`, false},
		{"auth without web", `{"interfaces":{"adapters":["web"],"image":"img","resources":{},"auth":{}}}`, false},
		{"web type empty", `{"interfaces":{"adapters":["web"],"image":"img","resources":{},"auth":{"web":{"type":""}}}}`, false},
		{"web type other", `{"interfaces":{"adapters":["web"],"image":"img","resources":{},"auth":{"web":{"type":"custom"}}}}`, false},
		{"no interfaces", `{}`, false},
		{"invalid json", `{broken`, false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := specHasOIDCAuth(tt.specJSON)
			if got != tt.want {
				t.Errorf("specHasOIDCAuth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildExpectedOIDCAnnotation(t *testing.T) {
	got := buildExpectedOIDCAnnotation(
		"https://login.example.com",
		"https://login.example.com/authorize",
		"https://login.example.com/token",
		"https://login.example.com/userinfo",
	)
	// Must be valid JSON containing all fields
	var parsed map[string]string
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if parsed["issuer"] != "https://login.example.com" {
		t.Errorf("issuer = %q", parsed["issuer"])
	}
	if parsed["secretName"] != "messaging-oidc" {
		t.Errorf("secretName = %q", parsed["secretName"])
	}
	if parsed["authorizationEndpoint"] != "https://login.example.com/authorize" {
		t.Errorf("authorizationEndpoint = %q", parsed["authorizationEndpoint"])
	}
}

func TestReconcileOIDCIssuer_SkipsWhenDisabled(t *testing.T) {
	// When issuer is empty, reconcileOIDCIssuer should return immediately
	// without querying the store.
	w := &ReconcileWorker{
		deployer: &deployer.Deployer{Cfg: &config.Config{}},
		log:      logger.New("error", "json"),
	}
	// If this tried to access the store it would panic (store is nil)
	w.reconcileOIDCIssuer(t.Context())
}

func TestReconcileOIDCIssuer_SkipsNonOIDCDeployments(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	cfg := &config.Config{}
	cfg.Deployment.MessagingOIDCIssuer = "https://login.example.com"

	// Return one active deployment without OIDC auth in its spec
	rows := sqlmock.NewRows(testDeployColumns)
	now := time.Now()
	rows.AddRow("dep-1", "acct-1", nil, "agent", "build-1", "astro-ns-0", "agent",
		`{"interfaces":{"adapters":["web"],"image":"img","resources":{}}}`, nil, nil,
		"active", nil, nil, now, nil,
		now, nil, nil)
	mock.ExpectQuery("SELECT .+ FROM deployments").WillReturnRows(rows)

	// K8s client that returns empty ingress list (shouldn't even be called for non-OIDC)
	k8sClient := newTestK8sClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("K8s API should not be called for non-OIDC deployments")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"apiVersion":"networking.k8s.io/v1","kind":"IngressList","items":[]}`)
	}))

	w := &ReconcileWorker{
		deployer: &deployer.Deployer{Cfg: cfg},
		store:    store,
		k8s:      k8sClient,
		log:      logger.New("error", "json"),
	}

	w.reconcileOIDCIssuer(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

func TestReconcileOIDCIssuer_NoReapplyWhenConfigMatches(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	cfg := &config.Config{}
	cfg.Deployment.MessagingOIDCIssuer = "https://login.example.com"
	cfg.Deployment.MessagingOIDCAuthEndpoint = "https://login.example.com/authorize"
	cfg.Deployment.MessagingOIDCTokenEndpoint = "https://login.example.com/token"
	cfg.Deployment.MessagingOIDCUserInfoEndpoint = "https://login.example.com/userinfo"
	cfg.Deployment.MessagingOIDCClientID = "client-id-123"
	cfg.Deployment.MessagingOIDCClientSecret = "client-secret-456"
	cfg.Deployment.MessagingOIDCSessionTimeout = 3600

	oidcSpec := `{"interfaces":{"adapters":["web"],"image":"img","resources":{},"auth":{"web":{"type":"oidc"}}}}`
	rows := sqlmock.NewRows(testDeployColumns)
	now := time.Now()
	rows.AddRow("dep-1", "acct-1", nil, "agent", "build-1", "astro-ns-0", "agent",
		oidcSpec, nil, nil,
		"active", nil, nil, now, nil,
		now, nil, nil)
	mock.ExpectQuery("SELECT .+ FROM deployments").WillReturnRows(rows)

	// Build the exact annotation that the reconciler expects
	expectedAnnotation := buildExpectedOIDCAnnotation(
		"https://login.example.com",
		"https://login.example.com/authorize",
		"https://login.example.com/token",
		"https://login.example.com/userinfo",
	)

	// K8s fake: serve ingress list and secret GET
	k8sClient := newTestK8sClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/ingresses"):
			escapedAnnotation, _ := json.Marshal(expectedAnnotation)
			fmt.Fprintf(w, `{
				"apiVersion":"networking.k8s.io/v1","kind":"IngressList",
				"items":[{"metadata":{"name":"agent-web","namespace":"astro-ns-0","annotations":{
					"alb.ingress.kubernetes.io/auth-idp-oidc":%s,
					"alb.ingress.kubernetes.io/auth-session-timeout":"3600"
				}},"spec":{"rules":[]}}]
			}`, string(escapedAnnotation))
		case strings.Contains(r.URL.Path, "/secrets/messaging-oidc"):
			fmt.Fprint(w, `{
				"apiVersion":"v1","kind":"Secret","metadata":{"name":"messaging-oidc","namespace":"astro-ns-0"},
				"data":{"clientId":"Y2xpZW50LWlkLTEyMw==","clientSecret":"Y2xpZW50LXNlY3JldC00NTY="}
			}`)
		default:
			http.NotFound(w, r)
		}
	}))

	w := &ReconcileWorker{
		deployer: &deployer.Deployer{Cfg: cfg},
		store:    store,
		k8s:      k8sClient,
		log:      logger.New("error", "json"),
	}

	w.reconcileOIDCIssuer(t.Context())

	// No UpdateStatus or InsertDeployJob calls expected — all config matches
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

// ---------------------------------------------------------------------------
// Pod-failure escalation tests
// ---------------------------------------------------------------------------

func makeWaitingPod(name string, age time.Duration, reason string, restartCount int32, image string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "main",
				Image:        image,
				RestartCount: restartCount,
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
					Reason: reason,
				}},
			}},
		},
	}
}

func TestClassifyPodFailure(t *testing.T) {
	tests := []struct {
		name       string
		pod        corev1.Pod
		wantReason string
	}{
		{
			name:       "ImagePullBackOff after grace → escalate",
			pod:        makeWaitingPod("p1", 5*time.Minute, "ImagePullBackOff", 0, "img:bad"),
			wantReason: "ImagePullBackOff",
		},
		{
			name:       "ImagePullBackOff inside grace → wait",
			pod:        makeWaitingPod("p1", 30*time.Second, "ImagePullBackOff", 0, "img:bad"),
			wantReason: "",
		},
		{
			name:       "ErrImagePull after grace → escalate",
			pod:        makeWaitingPod("p1", 3*time.Minute, "ErrImagePull", 0, "img:bad"),
			wantReason: "ErrImagePull",
		},
		{
			name:       "InvalidImageName after grace → escalate",
			pod:        makeWaitingPod("p1", 3*time.Minute, "InvalidImageName", 0, "img:bad"),
			wantReason: "InvalidImageName",
		},
		{
			name:       "CrashLoopBackOff with 3 restarts → wait",
			pod:        makeWaitingPod("p1", 10*time.Minute, "CrashLoopBackOff", 3, "img:ok"),
			wantReason: "",
		},
		{
			name:       "CrashLoopBackOff with 6 restarts → escalate",
			pod:        makeWaitingPod("p1", 10*time.Minute, "CrashLoopBackOff", 6, "img:ok"),
			wantReason: "CrashLoopBackOff",
		},
		{
			name:       "PodInitializing is transient → no escalation",
			pod:        makeWaitingPod("p1", 10*time.Minute, "PodInitializing", 0, "img:ok"),
			wantReason: "",
		},
		{
			name:       "no waiting state → no escalation",
			pod:        corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}}},
			wantReason: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := classifyPodFailure(tt.pod)
			if r != tt.wantReason {
				t.Errorf("reason=%q, want %q", r, tt.wantReason)
			}
		})
	}
}

// expectedManagedByLabelSelector mirrors managedByAstroLabelSelector in
// reconcile.go. Hard-coded here so a refactor that drops the selector breaks
// the test instead of silently regressing the cluster-wide list cost.
const expectedManagedByLabelSelector = "app.kubernetes.io/managed-by=astro-server"

// k8sPodListHandler returns an http.Handler that serves a single
// cluster-wide pod list (matching the LabelSelector the production code uses
// to fan out across all astro-managed pods in one round-trip). It also
// asserts that the request actually carries that selector so any future
// regression that drops it fails this test instead of silently fanning out
// across every pod in the cluster. The namespace argument is preserved in
// the API to keep call sites readable but is only used for asserting test
// intent.
func k8sPodListHandler(t *testing.T, _ string, podJSON string) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/pods", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("labelSelector"); got != expectedManagedByLabelSelector {
			t.Errorf("labelSelector: want %q, got %q", expectedManagedByLabelSelector, got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"apiVersion":"v1","kind":"PodList","items":[%s]}`, podJSON)
	})
	return mux
}

func imagePullBackOffPodJSON(name, image string, ageSeconds int) string {
	created := time.Now().Add(-time.Duration(ageSeconds) * time.Second).UTC().Format(time.RFC3339)
	return fmt.Sprintf(`{
		"metadata": {"name": %q, "namespace": "astro-stuck-0", "creationTimestamp": %q},
		"status": {
			"containerStatuses": [{
				"name": "main",
				"image": %q,
				"restartCount": 0,
				"state": {"waiting": {"reason": "ImagePullBackOff", "message": "Back-off pulling image"}}
			}]
		}
	}`, name, created, image)
}

func TestEscalatePodFailures_StuckOnImagePull_MarksFailed(t *testing.T) {
	const ns = "astro-stuck-0"
	podJSON := imagePullBackOffPodJSON("agent-abc", "img:missing", 600)
	k8sClient := newTestK8sClient(k8sPodListHandler(t, ns, podJSON))

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	rows := sqlmock.NewRows(testDeployColumns)
	addDeployRow(rows, "dep-stuck", ns, "active")
	mock.ExpectQuery("SELECT .+ FROM deployments").WillReturnRows(rows)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE deployments").
		WithArgs("dep-stuck", "failed", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deployment_events").
		WithArgs("dep-stuck", "failed", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		log:   logger.New("error", "json"),
	}

	w.escalatePodFailures(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

func TestEscalatePodFailures_FreshPod_NoEscalation(t *testing.T) {
	const ns = "astro-stuck-0"
	podJSON := imagePullBackOffPodJSON("agent-abc", "img:slow", 30)
	k8sClient := newTestK8sClient(k8sPodListHandler(t, ns, podJSON))

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	rows := sqlmock.NewRows(testDeployColumns)
	addDeployRow(rows, "dep-fresh", ns, "active")
	mock.ExpectQuery("SELECT .+ FROM deployments").WillReturnRows(rows)
	// No UPDATE expected — pod isn't old enough.

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		log:   logger.New("error", "json"),
	}

	w.escalatePodFailures(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

func TestEscalatePodFailures_OnlyScansLiveDeployments(t *testing.T) {
	mux := http.NewServeMux()
	called := false
	mux.HandleFunc("/api/v1/pods", func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})
	k8sClient := newTestK8sClient(mux)

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	// Returning zero rows simulates the SQL filter
	// (GetDeploymentsInStatus(active, prov, pending)) excluding everything;
	// the worker should skip the pod list entirely when there are no live
	// deployments to escalate.
	mock.ExpectQuery("SELECT .+ FROM deployments WHERE status").
		WillReturnRows(sqlmock.NewRows(testDeployColumns))

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		log:   logger.New("error", "json"),
	}

	w.escalatePodFailures(t.Context())

	if called {
		t.Error("pod list called when no live deployments; expected early return")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

// Multi-namespace fan-in: a single cluster-wide pod list returns pods from
// two distinct namespaces. Only the deployment whose namespace contains the
// failing pod should be escalated; the healthy-namespace deployment must be
// left alone. This is the load-bearing behavior of the cluster-wide list
// optimization (one round-trip across N namespaces).
func TestEscalatePodFailures_MultiNamespaceFanIn(t *testing.T) {
	const stuckNS = "astro-stuck-0"
	const healthyNS = "astro-healthy-0"

	stuckPod := imagePullBackOffPodJSONWithNamespace("agent-stuck", "img:missing", 600, stuckNS)
	healthyPod := fmt.Sprintf(`{
		"metadata": {"name": "agent-healthy", "namespace": %q},
		"status": {
			"phase": "Running",
			"containerStatuses": [{
				"name": "main",
				"image": "img:ok",
				"state": {"running": {"startedAt": %q}}
			}]
		}
	}`, healthyNS, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339))
	bothPods := stuckPod + "," + healthyPod

	k8sClient := newTestK8sClient(k8sPodListHandler(t, "", bothPods))

	db, mock, _ := sqlmock.New()
	store := deploymentstore.NewStore(db)

	rows := sqlmock.NewRows(testDeployColumns)
	addDeployRow(rows, "dep-stuck", stuckNS, "active")
	addDeployRow(rows, "dep-healthy", healthyNS, "active")
	mock.ExpectQuery("SELECT .+ FROM deployments").WillReturnRows(rows)

	// Only the stuck deployment should be flipped to failed; the healthy one
	// must not generate any DB writes.
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE deployments").
		WithArgs("dep-stuck", "failed", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deployment_events").
		WithArgs("dep-stuck", "failed", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	w := &ReconcileWorker{
		store: store,
		k8s:   k8sClient,
		log:   logger.New("error", "json"),
	}

	w.escalatePodFailures(t.Context())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled DB expectations: %v", err)
	}
}

// imagePullBackOffPodJSONWithNamespace is a small generalization of
// imagePullBackOffPodJSON that lets the caller place the pod in an arbitrary
// namespace, used by the multi-namespace fan-in test.
func imagePullBackOffPodJSONWithNamespace(name, image string, ageSeconds int, namespace string) string {
	created := time.Now().Add(-time.Duration(ageSeconds) * time.Second).UTC().Format(time.RFC3339)
	return fmt.Sprintf(`{
		"metadata": {"name": %q, "namespace": %q, "creationTimestamp": %q},
		"status": {
			"containerStatuses": [{
				"name": "main",
				"image": %q,
				"restartCount": 0,
				"state": {"waiting": {"reason": "ImagePullBackOff", "message": "Back-off pulling image"}}
			}]
		}
	}`, name, namespace, created, image)
}

// groupPodsByNamespace must drop pods that are mid-deletion or have
// already finished successfully so a terminating-but-still-CrashLooping pod
// from a previous rollout does not cause the next rollout to escalate.
func TestGroupPodsByNamespace_FiltersTerminatingAndSucceeded(t *testing.T) {
	now := metav1.NewTime(time.Now().Add(-time.Hour))
	deletionTime := metav1.Now()

	terminating := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "terminating",
			Namespace:         "ns-a",
			CreationTimestamp: now,
			DeletionTimestamp: &deletionTime,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
			}},
		},
	}
	succeeded := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "succeeded", Namespace: "ns-a"},
		Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
	}
	live := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "live", Namespace: "ns-b"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	got := groupPodsByNamespace([]corev1.Pod{terminating, succeeded, live})

	if pods := got["ns-a"]; len(pods) != 0 {
		t.Errorf("ns-a should be empty after filtering terminating+Succeeded, got %d pods", len(pods))
	}
	if pods := got["ns-b"]; len(pods) != 1 || pods[0].Name != "live" {
		t.Errorf("ns-b should contain only the live pod, got %+v", pods)
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
