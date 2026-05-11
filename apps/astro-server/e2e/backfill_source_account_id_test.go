//go:build integration

package e2e

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	_ "github.com/lib/pq"
)

/*
Integration coverage for deploymentstore.BackfillSourceAccountIDs. This
runs at server startup (async) and its job is to populate
deployments.source_account_id for rows that predate the column. The tests
cover every decision branch it has:

 1. source.account names a real account          → column filled from spec (FromSpec++).
 2. source.account names an unknown account      → fall back to row's own account_id (SpecMisses++, FromSelf++).
 3. spec omits source                              → fall back to own account_id (FromSelf++).
 4. spec is malformed JSON                         → fall back to own account_id (FromSelf++).
 5. source_account_id already set                  → row untouched; re-run is a no-op.
 6. spec names a real publisher without this build on source → fall back to the
    owning account when that account’s tuple validates (FromSelf++); increment
    SkippedInvalidLineage for the rejected spec publisher.


The idempotency case is the important one: the backfill must never
clobber a value that a newer write has already stored, because new deploy
writes populate the column directly. The WHERE source_account_id IS NULL
predicate is the only thing protecting those writes from a later backfill.
*/

type backfillFixture struct {
	db         *sql.DB
	store      *ds.Store
	index      *agentindex.Index
	sourceID   string
	sourceName string
	targetID   string
	targetName string
}

func setupBackfillFixture(t *testing.T) *backfillFixture {
	t.Helper()
	db := testDB(t)
	index := agentindex.NewIndexWithDB(db)
	store := ds.NewStore(db).WithLineageValidator(index)

	sourceName := "bf-src-" + strings.ToLower(deployid.New())
	var sourceID string
	if err := db.QueryRow(
		`INSERT INTO accounts (name, type) VALUES ($1, 'organization') RETURNING id`,
		sourceName,
	).Scan(&sourceID); err != nil {
		t.Fatalf("create source account: %v", err)
	}

	targetName := "bf-tgt-" + strings.ToLower(deployid.New())
	var targetID string
	if err := db.QueryRow(
		`INSERT INTO accounts (name, type) VALUES ($1, 'organization') RETURNING id`,
		targetName,
	).Scan(&targetID); err != nil {
		t.Fatalf("create target account: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM accounts WHERE id IN ($1, $2)", sourceID, targetID)
	})

	srcAcct := &account.Account{ID: sourceID, Name: sourceName}
	tgtAcct := &account.Account{ID: targetID, Name: targetName}
	registerBackfillAgent(t, index, srcAcct, "bf-agent", "bf-build-1")
	registerBackfillAgent(t, index, tgtAcct, "bf-agent", "bf-build-1")

	return &backfillFixture{
		db: db, store: store, index: index,
		sourceID: sourceID, sourceName: sourceName,
		targetID: targetID, targetName: targetName,
	}
}

func registerBackfillAgent(t *testing.T, index *agentindex.Index, acct *account.Account, name, buildID string) {
	t.Helper()
	specMap := map[string]any{
		"name": name,
		"agent": map[string]any{
			"image": "registry.io/" + acct.Name + "/" + name + ":" + buildID,
		},
	}
	if err := index.Register(acct.ID, name, buildID, "registry.io", acct.Name, specMap, "", "", ""); err != nil {
		t.Fatalf("register agent %s/%s@%s: %v", acct.Name, name, buildID, err)
	}
}

/*
insertLegacyDeployment writes a deployment row directly, bypassing
SaveDeploymentPending so source_account_id can be controlled exactly.
preset=nil simulates a row written before the column existed; a non-nil
preset simulates a row written after the migration by newer code.
*/
func insertLegacyDeployment(t *testing.T, db *sql.DB, targetID, specJSON string, preset *string) string {
	t.Helper()
	id := deployid.New()
	_, err := db.Exec(`
		INSERT INTO deployments (id, account_id, source_account_id, agent_name, build_id,
			namespace, display_name, deployment_spec_json, status, status_changed_at, deployed_at)
		VALUES ($1, $2, $3, 'bf-agent', 'bf-build-1', $4, 'BF', $5, 'active', NOW(), NOW())
	`, id, targetID, preset, "ns-"+id, specJSON)
	if err != nil {
		t.Fatalf("insert deployment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM deployments WHERE id = $1", id)
	})
	return id
}

// insertLegacyDeploymentWithBuild inserts a legacy row with arbitrary agent/build columns.
func insertLegacyDeploymentWithBuild(t *testing.T, db *sql.DB, targetID, agentName, buildID, specJSON string, preset *string) string {
	t.Helper()
	id := deployid.New()
	_, err := db.Exec(`
		INSERT INTO deployments (id, account_id, source_account_id, agent_name, build_id,
			namespace, display_name, deployment_spec_json, status, status_changed_at, deployed_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'BF', $7, 'active', NOW(), NOW())
	`, id, targetID, preset, agentName, buildID, "ns-"+id, specJSON)
	if err != nil {
		t.Fatalf("insert deployment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM deployments WHERE id = $1", id)
	})
	return id
}

func readSourceAccountID(t *testing.T, db *sql.DB, id string) *string {
	t.Helper()
	var got sql.NullString
	if err := db.QueryRow(`SELECT source_account_id FROM deployments WHERE id = $1`, id).Scan(&got); err != nil {
		t.Fatalf("read source_account_id for %s: %v", id, err)
	}
	if !got.Valid {
		return nil
	}
	return &got.String
}

func TestBackfillSourceAccountIDs_ResolvesFromSpec(t *testing.T) {
	fx := setupBackfillFixture(t)
	specJSON := `{"source":{"account":"` + fx.sourceName + `","name":"bf-agent","build":"bf-build-1"}}`
	depID := insertLegacyDeployment(t, fx.db, fx.targetID, specJSON, nil)

	if _, err := fx.store.BackfillSourceAccountIDs(context.Background()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	got := readSourceAccountID(t, fx.db, depID)
	if got == nil {
		t.Fatal("source_account_id still NULL after backfill")
	}
	if *got != fx.sourceID {
		t.Errorf("source_account_id = %q, want %q (source account)", *got, fx.sourceID)
	}
}

func TestBackfillSourceAccountIDs_FallsBackWhenSpecNameMisses(t *testing.T) {
	fx := setupBackfillFixture(t)
	specJSON := `{"source":{"account":"does-not-exist-` + deployid.New() + `"}}`
	depID := insertLegacyDeployment(t, fx.db, fx.targetID, specJSON, nil)

	if _, err := fx.store.BackfillSourceAccountIDs(context.Background()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	got := readSourceAccountID(t, fx.db, depID)
	if got == nil {
		t.Fatal("source_account_id still NULL after backfill")
	}
	if *got != fx.targetID {
		t.Errorf("source_account_id = %q, want %q (target as fallback)", *got, fx.targetID)
	}
}

func TestBackfillSourceAccountIDs_FallsBackWhenSpecHasNoSource(t *testing.T) {
	fx := setupBackfillFixture(t)
	depID := insertLegacyDeployment(t, fx.db, fx.targetID, `{"spec":"v1"}`, nil)

	if _, err := fx.store.BackfillSourceAccountIDs(context.Background()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	got := readSourceAccountID(t, fx.db, depID)
	if got == nil {
		t.Fatal("source_account_id still NULL after backfill")
	}
	if *got != fx.targetID {
		t.Errorf("source_account_id = %q, want %q (target as fallback)", *got, fx.targetID)
	}
}

func TestBackfillSourceAccountIDs_FallsBackOnMalformedSpec(t *testing.T) {
	fx := setupBackfillFixture(t)
	depID := insertLegacyDeployment(t, fx.db, fx.targetID, `{not json`, nil)

	if _, err := fx.store.BackfillSourceAccountIDs(context.Background()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	got := readSourceAccountID(t, fx.db, depID)
	if got == nil {
		t.Fatal("source_account_id still NULL after backfill on malformed spec")
	}
	if *got != fx.targetID {
		t.Errorf("source_account_id = %q, want %q (target as fallback)", *got, fx.targetID)
	}
}

/*
TestBackfillSourceAccountIDs_DoesNotClobberPopulatedRows is the
regression guard for the race described above: once new code has written
a value into source_account_id, the backfill running on a replica pod
must not overwrite it, even if the spec JSON would point elsewhere.
*/
func TestBackfillSourceAccountIDs_DoesNotClobberPopulatedRows(t *testing.T) {
	fx := setupBackfillFixture(t)
	preset := fx.targetID
	specJSON := `{"source":{"account":"` + fx.sourceName + `"}}`
	depID := insertLegacyDeployment(t, fx.db, fx.targetID, specJSON, &preset)

	if _, err := fx.store.BackfillSourceAccountIDs(context.Background()); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	got := readSourceAccountID(t, fx.db, depID)
	if got == nil || *got != preset {
		t.Errorf("preset row was modified: got=%v want=%q", got, preset)
	}
}

func TestBackfillSourceAccountIDs_ReRunIsNoOpForSameFixture(t *testing.T) {
	fx := setupBackfillFixture(t)
	specJSON := `{"source":{"account":"` + fx.sourceName + `"}}`
	legacyID := insertLegacyDeployment(t, fx.db, fx.targetID, specJSON, nil)

	if _, err := fx.store.BackfillSourceAccountIDs(context.Background()); err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	first := readSourceAccountID(t, fx.db, legacyID)
	if first == nil || *first != fx.sourceID {
		t.Fatalf("first run did not populate: got=%v want=%q", first, fx.sourceID)
	}

	if _, err := fx.store.BackfillSourceAccountIDs(context.Background()); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	after := readSourceAccountID(t, fx.db, legacyID)
	if after == nil || *after != *first {
		t.Errorf("re-run changed value: first=%q after=%v", *first, after)
	}
}

func TestBackfillSourceAccountIDs_SpecNamesPublisherButRowBuildNotOnPublisher(t *testing.T) {
	fx := setupBackfillFixture(t)
	specJSON := `{"source":{"account":"` + fx.sourceName + `","name":"bf-agent","build":"bf-build-1"}}`
	// Row pins bf-agent@(ghost) — only registered on target, not source — while spec.names source.
	ghost := "bf-ghost-" + strings.ToLower(deployid.New())
	tgtAcct := &account.Account{ID: fx.targetID, Name: fx.targetName}
	registerBackfillAgent(t, fx.index, tgtAcct, "bf-agent", ghost)
	depID := insertLegacyDeploymentWithBuild(t, fx.db, fx.targetID, "bf-agent", ghost, specJSON, nil)

	res, err := fx.store.BackfillSourceAccountIDs(context.Background())
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res.FromSelf == 0 {
		t.Fatalf("expected FromSelf > 0, got %+v", res)
	}
	got := readSourceAccountID(t, fx.db, depID)
	if got == nil || *got != fx.targetID {
		t.Fatalf("want target fallback publisher %q; got %v res=%+v", fx.targetID, got, res)
	}
}

func TestBackfillSourceAccountIDs_SkipWhenOwningTupleAbsent(t *testing.T) {
	fx := setupBackfillFixture(t)
	specJSON := `{"spec":"v1"}`
	ghost := "bf-ghost-" + strings.ToLower(deployid.New())
	depID := insertLegacyDeploymentWithBuild(t, fx.db, fx.targetID, "bf-agent", ghost, specJSON, nil)

	res, err := fx.store.BackfillSourceAccountIDs(context.Background())
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res.SkippedInvalidLineage == 0 {
		t.Fatalf("expected SkippedInvalidLineage > 0, got %+v", res)
	}
	got := readSourceAccountID(t, fx.db, depID)
	if got != nil {
		t.Fatalf("expected NULL source_account_id, got %v res=%+v", *got, res)
	}
}
