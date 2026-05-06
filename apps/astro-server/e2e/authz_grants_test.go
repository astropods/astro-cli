//go:build integration

package e2e

// End-to-end coverage for the deployment_authorization_grants table:
//
//   - The deploy handler's atomicity guarantee — when the grants write
//     fails inside SaveDeploymentPending's txFn, the deployment row must
//     not survive. Pre-fix this was the documented fail-open: the
//     deployment row committed, the grants rolled back, and the no-grants
//     owner-fallback widened access.
//
//   - The schema-level CHECK constraints. Sqlmock can't catch a wrong
//     CHECK, so these only show up against real Postgres.
//
// Bring up the e2e Postgres once and target this file directly:
//
//	moon run astro-server:e2e.setup
//	cd apps/astro-server
//	DATABASE_URL=postgres://postgres:postgres@localhost:5433/astro_e2e?sslmode=disable \
//	  go test -tags integration -race -v -run TestE2E_AuthzGrants ./e2e/...

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authorizationstore"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

// Happy path: SaveDeploymentPending's txFn calls ReplaceGrantsTx; both
// land in the same transaction and are visible after commit.
func TestE2E_AuthzGrants_AtomicWrite_BothCommit(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	accountID := ensureTestAccount(t, db)
	depID := deployid.New()
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM deployment_authorization_grants WHERE deployment_id = $1", depID)
	})

	_, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: "authz-atomic-ok",
		DisplayName: "Authz Atomic OK", BuildID: "b1",
		Namespace: "ns-authz-atomic-ok-" + deployid.Compact(deployid.New()),
		SpecJSON:  `{"spec":"deployment/v1"}`,
	}, func(tx *sql.Tx, id string) error {
		return authorizationstore.ReplaceGrantsTx(tx, id, []authorizationstore.Grant{
			{DeploymentID: id, SubjectType: authorizationstore.SubjectTypeAnyone, SubjectID: "", Adapter: authorizationstore.AdapterWeb},
		})
	})
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	if !grantsDeploymentExists(t, db, depID) {
		t.Fatal("deployment row not visible after successful save")
	}
	if got := grantsCount(t, db, depID); got != 1 {
		t.Errorf("grants after commit: got %d, want 1", got)
	}
}

// Rollback path: when the grants insert violates a schema CHECK, the
// caller's txFn returns the error and SaveDeploymentPending rolls back
// the entire transaction — neither the deployment row nor any grants
// remain. This is the fail-open fix Matt called out: pre-fix the
// deployment row committed and the grants rolled back independently.
func TestE2E_AuthzGrants_AtomicWrite_GrantsFailureRollsBackDeployment(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	accountID := ensureTestAccount(t, db)
	depID := deployid.New()

	// anyone + non-empty subject_id violates the anyone_empty_check constraint,
	// so the INSERT inside ReplaceGrantsTx fails. Any production failure
	// mode (deadlock, transient network, etc.) lands us in the same
	// rollback path.
	_, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: "authz-atomic-fail",
		DisplayName: "Authz Atomic Fail", BuildID: "b1",
		Namespace: "ns-authz-atomic-fail-" + deployid.Compact(deployid.New()),
		SpecJSON:  `{"spec":"deployment/v1"}`,
	}, func(tx *sql.Tx, id string) error {
		return authorizationstore.ReplaceGrantsTx(tx, id, []authorizationstore.Grant{
			{DeploymentID: id, SubjectType: authorizationstore.SubjectTypeAnyone, SubjectID: "non-empty", Adapter: authorizationstore.AdapterWeb},
		})
	})
	if err == nil {
		t.Fatal("expected SaveDeploymentPending to fail when grants insert violates the anyone_empty_check constraint")
	}

	if grantsDeploymentExists(t, db, depID) {
		t.Errorf("deployment %s persisted after grants insert failed; the SaveDeploymentPending transaction should have rolled back the deployment row", depID)
	}
	if got := grantsCount(t, db, depID); got != 0 {
		t.Errorf("expected 0 grants after rollback, got %d", got)
	}
}

// Schema CHECK: anyone under slack is now legal — collapses to the same
// scope as account_id:<owner> for slack since slack identity always
// resolves to the bot's owning account.
func TestE2E_AuthzGrants_SchemaCheck_AnyoneOnSlackAccepted(t *testing.T) {
	db := testDB(t)
	depID := seedDeploymentForGrantsTest(t, db)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM deployment_authorization_grants WHERE deployment_id = $1", depID)
	})

	if _, err := db.Exec(`
		INSERT INTO deployment_authorization_grants
		    (deployment_id, subject_type, subject_id, adapter)
		VALUES ($1, 'anyone', '', 'slack')
	`, depID); err != nil {
		t.Errorf("anyone+slack INSERT was rejected; the user_web_only_check constraint should not block anyone: %v", err)
	}
}

// Schema CHECK: user under slack is now accepted. The previous
// user_web_only_check constraint blocked these rows when slack identity
// resolution couldn't produce a user candidate; with slack_identity_mappings
// in place, the resolver does emit a user candidate when the team_id is
// linked, so user grants on slack are storable and matchable.
func TestE2E_AuthzGrants_SchemaCheck_UserOnSlackAccepted(t *testing.T) {
	db := testDB(t)
	depID := seedDeploymentForGrantsTest(t, db)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM deployment_authorization_grants WHERE deployment_id = $1", depID)
	})

	if _, err := db.Exec(`
		INSERT INTO deployment_authorization_grants
		    (deployment_id, subject_type, subject_id, adapter)
		VALUES ($1, 'user', 'alice', 'slack')
	`, depID); err != nil {
		t.Errorf("user+slack INSERT was rejected; the user_web_only_check constraint should be gone: %v", err)
	}
}

// Schema CHECK: account on slack is legal (the conventional case — the
// bot's owning account is the only thing the server resolves slack
// identity to).
func TestE2E_AuthzGrants_SchemaCheck_AccountOnSlackAccepted(t *testing.T) {
	db := testDB(t)
	depID := seedDeploymentForGrantsTest(t, db)
	accountID := ensureTestAccount(t, db)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM deployment_authorization_grants WHERE deployment_id = $1", depID)
	})

	if _, err := db.Exec(`
		INSERT INTO deployment_authorization_grants
		    (deployment_id, subject_type, subject_id, adapter)
		VALUES ($1, 'account', $2, 'slack')
	`, depID, accountID); err != nil {
		t.Errorf("account+slack INSERT was rejected: %v", err)
	}
}

// seedDeploymentForGrantsTest writes a deployment row that the grants
// table can FK to. It does NOT write any grants of its own.
func seedDeploymentForGrantsTest(t *testing.T, db *sql.DB) string {
	t.Helper()
	store := ds.NewStore(db)
	accountID := ensureTestAccount(t, db)
	depID := deployid.New()
	if _, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: depID, AccountID: accountID, AgentName: "authz-schema-check",
		DisplayName: "Authz Schema Check " + depID, BuildID: "b1",
		Namespace: "ns-authz-schema-" + deployid.Compact(deployid.New()),
		SpecJSON:  `{"spec":"deployment/v1"}`,
	}, nil); err != nil {
		t.Fatalf("seed deployment: %v", err)
	}
	return depID
}

func grantsDeploymentExists(t *testing.T, db *sql.DB, depID string) bool {
	t.Helper()
	var found int
	err := db.QueryRow(`SELECT 1 FROM deployments WHERE id = $1`, depID).Scan(&found)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false
		}
		t.Fatalf("check deployment exists: %v", err)
	}
	return found == 1
}

func grantsCount(t *testing.T, db *sql.DB, depID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`
		SELECT count(*) FROM deployment_authorization_grants WHERE deployment_id = $1
	`, depID).Scan(&n); err != nil {
		t.Fatalf("count grants: %v", err)
	}
	return n
}
