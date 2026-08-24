//go:build integration

// Integration coverage for deploymentstore.RebindStaleSourceAccountIDs.
// This is the data-sweep counterpart to PR1's transactional fix in
// agentindex.Transfer: existing rows that were left in the inconsistent
// state by pre-fix transfers (source_account_id pointing at the old
// publisher account) are detected and repointed at the unique current
// owner from agent_versions. The cases below exercise every decision
// branch the WHERE clause encodes:
//
//  1. Stale row with a unique candidate                          → rebind.
//  2. Stale row with multiple candidates (collision)             → leave alone.
//  3. Stale row whose (name, build_id) is absent from agent_versions
//     (build deleted)                                            → leave alone.
//  4. Already-correct row                                        → leave alone.
//  5. NULL source_account_id (publisher-deleted, FK SET NULL)    → leave alone.
//  6. Re-running over the same fixture                           → no-op.
package e2e

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	_ "github.com/lib/pq"
)

// rebindFixture creates two distinct accounts and a deploymentstore.Store
// against the shared test DB. Distinct from setupBackfillFixture (which is
// scoped to NULL-fill scenarios) because rebind tests need to register
// agents in a *different* account than the deployment's stale source.
type rebindFixture struct {
	db       *sql.DB
	store    *ds.Store
	index    *agentindex.Index
	sourceID string // pre-transfer publisher (where deployments still point)
	targetID string // post-transfer publisher (where agent_versions actually lives)
	thirdID  string // unrelated third account, used to construct ambiguous cases
}

func setupRebindFixture(t *testing.T) *rebindFixture {
	t.Helper()
	db := testDB(t)
	store := ds.NewStore(db)
	index := agentindex.NewIndexWithDB(db)

	mk := func(prefix string) (string, string) {
		t.Helper()
		name := prefix + "-" + strings.ToLower(deployid.New())
		var id string
		if err := db.QueryRow(
			`WITH acct AS (INSERT INTO accounts (name, type, owner_user_id) VALUES ($1, 'organization', 'test-owner') RETURNING id), member AS (INSERT INTO account_members (account_id, user_id) SELECT id, 'test-owner' FROM acct ON CONFLICT DO NOTHING) SELECT id FROM acct`,
			name,
		).Scan(&id); err != nil {
			t.Fatalf("create %s account: %v", prefix, err)
		}
		return id, name
	}

	sourceID, _ := mk("rebind-src")
	targetID, _ := mk("rebind-tgt")
	thirdID, _ := mk("rebind-third")

	t.Cleanup(func() {
		// FK ON DELETE CASCADE on agents/agent_versions handles itself
		// when accounts go away; deployments are SET NULL and then
		// deleted by the fixture-scoped row cleanup that callers add.
		_, _ = db.Exec("DELETE FROM accounts WHERE id IN ($1, $2, $3)", sourceID, targetID, thirdID)
	})

	return &rebindFixture{
		db: db, store: store, index: index,
		sourceID: sourceID, targetID: targetID, thirdID: thirdID,
	}
}

// insertDeploymentRaw writes a deployment row directly so we can pin an
// arbitrary source_account_id that does not match agent_versions — the
// production code path validates lineage before writing, so a "stale"
// row can only be produced by either (a) the historical Transfer bug or
// (b) this helper. The cleanup is name-scoped so the row is removed on
// test exit.
func insertDeploymentRaw(t *testing.T, db *sql.DB, ownerID string, sourceID *string, agentName, buildID string) string {
	t.Helper()
	id := deployid.New()
	_, err := db.Exec(`
		INSERT INTO deployments (id, account_id, source_account_id, agent_name, build_id,
			namespace, display_name, deployment_spec_json, status, status_changed_at, deployed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, '{"spec":"deployment/v1"}', 'active', NOW(), NOW())
	`, id, ownerID, sourceID, agentName, buildID, "ns-"+id, agentName)
	if err != nil {
		t.Fatalf("insert raw deployment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM deployments WHERE id = $1", id)
	})
	return id
}

// registerAgentByID is the ID-keyed variant of source_account_attribution_test.go's
// registerAgent helper (which takes an *account.Account); rebind tests
// only have account IDs in hand because they construct accounts directly
// via SQL rather than through the account.Store API.
func registerAgentByID(t *testing.T, idx *agentindex.Index, db *sql.DB, accountID, name, buildID string) {
	t.Helper()
	if err := idx.Register(accountID, name, buildID, "test-registry", "test-ns",
		map[string]any{"spec": "deployment/v1"}, "", "", ""); err != nil {
		t.Fatalf("Register(%s/%s/%s): %v", accountID, name, buildID, err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM agent_versions WHERE account_id = $1 AND name = $2", accountID, name)
		_, _ = db.Exec("DELETE FROM agents WHERE account_id = $1 AND name = $2", accountID, name)
	})
}

func sourceAcctOf(t *testing.T, db *sql.DB, depID string) *string {
	t.Helper()
	var got sql.NullString
	if err := db.QueryRow(`SELECT source_account_id FROM deployments WHERE id = $1`, depID).Scan(&got); err != nil {
		t.Fatalf("read source_account_id for %s: %v", depID, err)
	}
	if !got.Valid {
		return nil
	}
	return &got.String
}

// (1) Happy path: a transferred agent's deployment is repointed at the
// account that currently owns the (name, build_id) tuple in
// agent_versions. This is the historical bug the sweep exists to fix.
func TestRebindStaleSourceAccountIDs_RebindsTransferredAgent(t *testing.T) {
	fx := setupRebindFixture(t)
	agentName := "rebind-" + strings.ToLower(deployid.New())[:8]

	registerAgentByID(t, fx.index, fx.db, fx.targetID, agentName, "build-1")
	depID := insertDeploymentRaw(t, fx.db, fx.targetID, &fx.sourceID, agentName, "build-1")

	res, err := fx.store.RebindStaleSourceAccountIDs(context.Background())
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if res.Rebound != 1 {
		t.Errorf("Rebound = %d, want 1", res.Rebound)
	}
	if got := sourceAcctOf(t, fx.db, depID); got == nil || *got != fx.targetID {
		t.Errorf("source_account_id = %v, want %q (target after rebind)", got, fx.targetID)
	}
}

// (2) Ambiguous: when the same (name, build_id) exists in two different
// accounts (e.g. an unrelated agent that happens to share the build
// hash) the sweep must NOT pick one — it leaves the row alone for a
// human to triage. This is the safety-bound the c.n = 1 predicate buys.
func TestRebindStaleSourceAccountIDs_AmbiguousLeftAlone(t *testing.T) {
	fx := setupRebindFixture(t)
	agentName := "rebind-amb-" + strings.ToLower(deployid.New())[:8]

	registerAgentByID(t, fx.index, fx.db, fx.targetID, agentName, "build-1")
	registerAgentByID(t, fx.index, fx.db, fx.thirdID, agentName, "build-1")

	depID := insertDeploymentRaw(t, fx.db, fx.targetID, &fx.sourceID, agentName, "build-1")

	res, err := fx.store.RebindStaleSourceAccountIDs(context.Background())
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if res.Rebound != 0 {
		t.Errorf("Rebound = %d, want 0 for ambiguous (name, build_id)", res.Rebound)
	}
	if got := sourceAcctOf(t, fx.db, depID); got == nil || *got != fx.sourceID {
		t.Errorf("source_account_id = %v, want %q (unchanged)", got, fx.sourceID)
	}
}

// (3) The build was actually deleted (not transferred): no candidate in
// agent_versions for (name, build_id). The sweep must leave the row
// alone — letting PR4's FK or PR5's resolveSourceAccountName decide
// what to do with rows whose lineage genuinely no longer exists.
func TestRebindStaleSourceAccountIDs_UnknownBuildLeftAlone(t *testing.T) {
	fx := setupRebindFixture(t)
	agentName := "rebind-gone-" + strings.ToLower(deployid.New())[:8]

	depID := insertDeploymentRaw(t, fx.db, fx.targetID, &fx.sourceID, agentName, "build-deleted")

	res, err := fx.store.RebindStaleSourceAccountIDs(context.Background())
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if res.Rebound != 0 {
		t.Errorf("Rebound = %d, want 0 when build is absent from agent_versions", res.Rebound)
	}
	if got := sourceAcctOf(t, fx.db, depID); got == nil || *got != fx.sourceID {
		t.Errorf("source_account_id = %v, want %q (unchanged)", got, fx.sourceID)
	}
}

// (4) Already-correct row: source_account_id already matches the unique
// owner. The `<>` predicate must filter it out so we don't waste an
// UPDATE and (more importantly) so the Rebound counter stays honest.
func TestRebindStaleSourceAccountIDs_ConsistentRowLeftAlone(t *testing.T) {
	fx := setupRebindFixture(t)
	agentName := "rebind-ok-" + strings.ToLower(deployid.New())[:8]

	registerAgentByID(t, fx.index, fx.db, fx.targetID, agentName, "build-1")
	depID := insertDeploymentRaw(t, fx.db, fx.targetID, &fx.targetID, agentName, "build-1")

	res, err := fx.store.RebindStaleSourceAccountIDs(context.Background())
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if res.Rebound != 0 {
		t.Errorf("Rebound = %d, want 0 for already-correct row", res.Rebound)
	}
	if got := sourceAcctOf(t, fx.db, depID); got == nil || *got != fx.targetID {
		t.Errorf("source_account_id = %v, want %q (unchanged)", got, fx.targetID)
	}
}

// (5) NULL source_account_id (the publisher account was deleted; FK SET
// NULL has fired). These rows belong to BackfillSourceAccountIDs's
// NULL-fill pass — rebind must not touch them, otherwise the two passes
// fight and the row's history depends on which ran last.
func TestRebindStaleSourceAccountIDs_NullSourceLeftAlone(t *testing.T) {
	fx := setupRebindFixture(t)
	agentName := "rebind-null-" + strings.ToLower(deployid.New())[:8]

	registerAgentByID(t, fx.index, fx.db, fx.targetID, agentName, "build-1")
	depID := insertDeploymentRaw(t, fx.db, fx.targetID, nil, agentName, "build-1")

	res, err := fx.store.RebindStaleSourceAccountIDs(context.Background())
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if res.Rebound != 0 {
		t.Errorf("Rebound = %d, want 0 for NULL source_account_id", res.Rebound)
	}
	if got := sourceAcctOf(t, fx.db, depID); got != nil {
		t.Errorf("source_account_id = %q, want NULL (unchanged)", *got)
	}
}

// (6) Idempotency: every replica that boots will run this. After the
// first pass repoints a row, the second pass MUST find nothing to do —
// otherwise the periodic re-runs amplify into a loop or burn unbounded
// UPDATE bandwidth on a healthy cluster.
func TestRebindStaleSourceAccountIDs_Idempotent(t *testing.T) {
	fx := setupRebindFixture(t)
	agentName := "rebind-idem-" + strings.ToLower(deployid.New())[:8]

	registerAgentByID(t, fx.index, fx.db, fx.targetID, agentName, "build-1")
	depID := insertDeploymentRaw(t, fx.db, fx.targetID, &fx.sourceID, agentName, "build-1")

	first, err := fx.store.RebindStaleSourceAccountIDs(context.Background())
	if err != nil {
		t.Fatalf("first rebind: %v", err)
	}
	if first.Rebound != 1 {
		t.Fatalf("first Rebound = %d, want 1", first.Rebound)
	}
	if got := sourceAcctOf(t, fx.db, depID); got == nil || *got != fx.targetID {
		t.Fatalf("after first run: source_account_id = %v, want %q", got, fx.targetID)
	}

	second, err := fx.store.RebindStaleSourceAccountIDs(context.Background())
	if err != nil {
		t.Fatalf("second rebind: %v", err)
	}
	if second.Rebound != 0 {
		t.Errorf("second Rebound = %d, want 0 (no-op on already-repaired row)", second.Rebound)
	}
}

// Bookkeeping: rebind multiple stale rows in a single pass. Pins that
// the sweep is set-based, not row-by-row — a future "fix" that loops in
// Go would still pass single-row tests but blow up here on the count.
func TestRebindStaleSourceAccountIDs_RepairsMultipleRowsInOnePass(t *testing.T) {
	fx := setupRebindFixture(t)
	suffix := strings.ToLower(deployid.New())[:8]
	a1 := "rebind-multi-a-" + suffix
	a2 := "rebind-multi-b-" + suffix

	registerAgentByID(t, fx.index, fx.db, fx.targetID, a1, "build-1")
	registerAgentByID(t, fx.index, fx.db, fx.targetID, a2, "build-1")

	d1 := insertDeploymentRaw(t, fx.db, fx.targetID, &fx.sourceID, a1, "build-1")
	d2 := insertDeploymentRaw(t, fx.db, fx.thirdID, &fx.sourceID, a2, "build-1")

	res, err := fx.store.RebindStaleSourceAccountIDs(context.Background())
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if res.Rebound != 2 {
		t.Errorf("Rebound = %d, want 2", res.Rebound)
	}
	if got := sourceAcctOf(t, fx.db, d1); got == nil || *got != fx.targetID {
		t.Errorf("d1 source_account_id = %v, want %q", got, fx.targetID)
	}
	if got := sourceAcctOf(t, fx.db, d2); got == nil || *got != fx.targetID {
		t.Errorf("d2 source_account_id = %v, want %q", got, fx.targetID)
	}
}
