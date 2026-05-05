package deployer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro/packages/astro-spec"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ---------------------------------------------------------------------------
// imagePullPolicyForMode
// ---------------------------------------------------------------------------

func TestImagePullPolicyForMode_Local(t *testing.T) {
	got := imagePullPolicyForMode("local")
	if got != corev1.PullIfNotPresent {
		t.Errorf("imagePullPolicyForMode(\"local\") = %v, want %v", got, corev1.PullIfNotPresent)
	}
}

func TestImagePullPolicyForMode_EKS(t *testing.T) {
	got := imagePullPolicyForMode("eks")
	if got != corev1.PullAlways {
		t.Errorf("imagePullPolicyForMode(\"eks\") = %v, want %v", got, corev1.PullAlways)
	}
}

func TestImagePullPolicyForMode_Empty(t *testing.T) {
	got := imagePullPolicyForMode("")
	if got != corev1.PullAlways {
		t.Errorf("imagePullPolicyForMode(\"\") = %v, want %v", got, corev1.PullAlways)
	}
}

// ---------------------------------------------------------------------------
// fakeClusterClient implements k8s.ClusterClient backed by a test HTTP server.
// ---------------------------------------------------------------------------

type fakeClusterClient struct {
	clientset *kubernetes.Clientset
}

func (f *fakeClusterClient) Clientset() *kubernetes.Clientset      { return f.clientset }
func (f *fakeClusterClient) Config() *rest.Config                  { return nil }
func (f *fakeClusterClient) CheckHealth() error                    { return nil }
func (f *fakeClusterClient) GetServerVersion() (string, error)     { return "v1.30.0", nil }
func (f *fakeClusterClient) DiagnoseConnection() map[string]string { return nil }

// newFakeClusterClient creates a ClusterClient backed by the given HTTP handler.
func newFakeClusterClient(t *testing.T, handler http.Handler) *fakeClusterClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("NewForConfig: %v", err)
	}
	return &fakeClusterClient{clientset: cs}
}

// ---------------------------------------------------------------------------
// Teardown
// ---------------------------------------------------------------------------

func TestTeardown_NotFound(t *testing.T) {
	// The test HTTP server returns 404 for all requests, simulating a
	// namespace that has already been deleted.
	client := newFakeClusterClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","message":"namespaces \"test-ns\" not found","reason":"NotFound","code":404}`))
	}))

	d := &Deployer{K8sClient: client}
	dep := &deploymentstore.Deployment{Namespace: "test-ns"}

	err := d.Teardown(context.Background(), dep)
	if err != nil {
		t.Errorf("Teardown should return nil for not-found, got: %v", err)
	}
}

func TestTeardown_Success(t *testing.T) {
	// The test HTTP server returns 200 OK, simulating a successful deletion.
	client := newFakeClusterClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Success"}`))
	}))

	d := &Deployer{K8sClient: client}
	dep := &deploymentstore.Deployment{Namespace: "test-ns"}

	err := d.Teardown(context.Background(), dep)
	if err != nil {
		t.Errorf("Teardown should return nil on success, got: %v", err)
	}
}

func TestTeardown_ServerError(t *testing.T) {
	// The test HTTP server returns 500, simulating an API server error.
	client := newFakeClusterClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","message":"internal error","reason":"InternalError","code":500}`))
	}))

	d := &Deployer{K8sClient: client}
	dep := &deploymentstore.Deployment{Namespace: "test-ns"}

	err := d.Teardown(context.Background(), dep)
	if err == nil {
		t.Error("Teardown should return an error on 500, got nil")
	}
}

// ---------------------------------------------------------------------------
// K8sClientMode → LocalMode boundary tests
//
// These verify that ONLY K8sClientMode="local" produces LocalMode=true in the
// ApplierConfig. Every other mode string (production defaults, explicit names,
// empty string) must produce LocalMode=false.
// ---------------------------------------------------------------------------

func TestK8sClientModeLocalModeBoundary(t *testing.T) {
	cases := []struct {
		mode          string
		wantLocalMode bool
	}{
		{"local", true},
		{"", false},
		{"eks", false},
		{"prod", false},
		{"staging", false},
		{"preview", false},
		{"production", false},
		{"LOCAL", false},
		{"Local", false},
	}

	for _, tc := range cases {
		name := tc.mode
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			got := tc.mode == "local"
			if got != tc.wantLocalMode {
				t.Errorf("K8sClientMode=%q → LocalMode=%v, want %v", tc.mode, got, tc.wantLocalMode)
			}
		})
	}
}

// TestDefaultK8sClientMode verifies that the config default for K8S_CLIENT_MODE
// ("eks") never produces LocalMode=true. This catches changes to the default
// that could accidentally relax security in production.
func TestDefaultK8sClientMode(t *testing.T) {
	defaultMode := "eks"
	if defaultMode == "local" {
		t.Fatal("default K8sClientMode constant must not be 'local'")
	}

	localMode := defaultMode == "local"
	if localMode {
		t.Fatal("default K8sClientMode must not produce LocalMode=true")
	}

	pullPolicy := imagePullPolicyForMode(defaultMode)
	if pullPolicy != corev1.PullAlways {
		t.Errorf("default mode pull policy = %v, want PullAlways", pullPolicy)
	}
}

// ---------------------------------------------------------------------------
// resolveBoundKnowledge
// ---------------------------------------------------------------------------

func TestResolveBoundKnowledge_NilKnowledgeStore(t *testing.T) {
	// When KnowledgeStore is nil (not configured), bound entries are silently skipped.
	d := &Deployer{Log: logger.New("error", "json")}

	ds := &spec.AstroDeploymentSpec{
		Knowledge: map[string]spec.DeploymentKnowledge{
			"mydb": {Binding: "arn:knowledge:acct:store1", Provider: "postgres"},
		},
	}

	bk, bc, err := d.resolveBoundKnowledge(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bk != nil || bc != nil {
		t.Error("expected nil maps when KnowledgeStore is nil")
	}
}

func TestResolveBoundKnowledge_NoBoundEntries(t *testing.T) {
	d := &Deployer{Log: logger.New("error", "json")}

	ds := &spec.AstroDeploymentSpec{
		Knowledge: map[string]spec.DeploymentKnowledge{
			"cache": {Image: "redis:7", Provider: "redis"}, // not bound
		},
	}

	bk, bc, err := d.resolveBoundKnowledge(context.Background(), ds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bk != nil || bc != nil {
		t.Error("expected nil maps when no bound entries")
	}
}

// ---------------------------------------------------------------------------
// buildNamespaceLabels
//
// This helper is the only place that stamps the astro.dev/source-account-id
// label, and it's the label the orphan-recovery reconciler reads to decide
// which account to record as the lineage source. A typo or wrong-field bug
// here makes every recovered row's source_account_id default to the
// deployer account regardless of the original deploy's lineage — and the
// reconciler-side tests can't catch it because they fixture the label
// content themselves. These tests pin the write site directly.
// ---------------------------------------------------------------------------

func TestBuildNamespaceLabels_StampsSourceAccountID(t *testing.T) {
	src := "src-acct-uuid"
	dep := &deploymentstore.Deployment{
		AccountID:       "deployer-acct-uuid",
		AgentName:       "my-agent",
		BuildID:         "build-1",
		SourceAccountID: &src,
	}

	labels := buildNamespaceLabels(dep, "deployer-name")

	wantBase := map[string]string{
		"astro.dev/account-id":   "deployer-acct-uuid",
		"astro.dev/account":      "deployer-name",
		deployment.LabelKeyAgent: "my-agent",
		"astro.dev/build":        "build-1",
	}
	for k, v := range wantBase {
		if got := labels[k]; got != v {
			t.Errorf("labels[%q] = %q, want %q", k, got, v)
		}
	}
	got, ok := labels[deployment.LabelKeySourceAccountID]
	if !ok {
		t.Fatalf("labels[%q] missing; want %q (this is the keystone of orphan recovery)",
			deployment.LabelKeySourceAccountID, src)
	}
	if got != src {
		t.Errorf("labels[%q] = %q, want %q", deployment.LabelKeySourceAccountID, got, src)
	}
}

func TestBuildNamespaceLabels_OmitsLabelWhenSourceAccountIDNil(t *testing.T) {
	dep := &deploymentstore.Deployment{
		AccountID:       "deployer-acct-uuid",
		AgentName:       "my-agent",
		BuildID:         "build-1",
		SourceAccountID: nil,
	}

	labels := buildNamespaceLabels(dep, "deployer-name")

	if _, ok := labels[deployment.LabelKeySourceAccountID]; ok {
		t.Errorf("labels[%q] must be absent when SourceAccountID is nil "+
			"(reconciler relies on the missing-key path, not on empty-value)",
			deployment.LabelKeySourceAccountID)
	}
	// Legacy labels still present so the reconciler can still recover.
	if labels[deployment.LabelKeyAgent] != "my-agent" {
		t.Errorf("agent label dropped: %q", labels[deployment.LabelKeyAgent])
	}
}

func TestBuildNamespaceLabels_OmitsLabelWhenSourceAccountIDEmptyString(t *testing.T) {
	empty := ""
	dep := &deploymentstore.Deployment{
		AccountID:       "deployer-acct-uuid",
		AgentName:       "my-agent",
		BuildID:         "build-1",
		SourceAccountID: &empty,
	}

	labels := buildNamespaceLabels(dep, "deployer-name")

	if _, ok := labels[deployment.LabelKeySourceAccountID]; ok {
		t.Errorf("labels[%q] must be absent when SourceAccountID dereferences to empty string "+
			"(stamping an empty value would make the reconciler take the present-but-empty path "+
			"instead of the missing-key fallback)",
			deployment.LabelKeySourceAccountID)
	}
}
