//go:build k8s

// Teardown (undeploy) integration tests — requires both Postgres (DATABASE_URL)
// and a real K8s cluster (KUBECONFIG). These tests verify the full destroy loop:
// create namespace + deployment record → teardown → verify namespace deleted and
// DB status updated correctly.
//
// Run: go test -tags k8s -race -timeout 5m ./e2e/...
// Requires: KUBECONFIG + DATABASE_URL
package e2e

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	_ "github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type teardownTestEnv struct {
	t        *testing.T
	db       *sql.DB
	store    *ds.Store
	client   k8s.ClusterClient
	deployer *deployer.Deployer
}

func setupTeardownEnv(t *testing.T) *teardownTestEnv {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping teardown integration test")
	}
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("KUBECONFIG not set — skipping teardown integration test")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	client, err := k8s.NewClusterClient(context.Background(), k8s.ClusterClientConfig{
		Mode:           k8s.ClientModeLocal,
		KubeconfigPath: kubeconfig,
	})
	if err != nil {
		t.Fatalf("NewClusterClient: %v", err)
	}

	store := ds.NewStore(db)
	dep := &deployer.Deployer{Registry: k8s.NewRegistryWithPrimary(client)}

	return &teardownTestEnv{
		t: t, db: db, store: store, client: client, deployer: dep,
	}
}

func (e *teardownTestEnv) ensureTestAccount() string {
	e.t.Helper()
	var accountID string
	err := e.db.QueryRow(`
		INSERT INTO accounts (name, type) VALUES ('teardown-e2e', 'personal')
		ON CONFLICT DO NOTHING RETURNING id
	`).Scan(&accountID)
	if err != nil {
		err = e.db.QueryRow(`SELECT id FROM accounts WHERE name = 'teardown-e2e'`).Scan(&accountID)
		if err != nil {
			e.t.Fatalf("get test account: %v", err)
		}
	}
	return accountID
}

// createNamespaceAndDeployment creates a K8s namespace and a matching DB deployment record.
func (e *teardownTestEnv) createNamespaceAndDeployment(accountID, agentName, status string) *ds.Deployment {
	e.t.Helper()
	depID := deployid.New()
	ns := "astro-" + deployid.Compact(depID) + "-0"

	// Create K8s namespace
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := e.client.Clientset().CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "astro-server",
				"astro.dev/account-id":         accountID,
				"astro.dev/agent":              agentName,
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		e.t.Fatalf("create namespace %s: %v", ns, err)
	}
	e.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = e.client.Clientset().CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
	})

	// Create DB deployment record
	_, err = e.db.Exec(`
		INSERT INTO deployments (id, account_id, agent_name, build_id, namespace,
		    deployment_spec_json, status, status_changed_at, deployed_at)
		VALUES ($1, $2, $3, 'b1', $4, '{}', $5, NOW(), NOW())
	`, depID, accountID, agentName, ns, status)
	if err != nil {
		e.t.Fatalf("insert deployment: %v", err)
	}
	e.t.Cleanup(func() {
		_, _ = e.db.Exec("DELETE FROM deployments WHERE id = $1", depID)
	})

	dep, err := e.store.GetDeploymentByID(depID)
	if err != nil || dep == nil {
		e.t.Fatalf("get deployment after insert: %v", err)
	}
	return dep
}

func (e *teardownTestEnv) namespaceExists(ns string) bool {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := e.client.Clientset().CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}
		e.t.Fatalf("get namespace %s: %v", ns, err)
	}
	return true
}

func (e *teardownTestEnv) waitForNamespaceDeletion(ns string, timeout time.Duration) bool {
	e.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !e.namespaceExists(ns) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// TestTeardown_DeletesNamespace verifies that Teardown removes the K8s namespace.
func TestTeardown_DeletesNamespace(t *testing.T) {
	env := setupTeardownEnv(t)
	accountID := env.ensureTestAccount()
	dep := env.createNamespaceAndDeployment(accountID, "teardown-basic", "undeploying")

	if !env.namespaceExists(dep.Namespace) {
		t.Fatal("namespace should exist before teardown")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := env.deployer.Teardown(ctx, dep); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if !env.waitForNamespaceDeletion(dep.Namespace, 60*time.Second) {
		t.Errorf("namespace %s still exists after teardown", dep.Namespace)
	}
}

// TestTeardown_Idempotent verifies that calling Teardown twice doesn't error
// (namespace already gone on second call).
func TestTeardown_Idempotent(t *testing.T) {
	env := setupTeardownEnv(t)
	accountID := env.ensureTestAccount()
	dep := env.createNamespaceAndDeployment(accountID, "teardown-idem", "undeploying")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := env.deployer.Teardown(ctx, dep); err != nil {
		t.Fatalf("first Teardown: %v", err)
	}

	if !env.waitForNamespaceDeletion(dep.Namespace, 60*time.Second) {
		t.Fatalf("namespace %s still exists after first teardown", dep.Namespace)
	}

	// Second call should be a no-op (NotFound → nil)
	if err := env.deployer.Teardown(ctx, dep); err != nil {
		t.Errorf("second Teardown should be idempotent, got: %v", err)
	}
}

// TestTeardown_StatusTransition verifies the full undeploy status lifecycle:
// undeploying → teardown → undeployed.
func TestTeardown_StatusTransition(t *testing.T) {
	env := setupTeardownEnv(t)
	accountID := env.ensureTestAccount()
	dep := env.createNamespaceAndDeployment(accountID, "teardown-status", "undeploying")

	// Verify initial status
	if dep.Status != ds.StatusUndeploying {
		t.Fatalf("initial status = %q, want %q", dep.Status, ds.StatusUndeploying)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := env.deployer.Teardown(ctx, dep); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	// Mark as undeployed (what the UndeployWorker does after teardown)
	if err := env.store.UpdateStatus(dep.ID, ds.StatusUpdate{Status: ds.StatusUndeployed}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	updated, err := env.store.GetDeploymentByID(dep.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID: %v", err)
	}
	if updated.Status != ds.StatusUndeployed {
		t.Errorf("status after teardown = %q, want %q", updated.Status, ds.StatusUndeployed)
	}

	// Undeployed deployments should NOT appear in live status queries
	deps, err := env.store.GetDeploymentsInStatus(
		ds.StatusActive, ds.StatusFailed, ds.StatusPending,
		ds.StatusProvisioning, ds.StatusUndeploying,
	)
	if err != nil {
		t.Fatalf("GetDeploymentsInStatus: %v", err)
	}
	for _, d := range deps {
		if d.ID == dep.ID {
			t.Errorf("undeployed deployment %s should not appear in live status query", dep.ID)
		}
	}
}

// TestTeardown_NamespaceNotInReconcile verifies that after teardown + status update,
// the namespace won't be detected as orphaned by the reconciler.
func TestTeardown_NamespaceNotInReconcile(t *testing.T) {
	env := setupTeardownEnv(t)
	accountID := env.ensureTestAccount()
	dep := env.createNamespaceAndDeployment(accountID, "teardown-reconcile", "undeploying")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := env.deployer.Teardown(ctx, dep); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if err := env.store.UpdateStatus(dep.ID, ds.StatusUpdate{Status: ds.StatusUndeployed}); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	if !env.waitForNamespaceDeletion(dep.Namespace, 60*time.Second) {
		t.Fatalf("namespace %s still exists after teardown", dep.Namespace)
	}

	// Namespace is gone from K8s AND deployment is undeployed in DB.
	// The reconciler lists K8s namespaces with managed-by label — this one
	// is deleted, so it won't appear. Double-check via K8s API.
	nsList, err := env.client.Clientset().CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=astro-server",
	})
	if err != nil {
		t.Fatalf("list namespaces: %v", err)
	}
	for _, ns := range nsList.Items {
		if ns.Name == dep.Namespace {
			t.Errorf("deleted namespace %s still appears in managed namespace list", dep.Namespace)
		}
	}
}
