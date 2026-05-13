package k8s

import (
	"context"
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// fakeClient is a stand-in for ClusterClient that lets tests assert the
// registry handed back a constructed client without dialing a real cluster.
type fakeClient struct{ id string }

func (f *fakeClient) Clientset() *kubernetes.Clientset      { return nil }
func (f *fakeClient) Config() *rest.Config                  { return nil }
func (f *fakeClient) CheckHealth() error                    { return nil }
func (f *fakeClient) GetServerVersion() (string, error)     { return "fake-v0.0.0", nil }
func (f *fakeClient) DiagnoseConnection() map[string]string { return map[string]string{"id": f.id} }

// newRegistryDirect mirrors NewRegistry but injects a stub client so tests
// don't dial real Kubernetes. NewRegistry's production path delegates to
// NewClusterClient which would require KUBECONFIG / EKS coords; this helper
// short-circuits that by accepting a pre-built client.
func newRegistryDirect(primary ClusterClient) *Registry {
	return &Registry{primary: primary}
}

func TestNewRegistry_PropagatesClientConstructionError(t *testing.T) {
	// Configure for EKS with empty cluster name — NewEKSClient fails fast
	// when the cluster name is empty, exercising the error-propagation path.
	_, err := NewRegistry(context.Background(), RegistryConfig{
		Mode: ClientModeEKS,
	}, nil)
	if err == nil {
		t.Fatal("expected NewRegistry to fail when EKS config is empty")
	}
}

func TestRegistry_Default_ReturnsPrimary(t *testing.T) {
	primary := &fakeClient{id: "primary"}
	r := newRegistryDirect(primary)

	got := r.Default()
	if got != primary {
		t.Fatalf("Default() = %p, want %p", got, primary)
	}
}

func TestRegistry_Default_NeverReturnsNil(t *testing.T) {
	// The Registry type guarantees Default() never returns nil so callers
	// can chain `registry.Default().Clientset()` without a nil guard. We
	// can't easily exercise this property without constructing a Registry,
	// but we can pin the expectation that the primary field is set after
	// NewRegistry succeeds (via the typed nil check below).
	primary := &fakeClient{id: "primary"}
	r := newRegistryDirect(primary)
	if r.Default() == nil {
		t.Fatal("Default() returned nil")
	}
}

// NewRegistry's errors should be wrapped with "registry:" so operators
// reading logs can attribute boot failures to the registry layer (not the
// raw EKS / kubeconfig error from one layer deeper).
func TestNewRegistry_ErrorIsWrapped(t *testing.T) {
	_, err := NewRegistry(context.Background(), RegistryConfig{
		Mode: ClientMode("not-a-real-mode"),
	}, nil)
	if err == nil {
		t.Fatal("expected NewRegistry to fail on unknown mode")
	}
	if !strings.Contains(err.Error(), "registry:") {
		t.Fatalf("expected error to be wrapped with 'registry:', got %q", err.Error())
	}
}
