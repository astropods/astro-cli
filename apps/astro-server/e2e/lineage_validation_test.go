//go:build integration

// Integration coverage for deploymentstore.Store's optional LineageValidator
// gate. CI job: `Integration tests (astro-server + Postgres)` in test.yml.
// Exercises the wired path that production runs:
//
//	store := ds.NewStore(db).WithLineageValidator(agentindex.NewIndexWithDB(db))
//
// The validator's job is to reject Save/UpdateDeploymentPending writes whose
// (SourceAccountID, AgentName, BuildID) tuple does not refer to a real
// published agent_versions row. The cases below pin the boundary:
//
//  1. Save: missing tuple              → rejected.
//  2. Save: matching tuple seeded      → accepted.
//  3. Save: empty SourceAccountID      → no validation, accepted (legacy/ancient
//     rows that predate the column resolve via spec-JSON fallback at read time
//     and are unaffected by this gate).
//  4. Save: cross-account drift        → rejected. SourceAccountID names acctA
//     but the only matching version row is on acctB. This is the lineage-
//     spoofing case the validator exists to close: a row that resolves to the
//     wrong publisher would feed a misleading upgrade signal.
//  5. Save: operational validator error → rejected (fail-closed contract).
//     Uses stubValidator below to simulate a non-sql.ErrNoRows error so we
//     don't have to induce real DB stress. Without this case a regression
//     that swallows transient errors and proceeds without validation would
//     still pass every other test.
//  6. Update: validator runs            → exercised twice (accept + reject) to
//     pin that the gate fires on both write paths, not only on Save.
//
// The "accept" cases require a seeded agent_versions row, which we create via
// agentindex.Register. The "reject" cases pass tuples we deliberately do not
// seed.
package e2e

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	_ "github.com/lib/pq"
)

// stubValidator is a hand-rolled LineageValidator used to simulate
// failure modes that the real agentindex-backed validator would surface
// only under DB stress. The real validator distinguishes "tuple not
// found" (logical) from "query failed" (operational); the integration
// tests below cover the logical path against real Postgres, and this
// stub covers the operational path so a regression that fails-open on
// transient errors gets caught.
type stubValidator struct{ err error }

func (s stubValidator) ValidateLineage(string, string, string) error { return s.err }

type lineageFixture struct {
	db        *sql.DB
	idx       *agentindex.Index
	store     *ds.Store
	sourceID  string
	deployID  string
	otherID   string
	agentName string
	buildID   string
}

// setupLineageFixture creates three accounts and wires a Store with a
// real agentindex-backed LineageValidator. Three accounts: source (the
// publisher whose tuple we'll seed), deployer (the account that owns the
// deployment), and "other" (used by the cross-account drift case to seed
// a same-named agent on a different publisher).
func setupLineageFixture(t *testing.T) *lineageFixture {
	t.Helper()
	db := testDB(t)
	idx := agentindex.NewIndexWithDB(db)
	store := ds.NewStore(db).WithLineageValidator(idx)

	suffix := strings.ToLower(deployid.New())
	sourceID := insertLineageAccount(t, db, "lv-src-"+suffix)
	deployID := insertLineageAccount(t, db, "lv-dep-"+suffix)
	otherID := insertLineageAccount(t, db, "lv-other-"+suffix)

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM accounts WHERE id IN ($1, $2, $3)", sourceID, deployID, otherID)
	})

	return &lineageFixture{
		db: db, idx: idx, store: store,
		sourceID: sourceID, deployID: deployID, otherID: otherID,
		agentName: "lv-agent-" + suffix,
		buildID:   "lv-build-" + suffix,
	}
}

func insertLineageAccount(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(
		`WITH acct AS (INSERT INTO accounts (name, type, owner_user_id) VALUES ($1, 'organization', 'test-owner') RETURNING id), member AS (INSERT INTO account_members (account_id, user_id) SELECT id, 'test-owner' FROM acct ON CONFLICT DO NOTHING) SELECT id FROM acct`,
		name,
	).Scan(&id); err != nil {
		t.Fatalf("create account %s: %v", name, err)
	}
	return id
}

// seedVersion publishes (accountID, name, buildID) so the validator's
// existence check resolves. The spec/readme/cards content is irrelevant
// to the validator — it only checks the row's presence.
func (f *lineageFixture) seedVersion(t *testing.T, accountID, name, buildID string) {
	t.Helper()
	if err := f.idx.Register(accountID, name, buildID, "registry.io", "ecr-ns",
		map[string]any{"name": name}, "", "", ""); err != nil {
		t.Fatalf("Register(%s/%s@%s): %v", accountID, name, buildID, err)
	}
	t.Cleanup(func() {
		// agent_versions rows cascade with the accounts cleanup, but
		// remove agents explicitly so per-suffix isolation is total.
		_, _ = f.db.Exec("DELETE FROM agents WHERE account_id = $1 AND name = $2", accountID, name)
	})
}

// saveDeployment is a thin wrapper around SaveDeploymentPending so each
// test reads as one assertion-bearing line. Returns the error verbatim so
// callers can inspect wrapping.
func (f *lineageFixture) saveDeployment(sourceAccountID, agentName, buildID string) error {
	id := deployid.New()
	_, err := f.store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID:              id,
		AccountID:       f.deployID,
		SourceAccountID: sourceAccountID,
		AgentName:       agentName,
		BuildID:         buildID,
		Namespace:       "ns-" + id,
		SpecJSON:        "{}",
	}, nil)
	return err
}

// ── Save: rejection cases ────────────────────────────────────────────────

func TestSaveDeploymentPending_RejectsUnknownTuple(t *testing.T) {
	fx := setupLineageFixture(t)

	// fx.sourceID is a real account but no version was published for
	// (sourceID, agentName, buildID), so the validator must reject. We
	// only assert on the "lineage validation failed" wrapper — the inner
	// error string belongs to agentindex.GetVersion and shouldn't be
	// double-asserted here, otherwise rephrasing GetVersion's error
	// breaks this test for no security-relevant reason.
	err := fx.saveDeployment(fx.sourceID, fx.agentName, fx.buildID)
	if err == nil {
		t.Fatal("Save with unseeded tuple should be rejected, got nil")
	}
	if !strings.Contains(err.Error(), "lineage validation failed") {
		t.Errorf("error %q missing 'lineage validation failed' wrapper "+
			"(this is the contract callers/log readers depend on to recognize the gate)",
			err.Error())
	}

	// Pin the design invariant: rejection happens BEFORE tx.Begin, not as a
	// mid-transaction abort. If a future refactor moves validateLineage
	// inside the transaction (e.g. between INSERT and COMMIT for a
	// "consistent within tx" rationale), a rolled-back partial write could
	// still be observable on a different connection during the lifetime of
	// the transaction. Asserting zero rows on a fresh read enforces the
	// short-circuit shape — no INSERT statement was ever issued.
	if got := countDeployments(t, fx.db, fx.deployID, fx.agentName); got != 0 {
		t.Errorf("expected 0 deployments after rejection, got %d "+
			"(validator must reject before tx.Begin so no INSERT is issued)", got)
	}
}

// TestSaveDeploymentPending_FailsClosedOnValidatorError pins the
// fail-closed contract on operational validator failures.
//
// agentindex.GetVersion returns two distinct error shapes: "build not
// found" for sql.ErrNoRows (the logical case the integration tests
// above cover) and "failed to query version: …" for any other DB error
// (transient failures, connection drops, query timeouts, etc.). The
// Store treats both identically: the write is rejected. Without this
// test, a regression that catches the operational case and downgrades
// it to "log a warning, proceed without validation" would let bad rows
// through under stress while every existing test still passes.
//
// The stub validator lets us fabricate a non-sql.ErrNoRows error
// without inducing real DB stress in the test harness. The real Store
// against real Postgres is still the system under test — only the
// validator is faked.
func TestSaveDeploymentPending_FailsClosedOnValidatorError(t *testing.T) {
	db := testDB(t)
	deployerID := insertLineageAccount(t, db, "lv-fc-dep-"+strings.ToLower(deployid.New()))
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", deployerID) })

	simulated := errors.New("simulated transient db error")
	store := ds.NewStore(db).WithLineageValidator(stubValidator{err: simulated})

	id := deployid.New()
	_, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID:              id,
		AccountID:       deployerID,
		SourceAccountID: deployerID, // any non-empty value triggers the gate
		AgentName:       "any-agent",
		BuildID:         "any-build",
		Namespace:       "ns-" + id,
		SpecJSON:        "{}",
	}, nil)

	if err == nil {
		t.Fatal("Save must fail-closed on validator error, got nil " +
			"(silently proceeding under operational stress is the regression " +
			"this test guards against)")
	}
	if !errors.Is(err, simulated) {
		t.Errorf("error %q should wrap the simulated error via %%w; got cause %v "+
			"(downstream callers need errors.Is(err, x) to work for retry/log triage)",
			err.Error(), errors.Unwrap(err))
	}
	if !strings.Contains(err.Error(), "lineage validation failed") {
		t.Errorf("error %q missing 'lineage validation failed' wrapper", err.Error())
	}
	// Same shape invariant as RejectsUnknownTuple: no row is written when
	// the gate rejects, including on operational errors.
	if got := countDeployments(t, db, deployerID, "any-agent"); got != 0 {
		t.Errorf("expected 0 deployments after fail-closed rejection, got %d", got)
	}
}

func TestSaveDeploymentPending_RejectsCrossAccountDrift(t *testing.T) {
	fx := setupLineageFixture(t)
	// Seed the version on acctOther — same agent name, same build_id.
	fx.seedVersion(t, fx.otherID, fx.agentName, fx.buildID)

	// Now write a deployment claiming sourceID published it. The version
	// exists, but under a different account. This is the lineage-spoofing
	// shape: source_account_id points at an account that does not actually
	// publish the named build.
	err := fx.saveDeployment(fx.sourceID, fx.agentName, fx.buildID)
	if err == nil {
		t.Fatal("cross-account-drift Save should be rejected, got nil")
	}
	if !strings.Contains(err.Error(), "lineage validation failed") {
		t.Errorf("error %q missing 'lineage validation failed' wrapper", err.Error())
	}
	// And: the same tuple under the *correct* publisher (acctOther) must
	// still be writeable. This proves the validator rejects the *attribution*,
	// not the build_id outright.
	if err := fx.saveDeployment(fx.otherID, fx.agentName, fx.buildID); err != nil {
		t.Errorf("Save with correct publisher should succeed, got: %v", err)
	}
}

// ── Save: acceptance cases ───────────────────────────────────────────────

func TestSaveDeploymentPending_AcceptsKnownTuple(t *testing.T) {
	fx := setupLineageFixture(t)
	fx.seedVersion(t, fx.sourceID, fx.agentName, fx.buildID)

	if err := fx.saveDeployment(fx.sourceID, fx.agentName, fx.buildID); err != nil {
		t.Fatalf("Save with seeded tuple should succeed, got: %v", err)
	}
	if got := countDeployments(t, fx.db, fx.deployID, fx.agentName); got != 1 {
		t.Errorf("expected 1 deployment row after accepted Save, got %d", got)
	}
}

func TestSaveDeploymentPending_SkipsValidationWhenSourceAccountIDEmpty(t *testing.T) {
	fx := setupLineageFixture(t)

	// SourceAccountID == "" simulates a write that pre-dates the column or a
	// caller that intentionally omits attribution. The validator must
	// short-circuit even though the (sourceID, agentName, buildID) tuple is
	// not seeded — the gate only applies when SourceAccountID is set.
	if err := fx.saveDeployment("", fx.agentName, fx.buildID); err != nil {
		t.Fatalf("Save with empty SourceAccountID should bypass validator, got: %v", err)
	}
	if got := countDeployments(t, fx.db, fx.deployID, fx.agentName); got != 1 {
		t.Errorf("expected 1 deployment row after empty-source Save, got %d", got)
	}
}

// ── Update: gate fires on the redeploy path too ─────────────────────────

// TestUpdateDeploymentPending_RejectsUnknownTuple pins that the validator
// runs on Update as well. Without this test, the gate could regress on
// Update only and pure-Save tests would still pass.
func TestUpdateDeploymentPending_RejectsUnknownTuple(t *testing.T) {
	fx := setupLineageFixture(t)
	// Seed and Save the *first* build so we have a row to update.
	build1 := fx.buildID
	build2 := fx.buildID + "-v2"
	fx.seedVersion(t, fx.sourceID, fx.agentName, build1)

	id := deployid.New()
	if _, err := fx.store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID:              id,
		AccountID:       fx.deployID,
		SourceAccountID: fx.sourceID,
		AgentName:       fx.agentName,
		BuildID:         build1,
		Namespace:       "ns-" + id,
		SpecJSON:        "{}",
	}, nil); err != nil {
		t.Fatalf("seed Save failed: %v", err)
	}

	// Now redeploy with an unseeded build_id. Validator must reject before
	// the row is mutated. Note the build switch — Update also recomputes
	// source_account_id, which is exactly what makes lineage drift possible
	// on the redeploy path.
	_, err := fx.store.UpdateDeploymentPending(ds.SaveDeploymentParams{
		ID:              id,
		AccountID:       fx.deployID,
		SourceAccountID: fx.sourceID,
		AgentName:       fx.agentName,
		BuildID:         build2, // not seeded
		Namespace:       "ns-" + id,
		SpecJSON:        "{}",
	}, nil)
	if err == nil {
		t.Fatal("Update with unseeded build should be rejected, got nil")
	}
	if !strings.Contains(err.Error(), "lineage validation failed") {
		t.Errorf("error %q missing 'lineage validation failed' wrapper", err.Error())
	}

	// Sanity: original build_id is still recorded — the rejection rolled back
	// the entire write, not just the lineage update.
	var current string
	if err := fx.db.QueryRow(`SELECT build_id FROM deployments WHERE id = $1`, id).Scan(&current); err != nil {
		t.Fatalf("read back build_id: %v", err)
	}
	if current != build1 {
		t.Errorf("expected build_id to remain %q after rejected Update, got %q "+
			"(validator must short-circuit before the UPDATE statement)",
			build1, current)
	}
}

func TestUpdateDeploymentPending_AcceptsKnownTuple(t *testing.T) {
	fx := setupLineageFixture(t)
	build1 := fx.buildID
	build2 := fx.buildID + "-v2"
	fx.seedVersion(t, fx.sourceID, fx.agentName, build1)
	fx.seedVersion(t, fx.sourceID, fx.agentName, build2)

	id := deployid.New()
	if _, err := fx.store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID:              id,
		AccountID:       fx.deployID,
		SourceAccountID: fx.sourceID,
		AgentName:       fx.agentName,
		BuildID:         build1,
		Namespace:       "ns-" + id,
		SpecJSON:        "{}",
	}, nil); err != nil {
		t.Fatalf("seed Save failed: %v", err)
	}

	if _, err := fx.store.UpdateDeploymentPending(ds.SaveDeploymentParams{
		ID:              id,
		AccountID:       fx.deployID,
		SourceAccountID: fx.sourceID,
		AgentName:       fx.agentName,
		BuildID:         build2, // both seeded
		Namespace:       "ns-" + id,
		SpecJSON:        "{}",
	}, nil); err != nil {
		t.Fatalf("Update with seeded build should succeed, got: %v", err)
	}

	var current string
	if err := fx.db.QueryRow(`SELECT build_id FROM deployments WHERE id = $1`, id).Scan(&current); err != nil {
		t.Fatalf("read back build_id: %v", err)
	}
	if current != build2 {
		t.Errorf("expected build_id to be %q after accepted Update, got %q",
			build2, current)
	}
}

// countDeployments is a small read-side helper used to assert that a
// rejected Save left no row behind.
func countDeployments(t *testing.T, db *sql.DB, accountID, agentName string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM deployments WHERE account_id = $1 AND agent_name = $2`,
		accountID, agentName,
	).Scan(&n); err != nil {
		t.Fatalf("count deployments: %v", err)
	}
	return n
}
