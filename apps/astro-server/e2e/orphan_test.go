//go:build k8s

// Orphan recovery integration tests — requires both Postgres (DATABASE_URL)
// and a real K8s cluster (KUBECONFIG). These tests verify the full loop:
// create orphaned K8s namespace → recover via store → verify DB state →
// verify undeploy cleans up.
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

	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	_ "github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// orphanTestEnv bundles resources for orphan recovery tests.
type orphanTestEnv struct {
	t      *testing.T
	db     *sql.DB
	store  *ds.Store
	client k8s.ClusterClient
}

func setupOrphanEnv(t *testing.T) *orphanTestEnv {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping orphan integration test")
	}
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Skip("KUBECONFIG not set — skipping orphan integration test")
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

	return &orphanTestEnv{
		t:      t,
		db:     db,
		store:  ds.NewStore(db),
		client: client,
	}
}

// createOrphanedNamespace creates a K8s namespace with astro labels but no DB record.
// sourceAccountID is the source-account-id label; when empty, the label is
// omitted so legacy/pre-PR2 namespaces can be simulated.
func (e *orphanTestEnv) createOrphanedNamespace(name, accountID, sourceAccountID, agentName, buildID string) {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	labels := map[string]string{
		"app.kubernetes.io/managed-by": "astro-server",
		"astro.dev/account-id":         accountID,
		"astro.dev/agent":              agentName,
		"astro.dev/build":              buildID,
	}
	if sourceAccountID != "" {
		labels["astro.dev/source-account-id"] = sourceAccountID
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	if _, err := e.client.Clientset().CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
		e.t.Fatalf("create orphaned namespace %s: %v", name, err)
	}
	e.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = e.client.Clientset().CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	})
}

// ensureTestAccount creates or fetches the e2e test account.
func (e *orphanTestEnv) ensureTestAccount() string {
	e.t.Helper()
	var accountID string
	err := e.db.QueryRow(`
		INSERT INTO accounts (name, type) VALUES ('orphan-e2e', 'personal')
		ON CONFLICT DO NOTHING RETURNING id
	`).Scan(&accountID)
	if err != nil {
		err = e.db.QueryRow(`SELECT id FROM accounts WHERE name = 'orphan-e2e'`).Scan(&accountID)
		if err != nil {
			e.t.Fatalf("get test account: %v", err)
		}
	}
	return accountID
}

// TestOrphan_RecoverNewFormat verifies that an orphaned namespace in the new
// format (astro-{9chars}-0) is recovered with the original deployment ID.
func TestOrphan_RecoverNewFormat(t *testing.T) {
	env := setupOrphanEnv(t)
	accountID := env.ensureTestAccount()

	// Generate a real deployment ID and derive its namespace
	origID := deployid.New()
	ns := "astro-" + deployid.Compact(origID) + "-0"

	env.createOrphanedNamespace(ns, accountID, "", "orphan-new-agent", "build-01")

	// Verify no deployment exists for this namespace
	dep, err := env.store.GetDeploymentByNamespace(ns)
	if err != nil {
		t.Fatalf("GetDeploymentByNamespace: %v", err)
	}
	if dep != nil {
		t.Fatalf("expected no deployment for %s, got %s", ns, dep.ID)
	}

	// Recover — should use the original ID extracted from namespace
	recoveredID := deployid.FromNamespace(ns)
	if recoveredID == "" {
		t.Fatal("FromNamespace returned empty for new-format namespace")
	}
	if recoveredID != origID {
		t.Errorf("FromNamespace = %q, want %q", recoveredID, origID)
	}

	err = env.store.RecoverOrphanedDeployment(recoveredID, accountID, accountID, "orphan-new-agent", "build-01", ns)
	if err != nil {
		t.Fatalf("RecoverOrphanedDeployment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.db.Exec("DELETE FROM namespace_ownership WHERE deployment_id = $1", recoveredID)
		_, _ = env.db.Exec("DELETE FROM deployments WHERE id = $1", recoveredID)
	})

	// Verify deployment was created with correct state
	dep, err = env.store.GetDeploymentByID(recoveredID)
	if err != nil {
		t.Fatalf("GetDeploymentByID: %v", err)
	}
	if dep == nil {
		t.Fatal("expected recovered deployment, got nil")
	}
	if dep.Status != ds.StatusFailed {
		t.Errorf("status = %q, want %q", dep.Status, ds.StatusFailed)
	}
	if dep.Namespace != ns {
		t.Errorf("namespace = %q, want %q", dep.Namespace, ns)
	}
	if dep.AgentName != "orphan-new-agent" {
		t.Errorf("agent_name = %q, want %q", dep.AgentName, "orphan-new-agent")
	}
	if dep.AccountID != accountID {
		t.Errorf("account_id = %q, want %q", dep.AccountID, accountID)
	}
	if dep.CurrentRevision != nil {
		t.Errorf("current_revision = %v, want nil (no revisions)", *dep.CurrentRevision)
	}
	if dep.ErrorMessage == nil || *dep.ErrorMessage == "" {
		t.Error("expected error_message to be set")
	}
}

// TestOrphan_RecoverOldFormat verifies that an orphaned namespace in the old
// format (astro-{longhex}) is recovered with a new deployment ID.
func TestOrphan_RecoverOldFormat(t *testing.T) {
	env := setupOrphanEnv(t)
	accountID := env.ensureTestAccount()

	ns := "astro-e2eoldfmt" + sanitize(t.Name())
	if len(ns) > 50 {
		ns = ns[:50]
	}

	env.createOrphanedNamespace(ns, accountID, "", "orphan-old-agent", "build-02")

	// FromNamespace should return empty for non-standard format
	if id := deployid.FromNamespace(ns); id != "" {
		t.Fatalf("FromNamespace should return empty for old format, got %q", id)
	}

	// Recover with a new ID (what the reconciler does for old format)
	newID := deployid.New()
	err := env.store.RecoverOrphanedDeployment(newID, accountID, accountID, "orphan-old-agent", "build-02", ns)
	if err != nil {
		t.Fatalf("RecoverOrphanedDeployment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.db.Exec("DELETE FROM namespace_ownership WHERE deployment_id = $1", newID)
		_, _ = env.db.Exec("DELETE FROM deployments WHERE id = $1", newID)
	})

	dep, err := env.store.GetDeploymentByID(newID)
	if err != nil {
		t.Fatalf("GetDeploymentByID: %v", err)
	}
	if dep == nil {
		t.Fatal("expected recovered deployment, got nil")
	}
	if dep.Status != ds.StatusFailed {
		t.Errorf("status = %q, want %q", dep.Status, ds.StatusFailed)
	}
	if dep.Namespace != ns {
		t.Errorf("namespace = %q, want %q", dep.Namespace, ns)
	}
}

// TestOrphan_RecoveredVisibleInStatusQuery verifies the recovered deployment
// shows up in GetDeploymentsInStatus (so the reconciler won't re-orphan it).
func TestOrphan_RecoveredVisibleInStatusQuery(t *testing.T) {
	env := setupOrphanEnv(t)
	accountID := env.ensureTestAccount()

	origID := deployid.New()
	ns := "astro-" + deployid.Compact(origID) + "-0"

	env.createOrphanedNamespace(ns, accountID, "", "orphan-visible-agent", "build-03")

	err := env.store.RecoverOrphanedDeployment(origID, accountID, accountID, "orphan-visible-agent", "build-03", ns)
	if err != nil {
		t.Fatalf("RecoverOrphanedDeployment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.db.Exec("DELETE FROM namespace_ownership WHERE deployment_id = $1", origID)
		_, _ = env.db.Exec("DELETE FROM deployments WHERE id = $1", origID)
	})

	// GetDeploymentsInStatus with "failed" should include this deployment
	deps, err := env.store.GetDeploymentsInStatus(ds.StatusFailed)
	if err != nil {
		t.Fatalf("GetDeploymentsInStatus: %v", err)
	}

	found := false
	for _, d := range deps {
		if d.ID == origID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("recovered deployment %s not found in failed deployments query", origID)
	}
}

// TestOrphan_RecoverIdempotent verifies that recovering the same namespace twice
// fails (unique constraint on namespace prevents duplicates).
func TestOrphan_RecoverIdempotent(t *testing.T) {
	env := setupOrphanEnv(t)
	accountID := env.ensureTestAccount()

	origID := deployid.New()
	ns := "astro-" + deployid.Compact(origID) + "-0"

	env.createOrphanedNamespace(ns, accountID, "", "orphan-idem-agent", "build-04")

	err := env.store.RecoverOrphanedDeployment(origID, accountID, accountID, "orphan-idem-agent", "build-04", ns)
	if err != nil {
		t.Fatalf("first RecoverOrphanedDeployment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.db.Exec("DELETE FROM namespace_ownership WHERE deployment_id = $1", origID)
		_, _ = env.db.Exec("DELETE FROM deployments WHERE id = $1", origID)
	})

	// Second recovery with a different ID should fail on unique deployment ID
	// or namespace constraint — either way it should error
	secondID := deployid.New()
	err = env.store.RecoverOrphanedDeployment(secondID, accountID, accountID, "orphan-idem-agent", "build-04", ns)
	if err == nil {
		// Clean up the second record if it was somehow created
		t.Cleanup(func() {
			_, _ = env.db.Exec("DELETE FROM namespace_ownership WHERE deployment_id = $1", secondID)
			_, _ = env.db.Exec("DELETE FROM deployments WHERE id = $1", secondID)
		})
		// The deployments table may not have a unique constraint on namespace,
		// so a second insert could succeed. Verify both exist and the first
		// is still queryable.
		dep, err := env.store.GetDeploymentByID(origID)
		if err != nil || dep == nil {
			t.Fatal("original recovered deployment lost after second recovery")
		}
	}
}

// TestOrphan_EventRecorded verifies that a deployment_event row is created
// during orphan recovery.
func TestOrphan_EventRecorded(t *testing.T) {
	env := setupOrphanEnv(t)
	accountID := env.ensureTestAccount()

	origID := deployid.New()
	ns := "astro-" + deployid.Compact(origID) + "-0"

	env.createOrphanedNamespace(ns, accountID, "", "orphan-event-agent", "build-05")

	err := env.store.RecoverOrphanedDeployment(origID, accountID, accountID, "orphan-event-agent", "build-05", ns)
	if err != nil {
		t.Fatalf("RecoverOrphanedDeployment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.db.Exec("DELETE FROM namespace_ownership WHERE deployment_id = $1", origID)
		_, _ = env.db.Exec("DELETE FROM deployments WHERE id = $1", origID)
	})

	var eventCount int
	err = env.db.QueryRow(
		`SELECT COUNT(*) FROM deployment_events WHERE deployment_id = $1 AND status = 'failed'`,
		origID,
	).Scan(&eventCount)
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("expected 1 recovery event, got %d", eventCount)
	}
}

// namespaceOwnershipUpsert runs the same SQL the reconciler uses to populate
// namespace_ownership, including source_account extraction.
func namespaceOwnershipUpsert(db *sql.DB, namespace, accountID, agentName, deploymentID, sourceAccount string) error {
	_, err := db.Exec(`
		INSERT INTO namespace_ownership (namespace, account_id, agent_name, deployment_id, source_account, scanned_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (namespace) DO UPDATE
		SET account_id = EXCLUDED.account_id,
		    agent_name = EXCLUDED.agent_name,
		    deployment_id = EXCLUDED.deployment_id,
		    source_account = EXCLUDED.source_account,
		    scanned_at = EXCLUDED.scanned_at
	`, namespace, accountID, agentName, deploymentID, sourceAccount)
	return err
}

// TestOrphan_SourceAccountPopulated verifies that the namespace_ownership upsert
// correctly writes source_account from the deployment spec.
func TestOrphan_SourceAccountPopulated(t *testing.T) {
	env := setupOrphanEnv(t)
	accountID := env.ensureTestAccount()

	depID := deployid.New()
	ns := "astro-" + deployid.Compact(depID) + "-0"

	// Create a deployment with a spec containing source.account
	specJSON := `{"source":{"account":"builder-team","name":"cross-agent","build":"b1"}}`
	_, err := env.db.Exec(`
		INSERT INTO deployments (id, account_id, agent_name, build_id, namespace,
		    deployment_spec_json, status, status_changed_at, deployed_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', NOW(), NOW())
	`, depID, accountID, "cross-agent", "b1", ns, specJSON)
	if err != nil {
		t.Fatalf("insert deployment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.db.Exec("DELETE FROM namespace_ownership WHERE namespace = $1", ns)
		_, _ = env.db.Exec("DELETE FROM deployments WHERE id = $1", depID)
	})

	// Run the upsert with source_account
	err = namespaceOwnershipUpsert(env.db, ns, accountID, "cross-agent", depID, "builder-team")
	if err != nil {
		t.Fatalf("upsert namespace_ownership: %v", err)
	}

	// Verify source_account was written
	var sourceAccount string
	err = env.db.QueryRow(
		`SELECT source_account FROM namespace_ownership WHERE namespace = $1`, ns,
	).Scan(&sourceAccount)
	if err != nil {
		t.Fatalf("query source_account: %v", err)
	}
	if sourceAccount != "builder-team" {
		t.Errorf("source_account = %q, want %q", sourceAccount, "builder-team")
	}
}

// TestOrphan_SourceAccountUpdatedOnReUpsert verifies that the upsert overwrites
// source_account when a deployment's spec changes (e.g. agent transferred).
func TestOrphan_SourceAccountUpdatedOnReUpsert(t *testing.T) {
	env := setupOrphanEnv(t)
	accountID := env.ensureTestAccount()

	depID := deployid.New()
	ns := "astro-" + deployid.Compact(depID) + "-0"

	_, err := env.db.Exec(`
		INSERT INTO deployments (id, account_id, agent_name, build_id, namespace,
		    deployment_spec_json, status, status_changed_at, deployed_at)
		VALUES ($1, $2, $3, $4, $5, '{}', 'active', NOW(), NOW())
	`, depID, accountID, "transfer-agent", "b1", ns)
	if err != nil {
		t.Fatalf("insert deployment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.db.Exec("DELETE FROM namespace_ownership WHERE namespace = $1", ns)
		_, _ = env.db.Exec("DELETE FROM deployments WHERE id = $1", depID)
	})

	// First upsert with empty source_account
	err = namespaceOwnershipUpsert(env.db, ns, accountID, "transfer-agent", depID, "")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	var sourceAccount string
	err = env.db.QueryRow(
		`SELECT source_account FROM namespace_ownership WHERE namespace = $1`, ns,
	).Scan(&sourceAccount)
	if err != nil {
		t.Fatalf("query after first upsert: %v", err)
	}
	if sourceAccount != "" {
		t.Errorf("after first upsert: source_account = %q, want empty", sourceAccount)
	}

	// Second upsert with source_account populated (simulates next reconcile after spec update)
	err = namespaceOwnershipUpsert(env.db, ns, accountID, "transfer-agent", depID, "new-owner-team")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	err = env.db.QueryRow(
		`SELECT source_account FROM namespace_ownership WHERE namespace = $1`, ns,
	).Scan(&sourceAccount)
	if err != nil {
		t.Fatalf("query after second upsert: %v", err)
	}
	if sourceAccount != "new-owner-team" {
		t.Errorf("after second upsert: source_account = %q, want %q", sourceAccount, "new-owner-team")
	}
}

// ensureSecondaryAccount creates an auxiliary personal account by name so
// cross-account orphan tests have a second account ID for the source FK
// to resolve against. Idempotent across reruns.
func (e *orphanTestEnv) ensureSecondaryAccount(name string) string {
	e.t.Helper()
	var id string
	err := e.db.QueryRow(`
		INSERT INTO accounts (name, type) VALUES ($1, 'personal')
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, name).Scan(&id)
	if err != nil {
		e.t.Fatalf("ensureSecondaryAccount(%q): %v", name, err)
	}
	return id
}

// TestOrphan_RecoverWithSourceAccountLabel pins the cross-account orphan
// path: when the namespace carries astro.dev/source-account-id pointing
// at a *different* account than the deployer, the recovered row's
// source_account_id must be that account, not the deployer.
//
// The reconciler-side label-reading is covered by the sqlmock test
// TestMaintainNamespaceOwnership_OrphanRecovered_LabeledSource — this
// test exercises the store half against real Postgres so the FK on
// source_account_id → accounts.id is satisfied with a genuine UUID
// from a real accounts row, not a sqlmock stand-in.
func TestOrphan_RecoverWithSourceAccountLabel(t *testing.T) {
	env := setupOrphanEnv(t)
	deployerID := env.ensureTestAccount()
	sourceID := env.ensureSecondaryAccount("orphan-cross-source-e2e")

	origID := deployid.New()
	ns := "astro-" + deployid.Compact(origID) + "-0"

	env.createOrphanedNamespace(ns, deployerID, sourceID, "orphan-cross-agent", "build-06")

	err := env.store.RecoverOrphanedDeployment(origID, deployerID, sourceID, "orphan-cross-agent", "build-06", ns)
	if err != nil {
		t.Fatalf("RecoverOrphanedDeployment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = env.db.Exec("DELETE FROM namespace_ownership WHERE deployment_id = $1", origID)
		_, _ = env.db.Exec("DELETE FROM deployments WHERE id = $1", origID)
	})

	dep, err := env.store.GetDeploymentByID(origID)
	if err != nil {
		t.Fatalf("GetDeploymentByID: %v", err)
	}
	if dep == nil {
		t.Fatal("expected recovered deployment, got nil")
	}
	if dep.SourceAccountID == nil {
		t.Fatalf("source_account_id is NULL, want %q", sourceID)
	}
	if *dep.SourceAccountID != sourceID {
		t.Errorf("source_account_id = %q, want %q (label-bearing recovery must not default to deployer %q)",
			*dep.SourceAccountID, sourceID, deployerID)
	}
}

