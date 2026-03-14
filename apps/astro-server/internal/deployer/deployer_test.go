package deployer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ---------------------------------------------------------------------------
// imagePullPolicyForMode
// ---------------------------------------------------------------------------

func TestImagePullPolicyForMode_Local(t *testing.T) {
	got := imagePullPolicyForMode("local")
	if got != corev1.PullNever {
		t.Errorf("imagePullPolicyForMode(\"local\") = %v, want %v", got, corev1.PullNever)
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
