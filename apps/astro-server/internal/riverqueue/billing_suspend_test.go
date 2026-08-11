package riverqueue

import (
	"context"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
)

// primaryClient stands in for the primary cluster. Only identity matters here;
// the resolver never calls through it.
type primaryClient struct{}

func (primaryClient) Clientset() *kubernetes.Clientset      { return nil }
func (primaryClient) Config() *rest.Config                  { return nil }
func (primaryClient) CheckHealth() error                    { return nil }
func (primaryClient) GetServerVersion() (string, error)     { return "", nil }
func (primaryClient) DiagnoseConnection() map[string]string { return nil }

// A deployment row with no cluster_id lives on the primary cluster. Registry.Get
// rejects an empty id, so without the fallback suspension silently no-ops with
// "registry.Get: empty cluster id" and the account keeps running unpaid. That
// is what preview did, where 23 of 24 active rows carry no cluster_id.
func TestSuspendClusterClient_DefaultsWhenRowHasNoCluster(t *testing.T) {
	want := primaryClient{}
	reg := k8s.NewRegistryWithPrimary(want)

	got, err := suspendClusterClient(context.Background(), reg, &deploymentstore.Deployment{ID: "dep-1"})
	if err != nil {
		t.Fatalf("suspendClusterClient: %v", err)
	}
	if got != k8s.ClusterClient(want) {
		t.Errorf("client = %v, want the primary cluster", got)
	}
}

// No primary and no cluster_id is a real misconfiguration, not something to
// resolve to nil and dereference later.
func TestSuspendClusterClient_ErrorsWithoutPrimary(t *testing.T) {
	reg := k8s.NewRegistryWithPrimary(nil)

	if _, err := suspendClusterClient(context.Background(), reg, &deploymentstore.Deployment{ID: "dep-1"}); err == nil {
		t.Fatal("err is nil; a missing primary must not resolve to a nil client")
	}
}
