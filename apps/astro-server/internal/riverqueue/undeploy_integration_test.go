//go:build integration

package riverqueue

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/riverqueue/river"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	_ "github.com/lib/pq"
)

type integrationPrimaryClient struct {
	cs *kubernetes.Clientset
}

func (c *integrationPrimaryClient) Clientset() *kubernetes.Clientset      { return c.cs }
func (c *integrationPrimaryClient) Config() *rest.Config                  { return nil }
func (c *integrationPrimaryClient) CheckHealth() error                    { return nil }
func (c *integrationPrimaryClient) GetServerVersion() (string, error)     { return "v1.30.0", nil }
func (c *integrationPrimaryClient) DiagnoseConnection() map[string]string { return nil }

func integrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must be set for integration tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	return db
}

func TestUndeployWorker_Integration_UnreachableCluster(t *testing.T) {
	db := integrationDB(t)
	deployStore := deploymentstore.NewStore(db)
	clusterStore := clusterstore.New(db)

	const clusterID = "e2e-worker-unreachable"
	ctx := context.Background()
	fakeCA := []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----\n")
	if err := clusterStore.UpsertFromConfig(ctx, &clusterstore.Cluster{
		ID:                     clusterID,
		Region:                 "us-east-1",
		EKSClusterName:         "e2e-no-access",
		EKSClusterEndpoint:     "https://e2e-no-access.example",
		EKSClusterCA:           fakeCA,
		AgentIngressDomain:     "agents.e2e.example.com",
		IngestionIngressDomain: "ingestion.e2e.example.com",
		LangfuseBaseURLExt:     "http://langfuse.e2e.example:3000",
		PodSubnetCIDRs:         "10.0.0.0/24",
	}, false); err != nil {
		t.Fatalf("UpsertFromConfig: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM clusters WHERE id = $1`, clusterID) })

	var accountID string
	if err := db.QueryRow(`INSERT INTO accounts (name, type) VALUES ('undeploy-worker-e2e', 'personal') ON CONFLICT DO NOTHING RETURNING id`).Scan(&accountID); err != nil {
		_ = db.QueryRow(`SELECT id FROM accounts WHERE name = 'undeploy-worker-e2e'`).Scan(&accountID)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM deployments WHERE account_id = $1`, accountID) })

	dep, err := deployStore.SaveDeploymentPending(deploymentstore.SaveDeploymentParams{
		ID: deployid.New(), AccountID: accountID, AgentName: "worker-e2e",
		DisplayName: "worker-e2e", BuildID: "b1", Namespace: "astro-workere2e-0",
		SpecJSON: `{"spec":"deployment/v1"}`, ClusterID: clusterID,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}
	if err := deployStore.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusUndeploying}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("NewForConfig: %v", err)
	}

	log := logger.New("error", "json")
	// No clusterstore attached: Get(clusterID) returns ErrClusterNotFound
	// deterministically, regardless of what's actually stored above.
	reg := k8s.NewRegistryWithPrimary(&integrationPrimaryClient{cs: cs})
	w := &UndeployWorker{
		deployer: &deployer.Deployer{Registry: reg},
		store:    deployStore,
		log:      log,
		cache:    k8scache.NoopCache{},
	}

	if err := w.Work(ctx, &river.Job[UndeployArgs]{Args: UndeployArgs{DeploymentID: dep.ID, ClusterID: clusterID}}); err != nil {
		t.Fatalf("Work: %v", err)
	}

	updated, err := deployStore.GetDeploymentByID(dep.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID: %v", err)
	}
	if updated.Status != deploymentstore.StatusUndeployed {
		t.Fatalf("status = %q, want undeployed", updated.Status)
	}
}
