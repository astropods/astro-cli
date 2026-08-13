//go:build integration

package e2e

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
)

type noopPrimaryClusterClient struct {
	cs *kubernetes.Clientset
}

func (n *noopPrimaryClusterClient) Clientset() *kubernetes.Clientset      { return n.cs }
func (n *noopPrimaryClusterClient) Config() *rest.Config                  { return nil }
func (n *noopPrimaryClusterClient) CheckHealth() error                    { return nil }
func (n *noopPrimaryClusterClient) GetServerVersion() (string, error)     { return "v1.30.0", nil }
func (n *noopPrimaryClusterClient) DiagnoseConnection() map[string]string { return nil }

func newNoopPrimaryClusterClient(t *testing.T) k8s.ClusterClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("NewForConfig: %v", err)
	}
	return &noopPrimaryClusterClient{cs: cs}
}

// TestUndeployUnreachableAdditionalClusterMarksUndeployed covers a deployment
// pinned to a cluster the deployer's registry can't resolve — undeploy must
// still mark the deployment undeployed rather than getting stuck retrying
// forever. The registry has no clusterstore attached, so Get returns
// ErrClusterNotFound deterministically without dialing AWS; the row itself
// still needs to exist to satisfy deployments.cluster_id's FK.
func TestUndeployUnreachableAdditionalClusterMarksUndeployed(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	deployStore := ds.NewStore(db)
	clusterStore := clusterstore.New(db)

	const clusterID = "e2e-unreachable-secondary"
	fakeCA := []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n")
	if err := clusterStore.UpsertFromConfig(context.Background(), &clusterstore.Cluster{
		ID:                     clusterID,
		Region:                 "us-east-1",
		EKSClusterName:         "e2e-unreachable-eks",
		EKSClusterEndpoint:     "https://e2e-unreachable.example",
		EKSClusterCA:           fakeCA,
		AgentIngressDomain:     "agents.e2e.example.com",
		IngestionIngressDomain: "ingestion.e2e.example.com",
		LangfuseBaseURLExt:     "http://langfuse.e2e.example:3000",
		PodSubnetCIDRs:         "10.0.0.0/24",
	}, false); err != nil {
		t.Fatalf("UpsertFromConfig(%q): %v", clusterID, err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	dep, err := deployStore.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID:          deployid.New(),
		AccountID:   accountID,
		AgentName:   "cluster-e2e-agent",
		DisplayName: "cluster-e2e-agent",
		BuildID:     "build-e2e",
		Namespace:   "astro-clustere2e-0",
		SpecJSON:    `{"spec":"deployment/v1"}`,
		ClusterID:   clusterID,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}
	if err := deployStore.UpdateStatus(dep.ID, ds.StatusUpdate{Status: ds.StatusUndeploying}); err != nil {
		t.Fatalf("UpdateStatus undeploying: %v", err)
	}

	dep, err = deployStore.GetDeploymentByID(dep.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID: %v", err)
	}

	// No clusterstore attached: Get(clusterID) returns ErrClusterNotFound
	// deterministically, regardless of what's actually stored above.
	reg := k8s.NewRegistryWithPrimary(newNoopPrimaryClusterClient(t))
	depWorker := &deployer.Deployer{Registry: reg}

	teardownErr := depWorker.Teardown(context.Background(), dep)
	if !errors.Is(teardownErr, deployer.ErrClusterClientUnavailable) {
		t.Fatalf("Teardown = %v, want ErrClusterClientUnavailable", teardownErr)
	}

	if err := deployStore.UpdateStatus(dep.ID, ds.StatusUpdate{Status: ds.StatusUndeployed}); err != nil {
		t.Fatalf("UpdateStatus undeployed: %v", err)
	}

	updated, err := deployStore.GetDeploymentByID(dep.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID after undeploy: %v", err)
	}
	if updated.Status != ds.StatusUndeployed {
		t.Fatalf("status = %q, want %q", updated.Status, ds.StatusUndeployed)
	}

	visible, err := deployStore.GetVisibleDeploymentsByAccount(accountID)
	if err != nil {
		t.Fatalf("GetVisibleDeploymentsByAccount: %v", err)
	}
	for _, d := range visible {
		if d.ID == dep.ID {
			t.Fatalf("undeployed deployment %s still visible in account list", dep.ID)
		}
	}
}
