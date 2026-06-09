//go:build integration

package e2e

import (
	"context"
	"database/sql"
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
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// fakeClusterCA is the minimum PEM blob clusterstore accepts for registration.
var fakeClusterCA = []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n")

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

func registerTestCluster(t *testing.T, db *sql.DB, store *clusterstore.Store, id string, enabled bool) {
	t.Helper()
	ctx := context.Background()
	// Remove leftovers from interrupted prior runs so Register is idempotent.
	_, _ = db.ExecContext(ctx, `DELETE FROM clusters WHERE id = $1`, id)
	err := store.Register(ctx, &clusterstore.Cluster{
		ID:                     id,
		Region:                 "us-east-1",
		EKSClusterName:         "e2e-unreachable-eks",
		EKSClusterEndpoint:     "https://e2e-unreachable.example",
		EKSClusterCA:           fakeClusterCA,
		Enabled:                enabled,
		AgentIngressDomain:     "agents.e2e.example.com",
		AgentACMCertARN:        "arn:aws:acm:us-east-1:000000000000:certificate/e2e-agent",
		AgentALBGroupName:      "e2e-agents",
		IngestionIngressDomain: "ingestion.e2e.example.com",
		IngestionACMCertARN:    "arn:aws:acm:us-east-1:000000000000:certificate/e2e-ingestion",
		IngestionALBGroupName:  "e2e-ingestion",
		KnowledgeDomain:        "knowledge.e2e.example.com",
		LangfuseBaseURLExt:     "http://langfuse.e2e.example:3000",
		LangfuseVPCEIPs:        "10.0.0.10",
		PodSubnetCIDRs:         "10.0.0.0/24",
	})
	if err != nil {
		t.Fatalf("Register(%q): %v", id, err)
	}
	if !enabled {
		if err := store.SetEnabled(ctx, id, false); err != nil {
			t.Fatalf("SetEnabled(%q, false): %v", id, err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM clusters WHERE id = $1`, id)
	})
}

func TestClusterStore_DisabledClusterRejected(t *testing.T) {
	db := testDB(t)
	store := clusterstore.New(db)

	const id = "e2e-test-disabled"
	registerTestCluster(t, db, store, id, false)

	row, err := store.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if row.Enabled {
		t.Fatal("expected cluster to be disabled")
	}
}

func TestUndeployUnreachableAdditionalClusterMarksUndeployed(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	deployStore := ds.NewStore(db)
	clusterStore := clusterstore.New(db)

	// Row must exist (FK); disabled so registry.Get returns ErrClusterDisabled
	// without dialing AWS/EKS.
	const clusterID = "e2e-disabled-secondary"
	registerTestCluster(t, db, clusterStore, clusterID, false)

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

	log := logger.New("error", "json")
	reg := k8s.NewRegistryWithClusterStore(newNoopPrimaryClusterClient(t), clusterStore, log)
	depWorker := &deployer.Deployer{Registry: reg}

	teardownErr := depWorker.Teardown(context.Background(), dep)
	if !errors.Is(teardownErr, deployer.ErrClusterClientUnavailable) {
		t.Fatalf("Teardown = %v, want ErrClusterClientUnavailable", teardownErr)
	}

	if err := deployStore.ClearScaledDown(dep.Namespace); err != nil {
		t.Fatalf("ClearScaledDown: %v", err)
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
