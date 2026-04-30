//go:build integration

// Transfer integration tests — verifies that agentindex.Transfer rewrites
// agents, agent_versions, AND deployments.source_account_id together against
// a real Postgres so the cross-table effect is exercised end-to-end (the
// sqlmock unit tests next to Transfer can only assert that the right SQL is
// emitted, not that PostgreSQL applies it the way we expect).
package e2e

import (
	"database/sql"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	_ "github.com/lib/pq"
)

// TestTransfer_RepointsDeploymentsSourceAccountID is the regression test for
// the cross-account upgrade-signal bug: before the fix, transferring an
// agent left deployments.source_account_id pointing at the old account, so
// resolveSourceAccountName fell through to the spec-JSON fallback under
// the old (now stale or reclaimed) account name.
//
// We seed three deployments against a freshly registered agent in
// `source-acct`:
//
//   - matchA: cross-account deploy (account_id = deployer-acct, source = source-acct)
//   - matchB: same-account deploy   (account_id = source-acct,   source = source-acct)
//   - other:  unrelated deployment of a *different* agent on the same source
//
// After Transfer(source -> target, agentName), matchA and matchB must have
// source_account_id = target; `other` must be untouched. This pins the
// WHERE clause's selectivity so a future "fix" that drops `agent_name`
// from the predicate would fail loudly here instead of silently breaking
// unrelated lineage in production.
func TestTransfer_RepointsDeploymentsSourceAccountID(t *testing.T) {
	db := testDB(t)
	index := agentindex.NewIndexWithDB(db)
	store := ds.NewStore(db)

	sourceID := ensureNamedAccount(t, db, "transfer-src-e2e")
	targetID := ensureNamedAccount(t, db, "transfer-tgt-e2e")
	deployerID := ensureNamedAccount(t, db, "transfer-deployer-e2e")

	suffix := deployid.New()[:6]
	agentName := "xfer-" + suffix
	otherName := "xfer-other-" + suffix

	t.Cleanup(func() {
		// Order matters: deployments first (FK to agents would block via
		// agent_versions if we added the FK from PR4 later), then versions
		// + agents themselves. Bound the cleanup to the names this test
		// owns so re-runs against a shared DB stay isolated.
		_, _ = db.Exec(`DELETE FROM deployments WHERE agent_name IN ($1, $2)`, agentName, otherName)
		_, _ = db.Exec(`DELETE FROM agent_versions WHERE name IN ($1, $2)`, agentName, otherName)
		_, _ = db.Exec(`DELETE FROM agents WHERE name IN ($1, $2)`, agentName, otherName)
	})

	if err := index.Register(sourceID, agentName, "build-1", "test-registry", "test-ns",
		map[string]any{"spec": "deployment/v1"}, "", "", ""); err != nil {
		t.Fatalf("Register %s: %v", agentName, err)
	}
	if err := index.Register(sourceID, otherName, "build-1", "test-registry", "test-ns",
		map[string]any{"spec": "deployment/v1"}, "", "", ""); err != nil {
		t.Fatalf("Register %s: %v", otherName, err)
	}

	matchA := seedTransferDep(t, store, sourceID, deployerID, agentName)
	matchB := seedTransferDep(t, store, sourceID, sourceID, agentName)
	untouched := seedTransferDep(t, store, sourceID, deployerID, otherName)

	requireSourceAcct(t, db, matchA, sourceID)
	requireSourceAcct(t, db, matchB, sourceID)
	requireSourceAcct(t, db, untouched, sourceID)

	if err := index.Transfer(sourceID, targetID, agentName); err != nil {
		t.Fatalf("Transfer: %v", err)
	}

	requireSourceAcct(t, db, matchA, targetID)
	requireSourceAcct(t, db, matchB, targetID)
	requireSourceAcct(t, db, untouched, sourceID)
}

// TestTransfer_NoDeploymentsCommitsCleanly guards against the "no rows
// affected = error" pattern that the agents UPDATE uses. The deployments
// statement must accept zero matches (the common case for an agent with no
// deployed instances at the time of transfer) and still commit.
func TestTransfer_NoDeploymentsCommitsCleanly(t *testing.T) {
	db := testDB(t)
	index := agentindex.NewIndexWithDB(db)

	sourceID := ensureNamedAccount(t, db, "transfer-src-empty-e2e")
	targetID := ensureNamedAccount(t, db, "transfer-tgt-empty-e2e")

	suffix := deployid.New()[:6]
	agentName := "xfer-empty-" + suffix

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM agent_versions WHERE name = $1`, agentName)
		_, _ = db.Exec(`DELETE FROM agents WHERE name = $1`, agentName)
	})

	if err := index.Register(sourceID, agentName, "build-1", "test-registry", "test-ns",
		map[string]any{"spec": "deployment/v1"}, "", "", ""); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := index.Transfer(sourceID, targetID, agentName); err != nil {
		t.Fatalf("Transfer with no deployments must succeed: %v", err)
	}

	var movedAcct string
	if err := db.QueryRow(`SELECT account_id FROM agents WHERE name = $1`, agentName).Scan(&movedAcct); err != nil {
		t.Fatalf("query agents post-transfer: %v", err)
	}
	if movedAcct != targetID {
		t.Errorf("agent.account_id = %q, want %q (transfer should commit even with 0 deployments)", movedAcct, targetID)
	}
}

// --- helpers ---

// ensureNamedAccount idempotently creates a personal account by name and
// returns its ID. Distinct from ensureTestAccount in normalized_test.go,
// which is hardcoded to the single name "test-e2e".
func ensureNamedAccount(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
		INSERT INTO accounts (name, type) VALUES ($1, 'personal')
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, name).Scan(&id)
	if err != nil {
		t.Fatalf("ensureNamedAccount(%q): %v", name, err)
	}
	return id
}

// seedTransferDep writes a pending deployment row with an explicit source
// account so we can verify Transfer's WHERE clause against it. Returns the
// deployment ID.
func seedTransferDep(t *testing.T, store *ds.Store, sourceAcctID, ownerAcctID, agentName string) string {
	t.Helper()
	d, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID:              deployid.New(),
		AccountID:       ownerAcctID,
		SourceAccountID: sourceAcctID,
		AgentName:       agentName,
		BuildID:         "build-1",
		Namespace:       "ns-" + agentName + "-" + deployid.New()[:6],
		SpecJSON:        `{"spec":"deployment/v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending(%s): %v", agentName, err)
	}
	return d.ID
}

func requireSourceAcct(t *testing.T, db *sql.DB, depID, want string) {
	t.Helper()
	var got sql.NullString
	if err := db.QueryRow(`SELECT source_account_id FROM deployments WHERE id = $1`, depID).Scan(&got); err != nil {
		t.Fatalf("query source_account_id for deployment %s: %v", depID, err)
	}
	if !got.Valid {
		t.Errorf("deployment %s: source_account_id is NULL, want %q", depID, want)
		return
	}
	if got.String != want {
		t.Errorf("deployment %s: source_account_id = %q, want %q", depID, got.String, want)
	}
}
