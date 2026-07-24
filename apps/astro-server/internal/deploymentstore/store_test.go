package deploymentstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	spec "github.com/astropods/astro/packages/astro-spec"
	_ "github.com/lib/pq"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}
	return db
}

// ensureTestAccount inserts a test account and returns its ID.
func ensureTestAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
		INSERT INTO accounts (name, type) VALUES ('test-deploy-store', 'personal')
		ON CONFLICT DO NOTHING
		RETURNING id
	`).Scan(&id)
	if err != nil {
		// Already exists, fetch it
		err = db.QueryRow(`SELECT id FROM accounts WHERE name = 'test-deploy-store'`).Scan(&id)
		if err != nil {
			t.Fatalf("failed to get test account: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM deployments WHERE account_id = $1", id)
	})
	return id
}

func newID() string { return deployid.New() }

func TestSaveDeployment_FirstDeploy(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-a",
		DisplayName: "Agent A", BuildID: "build-1", Namespace: "ns-test",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}
	if d.Status != "pending" {
		t.Errorf("expected status 'pending', got %q", d.Status)
	}
	if d.AgentName != "agent-a" {
		t.Errorf("expected agent_name 'agent-a', got %q", d.AgentName)
	}
	if d.DisplayName != "Agent A" {
		t.Errorf("expected display_name 'Agent A', got %q", d.DisplayName)
	}
}

// Two SaveDeploymentPending calls with the same (account_id, display_name)
// while the prior row is still live must collide on the partial unique
// index — surfaced as ErrDuplicateDisplayName. Redeploys go through
// UpdateDeploymentPending, not a second insert.
func TestSaveDeployment_DuplicateDisplayName_Rejected(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d1, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-b",
		DisplayName: "Agent B", BuildID: "build-1", Namespace: "ns-test-1",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d1.ID)

	_, err = store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-b",
		DisplayName: "Agent B", BuildID: "build-2", Namespace: "ns-test-2",
		SpecJSON: `{"spec":"v2"}`,
	}, nil)
	if !errors.Is(err, ErrDuplicateDisplayName) {
		t.Fatalf("expected ErrDuplicateDisplayName, got %v", err)
	}

	// First row must remain active and untouched — the writer's prior
	// supersede block (which set undeployed_at on collision) is gone, so a
	// failed insert must not flip status or stamp undeployed_at.
	var (
		status       string
		undeployedAt sql.NullTime
	)
	if err := db.QueryRow(
		"SELECT status, undeployed_at FROM deployments WHERE id = $1", d1.ID,
	).Scan(&status, &undeployedAt); err != nil {
		t.Fatalf("failed to query first deployment: %v", err)
	}
	if status != "active" {
		t.Errorf("first deployment should remain 'active', got %q", status)
	}
	if undeployedAt.Valid {
		t.Errorf("first deployment should have NULL undeployed_at, got %v", undeployedAt.Time)
	}
}

// The broadened partial unique index covers every status except
// 'undeployed', so two pending rows can no longer slip past — the case the
// old WHERE status='active' index missed and the schema change is meant to
// fix.
func TestSaveDeployment_DuplicateDisplayName_PendingCollision(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// First insert lands as 'pending' — leave it there. Pre-fix this would
	// not have collided; post-fix the broadened index rejects.
	_, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-pending-collision",
		DisplayName: "Pending Agent", BuildID: "build-1", Namespace: "ns-pc-1",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}

	_, err = store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-pending-collision",
		DisplayName: "Pending Agent", BuildID: "build-2", Namespace: "ns-pc-2",
		SpecJSON: `{"spec":"v2"}`,
	}, nil)
	if !errors.Is(err, ErrDuplicateDisplayName) {
		t.Fatalf("expected ErrDuplicateDisplayName for pending collision, got %v", err)
	}
}

// UpdateDeploymentPending must surface the same 23505 → ErrDuplicateDisplayName
// translation. A redeploy that renames into a name owned by a different live
// row should reject; pre-fix this would have returned a generic 500 because
// only SaveDeploymentPending translated the constraint violation.
func TestUpdateDeployment_RenameCollision_Rejected(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Row A — owns display_name "Owned Name".
	depA, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-rename-a",
		DisplayName: "Owned Name", BuildID: "build-1", Namespace: "ns-rename-a",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("save A failed: %v", err)
	}
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", depA.ID)

	// Row B — distinct display_name; needs a real revision row so
	// UpdateDeploymentPending's MAX(revision)+1 lookup succeeds.
	depB, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-rename-b",
		DisplayName: "Free Name", BuildID: "build-1", Namespace: "ns-rename-b",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("save B failed: %v", err)
	}
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", depB.ID)

	// Redeploy B and rename into A's name — must reject.
	_, err = store.UpdateDeploymentPending(SaveDeploymentParams{
		ID: depB.ID, AccountID: accountID, AgentName: "agent-rename-b",
		DisplayName: "Owned Name", BuildID: "build-2", Namespace: "ns-rename-b",
		SpecJSON: `{"spec":"v2"}`,
	}, nil)
	if !errors.Is(err, ErrDuplicateDisplayName) {
		t.Fatalf("expected ErrDuplicateDisplayName for rename collision, got %v", err)
	}
}

// Once the first row is undeployed, the (account_id, display_name) slot is
// free again and a fresh deploy with the same name succeeds.
func TestSaveDeployment_DisplayNameReusableAfterUndeploy(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d1, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-b2",
		DisplayName: "Agent B2", BuildID: "build-1", Namespace: "ns-test-1",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}
	_, _ = db.Exec("UPDATE deployments SET status = 'undeployed' WHERE id = $1", d1.ID)

	if _, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-b2",
		DisplayName: "Agent B2", BuildID: "build-2", Namespace: "ns-test-2",
		SpecJSON: `{"spec":"v2"}`,
	}, nil); err != nil {
		t.Fatalf("second deploy after undeploy should succeed, got %v", err)
	}
}

func TestSaveDeployment_MultiDeploy(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Deploy same agent twice with different display names — both should be pending
	_, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-multi",
		DisplayName: "Production", BuildID: "build-1", Namespace: "ns-prod",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}

	_, err = store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-multi",
		DisplayName: "Staging", BuildID: "build-1", Namespace: "ns-staging",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("second deploy failed: %v", err)
	}

	// Mark both as active so GetActiveDeployments can find them
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE account_id = $1 AND agent_name = 'agent-multi'", accountID)

	deps, err := store.GetActiveDeployments(accountID, "agent-multi")
	if err != nil {
		t.Fatalf("GetActiveDeployments failed: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 active deployments, got %d", len(deps))
	}
}

func TestGetActiveDeployment(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Not found case
	d, err := store.GetActiveDeployment(accountID, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != nil {
		t.Errorf("expected nil for nonexistent agent, got %+v", d)
	}

	// Found case
	_, err = store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-c",
		DisplayName: "Agent C", BuildID: "build-1", Namespace: "ns-test",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}
	// Mark as active so GetActiveDeployment can find it
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE account_id = $1 AND agent_name = 'agent-c'", accountID)
	d, err = store.GetActiveDeployment(accountID, "agent-c")
	if err != nil {
		t.Fatalf("GetActiveDeployment failed: %v", err)
	}
	if d == nil {
		t.Fatal("expected deployment, got nil")
	}
	if d.BuildID != "build-1" {
		t.Errorf("expected build_id 'build-1', got %q", d.BuildID)
	}
}

func TestGetActiveDeploymentByDisplayName(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	_, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-dn",
		DisplayName: "My Agent", BuildID: "build-1", Namespace: "ns-test",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}
	// Mark as active so GetActiveDeploymentByDisplayName can find it
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE account_id = $1 AND display_name = 'My Agent'", accountID)

	d, err := store.GetActiveDeploymentByDisplayName(accountID, "My Agent")
	if err != nil {
		t.Fatalf("GetActiveDeploymentByDisplayName failed: %v", err)
	}
	if d == nil {
		t.Fatal("expected deployment, got nil")
	}
	if d.AgentName != "agent-dn" {
		t.Errorf("expected agent_name 'agent-dn', got %q", d.AgentName)
	}

	// Not found
	d, err = store.GetActiveDeploymentByDisplayName(accountID, "Nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != nil {
		t.Errorf("expected nil, got %+v", d)
	}
}

func TestGetDeploymentHistory(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	for i := 1; i <= 3; i++ {
		_, _ = store.SaveDeploymentPending(SaveDeploymentParams{
			ID: newID(), AccountID: accountID, AgentName: "agent-d",
			DisplayName: fmt.Sprintf("Agent D v%d", i), BuildID: fmt.Sprintf("build-%d", i),
			Namespace: "ns-test", SpecJSON: fmt.Sprintf(`{"v":%d}`, i),
		}, nil)
	}

	history, err := store.GetDeploymentHistory(accountID, "agent-d")
	if err != nil {
		t.Fatalf("GetDeploymentHistory failed: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 records, got %d", len(history))
	}
	// Should be ordered by deployed_at DESC (newest first)
	if history[0].BuildID != "build-3" {
		t.Errorf("expected newest first (build-3), got %q", history[0].BuildID)
	}
	if history[2].BuildID != "build-1" {
		t.Errorf("expected oldest last (build-1), got %q", history[2].BuildID)
	}
}

func TestUpdateStatus(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "status-agent",
		DisplayName: "Status", BuildID: "build-1", Namespace: "ns-status",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}

	// Update to active with an event message and details
	details := json.RawMessage(`{"reason":"test"}`)
	if err := store.UpdateStatus(d.ID, StatusUpdate{Status: StatusActive, EventMsg: "all good", ErrorDetails: details}); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify deployment row was updated
	dep, err := store.GetDeploymentByID(d.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID failed: %v", err)
	}
	if dep.Status != StatusActive {
		t.Errorf("expected status 'active', got %q", dep.Status)
	}
	if dep.ErrorMessage != nil {
		t.Errorf("expected error_message nil, got %v", *dep.ErrorMessage)
	}

	// Verify event row was created
	events, err := store.GetDeploymentEvents(d.ID, 100)
	if err != nil {
		t.Fatalf("GetDeploymentEvents failed: %v", err)
	}
	// Should have 2 events: pending (from save) + active (from UpdateStatus)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Status != StatusActive {
		t.Errorf("expected newest event 'active', got %q", events[0].Status)
	}
	if events[0].Message != "all good" {
		t.Errorf("expected event message 'all good', got %q", events[0].Message)
	}
}

func TestUpdateStatus_StampsUndeployedAt(t *testing.T) {
	// updateStatusTx is the single entry point for status changes. When the
	// new status is 'undeployed' it should populate undeployed_at in the same
	// UPDATE (replacing the old MarkUndeployedByID call which had a broken
	// WHERE-status guard). A second transition to 'undeployed' must NOT shift
	// the timestamp — the CASE guard only stamps when undeployed_at IS NULL.
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "undeploy-stamp",
		DisplayName: "Stamp", BuildID: "build-1", Namespace: "ns-undeploy-stamp",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d.ID)

	// First transition to 'undeployed' should stamp undeployed_at.
	if err := store.UpdateStatus(d.ID, StatusUpdate{Status: StatusUndeployed}); err != nil {
		t.Fatalf("UpdateStatus(undeployed) failed: %v", err)
	}
	dep, err := store.GetDeploymentByID(d.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID failed: %v", err)
	}
	if dep.UndeployedAt == nil {
		t.Fatalf("expected undeployed_at to be stamped, got nil")
	}
	firstStamp := *dep.UndeployedAt

	// A second transition to 'undeployed' must preserve the original timestamp.
	// The CASE guard `undeployed_at IS NULL THEN NOW() ELSE undeployed_at` keeps
	// the first stamp; we'd lose the original delete time otherwise.
	time.Sleep(10 * time.Millisecond) // ensure a clock tick so a buggy implementation would visibly shift
	if err := store.UpdateStatus(d.ID, StatusUpdate{Status: StatusUndeployed}); err != nil {
		t.Fatalf("UpdateStatus(undeployed) second call failed: %v", err)
	}
	dep, err = store.GetDeploymentByID(d.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID failed: %v", err)
	}
	if dep.UndeployedAt == nil {
		t.Fatalf("expected undeployed_at to still be set after second call, got nil")
	}
	if !dep.UndeployedAt.Equal(firstStamp) {
		t.Errorf("undeployed_at shifted: first=%v second=%v (expected idempotent)", firstStamp, *dep.UndeployedAt)
	}
}

func TestUpdateDeploymentPending(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Create initial deployment (revision 1)
	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "redeploy-rev",
		DisplayName: "RedeployRev", BuildID: "build-1", Namespace: "ns-redeploy-rev",
		SpecJSON: `{"v":1}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}

	// Mark active so we can redeploy
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d.ID)

	// Redeploy creates revision 2
	d2, err := store.UpdateDeploymentPending(SaveDeploymentParams{
		ID: d.ID, AccountID: accountID, AgentName: "redeploy-rev",
		DisplayName: "RedeployRev", BuildID: "build-2", Namespace: "ns-redeploy-rev",
		SpecJSON: `{"v":2}`,
	}, nil)
	if err != nil {
		t.Fatalf("UpdateDeploymentPending failed: %v", err)
	}
	if d2.Status != StatusPending {
		t.Errorf("expected status 'pending', got %q", d2.Status)
	}
	if d2.BuildID != "build-2" {
		t.Errorf("expected build_id 'build-2', got %q", d2.BuildID)
	}

	// Verify revision 2 exists and current_revision points to it
	dep, err := store.GetDeploymentByID(d.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID failed: %v", err)
	}
	if dep.CurrentRevision == nil || *dep.CurrentRevision != 2 {
		t.Errorf("expected current_revision=2, got %v", dep.CurrentRevision)
	}

	// Verify two revisions exist
	revisions, err := store.GetRevisions(d.ID)
	if err != nil {
		t.Fatalf("GetRevisions failed: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("expected 2 revisions, got %d", len(revisions))
	}
}

func TestGetDeploymentsInStatus(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Create deployments in different statuses
	d1, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "status-filter-1",
		DisplayName: "SF1", BuildID: "build-1", Namespace: "ns-sf1",
		SpecJSON: `{}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending 1 failed: %v", err)
	}

	d2, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "status-filter-2",
		DisplayName: "SF2", BuildID: "build-1", Namespace: "ns-sf2",
		SpecJSON: `{}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending 2 failed: %v", err)
	}

	d3, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "status-filter-3",
		DisplayName: "SF3", BuildID: "build-1", Namespace: "ns-sf3",
		SpecJSON: `{}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending 3 failed: %v", err)
	}

	// Set d1 to active, d2 to failed, d3 stays pending
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d1.ID)
	_, _ = db.Exec("UPDATE deployments SET status = 'failed' WHERE id = $1", d2.ID)

	// Query for pending + failed
	deps, err := store.GetDeploymentsInStatus(StatusPending, StatusFailed)
	if err != nil {
		t.Fatalf("GetDeploymentsInStatus failed: %v", err)
	}

	// Should include d2 (failed) and d3 (pending), but not d1 (active)
	found := map[string]bool{}
	for _, dep := range deps {
		found[dep.ID] = true
	}
	if !found[d2.ID] {
		t.Errorf("expected failed deployment %s in results", d2.ID)
	}
	if !found[d3.ID] {
		t.Errorf("expected pending deployment %s in results", d3.ID)
	}
	if found[d1.ID] {
		t.Errorf("did not expect active deployment %s in results", d1.ID)
	}
}

func TestScanDeployment_NullErrorDetails(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Create a deployment — error_details will be NULL
	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "null-details",
		DisplayName: "NullDetails", BuildID: "build-1", Namespace: "ns-null-details",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}

	// GetDeploymentByID uses scanDeployment — should not error on NULL error_details
	dep, err := store.GetDeploymentByID(d.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID failed with NULL error_details: %v", err)
	}
	if dep == nil {
		t.Fatal("expected deployment, got nil")
	}
	if dep.ErrorDetails != nil {
		t.Errorf("expected nil ErrorDetails, got %s", string(dep.ErrorDetails))
	}

	// Now set error_details to a non-null value and verify it scans correctly
	if err := store.UpdateStatus(d.ID, StatusUpdate{Status: StatusFailed, ErrorMsg: "boom", ErrorDetails: json.RawMessage(`{"code":"ERR_1"}`)}); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	dep, err = store.GetDeploymentByID(d.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID failed with non-null error_details: %v", err)
	}
	if dep.ErrorDetails == nil {
		t.Fatal("expected non-nil ErrorDetails")
	}
	if string(dep.ErrorDetails) != `{"code":"ERR_1"}` {
		t.Errorf("expected error_details '{\"code\":\"ERR_1\"}', got %s", string(dep.ErrorDetails))
	}
}

func TestGetDeploymentByNamespace(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ns := fmt.Sprintf("ns-byns-%s", newID()[:8])

	// Not found case
	d, err := store.GetDeploymentByNamespace(ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != nil {
		t.Errorf("expected nil for nonexistent namespace, got %+v", d)
	}

	// Create deployment and mark active
	dep, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "ns-lookup",
		DisplayName: "NsLookup", BuildID: "build-1", Namespace: ns,
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", dep.ID)

	// Found case
	d, err = store.GetDeploymentByNamespace(ns)
	if err != nil {
		t.Fatalf("GetDeploymentByNamespace failed: %v", err)
	}
	if d == nil {
		t.Fatal("expected deployment, got nil")
	}
	if d.ID != dep.ID {
		t.Errorf("expected ID %q, got %q", dep.ID, d.ID)
	}
	if d.Namespace != ns {
		t.Errorf("expected namespace %q, got %q", ns, d.Namespace)
	}

	// Undeployed should not be found
	_, _ = db.Exec("UPDATE deployments SET status = 'undeployed' WHERE id = $1", dep.ID)
	d, err = store.GetDeploymentByNamespace(ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != nil {
		t.Errorf("expected nil for undeployed namespace, got %+v", d)
	}
}

func TestListAllWithAccount(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// Create deployments in various statuses
	d1, _ := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "list-all-1",
		DisplayName: "LA1", BuildID: "build-1", Namespace: fmt.Sprintf("ns-la1-%s", newID()[:8]),
		SpecJSON: `{}`,
	}, nil)
	d2, _ := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "list-all-2",
		DisplayName: "LA2", BuildID: "build-1", Namespace: fmt.Sprintf("ns-la2-%s", newID()[:8]),
		SpecJSON: `{}`,
	}, nil)
	d3, _ := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "list-all-3",
		DisplayName: "LA3", BuildID: "build-1", Namespace: fmt.Sprintf("ns-la3-%s", newID()[:8]),
		SpecJSON: `{}`,
	}, nil)

	// d1=active, d2=failed, d3=undeployed
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d1.ID)
	_, _ = db.Exec("UPDATE deployments SET status = 'failed', error_message = 'oops' WHERE id = $1", d2.ID)
	_, _ = db.Exec("UPDATE deployments SET status = 'undeployed' WHERE id = $1", d3.ID)

	deps, err := store.ListAllWithAccount()
	if err != nil {
		t.Fatalf("ListAllWithAccount failed: %v", err)
	}

	found := map[string]bool{}
	for _, dep := range deps {
		found[dep.ID] = true
		if dep.AccountName == "" {
			t.Errorf("expected non-empty AccountName for deployment %s", dep.ID)
		}
	}

	if !found[d1.ID] {
		t.Errorf("expected active deployment %s in results", d1.ID)
	}
	if !found[d2.ID] {
		t.Errorf("expected failed deployment %s in results", d2.ID)
	}
	if found[d3.ID] {
		t.Errorf("did not expect undeployed deployment %s in results", d3.ID)
	}
}

// TestSaveDeployment_SupersedesCleansWorkloads verifies that when a new deployment
// supersedes an active one with the same display_name, the old deployment's
// normalized data (deployment_workloads, deployment_variables) is deleted.
func TestSaveDeployment_SupersedesCleansWorkloads(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "cleanup-agent"},
		Agent: spec.DeploymentAgent{
			Image:     "agent:latest",
			Replicas:  1,
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi"},
			Endpoints: map[string]spec.Endpoint{"http": {Port: 8080, Protocol: "http"}},
			Update:    spec.DefaultUpdateStrategy(),
		},
	}
	// Deploy v1 with normalized workloads
	d1, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "cleanup-agent",
		DisplayName: "Cleanup Agent", BuildID: "build-1", Namespace: "ns-cleanup",
		SpecJSON: `{"spec":"v1"}`,
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds, nil, nil)
	})
	if err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}

	// Verify workloads exist for d1
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM deployment_workloads WHERE deployment_id = $1", d1.ID).Scan(&count)
	if err != nil {
		t.Fatalf("query workloads: %v", err)
	}
	if count == 0 {
		t.Fatal("expected workloads for first deployment, got 0")
	}

	// Mark d1 as active so the second deploy will supersede it
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d1.ID)

	// Deploy v2 — should mark d1 as undeployed and clean up its workloads
	d2, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "cleanup-agent",
		DisplayName: "Cleanup Agent", BuildID: "build-2", Namespace: "ns-cleanup-2",
		SpecJSON: `{"spec":"v2"}`,
	}, func(tx *sql.Tx, depID string) error {
		return SaveNormalizedSpec(tx, depID, ds, nil, nil)
	})
	if err != nil {
		t.Fatalf("second deploy failed: %v", err)
	}

	// d1 should be undeployed
	var status string
	err = db.QueryRow("SELECT status FROM deployments WHERE id = $1", d1.ID).Scan(&status)
	if err != nil {
		t.Fatalf("query d1 status: %v", err)
	}
	if status != "undeployed" {
		t.Errorf("d1 should be undeployed, got %q", status)
	}

	// d1 workloads should be cleaned up
	err = db.QueryRow("SELECT COUNT(*) FROM deployment_workloads WHERE deployment_id = $1", d1.ID).Scan(&count)
	if err != nil {
		t.Fatalf("query d1 workloads: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 workloads for superseded deployment, got %d", count)
	}

	// d1 variables should be cleaned up
	err = db.QueryRow("SELECT COUNT(*) FROM deployment_variables WHERE deployment_id = $1", d1.ID).Scan(&count)
	if err != nil {
		t.Fatalf("query d1 variables: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 variables for superseded deployment, got %d", count)
	}

	// d2 should have its own workloads
	err = db.QueryRow("SELECT COUNT(*) FROM deployment_workloads WHERE deployment_id = $1", d2.ID).Scan(&count)
	if err != nil {
		t.Fatalf("query d2 workloads: %v", err)
	}
	if count == 0 {
		t.Error("expected workloads for new deployment, got 0")
	}
}

func TestFailStaleDeployments(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	// stale: deploying, status_changed_at well past the deadline.
	stale, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "stale-deploying",
		DisplayName: "Stale", BuildID: "build-1", Namespace: "ns-stale",
		SpecJSON: `{}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending stale failed: %v", err)
	}
	// fresh: deploying, but only just entered the status.
	fresh, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "fresh-deploying",
		DisplayName: "Fresh", BuildID: "build-1", Namespace: "ns-fresh",
		SpecJSON: `{}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending fresh failed: %v", err)
	}

	_, _ = db.Exec("UPDATE deployments SET status = 'deploying', status_changed_at = NOW() - interval '40 minutes' WHERE id = $1", stale.ID)
	_, _ = db.Exec("UPDATE deployments SET status = 'deploying', status_changed_at = NOW() - interval '2 minutes' WHERE id = $1", fresh.ID)

	ids, err := store.FailStaleDeployments(StatusDeploying, 30*time.Minute, "stuck in deploying")
	if err != nil {
		t.Fatalf("FailStaleDeployments failed: %v", err)
	}

	if len(ids) != 1 || ids[0] != stale.ID {
		t.Fatalf("failed IDs = %v, want [%s]", ids, stale.ID)
	}

	staleDep, _ := store.GetDeploymentByID(stale.ID)
	if staleDep.Status != StatusFailed {
		t.Errorf("stale deployment status = %q, want failed", staleDep.Status)
	}
	freshDep, _ := store.GetDeploymentByID(fresh.ID)
	if freshDep.Status != StatusDeploying {
		t.Errorf("fresh deployment status = %q, want deploying (not yet past deadline)", freshDep.Status)
	}

	// A failure event row was recorded for the stale deployment.
	var events int
	if err := db.QueryRow("SELECT COUNT(*) FROM deployment_events WHERE deployment_id = $1 AND status = 'failed'", stale.ID).Scan(&events); err != nil {
		t.Fatalf("query events: %v", err)
	}
	if events != 1 {
		t.Errorf("failed events for stale deployment = %d, want 1", events)
	}

	// Idempotent: the now-failed deployment is not swept again.
	ids, err = store.FailStaleDeployments(StatusDeploying, 30*time.Minute, "stuck in deploying")
	if err != nil {
		t.Fatalf("second FailStaleDeployments failed: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("second sweep failed IDs = %v, want none", ids)
	}
}
