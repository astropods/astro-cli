package deploymentstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployid"
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

func TestSaveDeployment_Redeploy(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d1, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-b",
		DisplayName: "Agent B", BuildID: "build-1", Namespace: "ns-test",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}
	// Mark first as active so the second deploy will mark it as undeployed
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d1.ID)

	d2, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-b",
		DisplayName: "Agent B", BuildID: "build-2", Namespace: "ns-test",
		SpecJSON: `{"spec":"v2"}`,
	}, nil)
	if err != nil {
		t.Fatalf("second deploy failed: %v", err)
	}

	if d2.Status != "pending" {
		t.Errorf("new deployment should be pending, got %q", d2.Status)
	}

	// Check that first deployment is now undeployed
	var status string
	err = db.QueryRow("SELECT status FROM deployments WHERE id = $1", d1.ID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query first deployment: %v", err)
	}
	if status != "undeployed" {
		t.Errorf("first deployment should be 'undeployed', got %q", status)
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

func TestMarkUndeployed(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	dep, _ := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "agent-e",
		DisplayName: "Agent E", BuildID: "build-1", Namespace: "ns-test",
		SpecJSON: `{"spec":"v1"}`,
	}, nil)
	// Mark as active so MarkUndeployedByID has an active row
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", dep.ID)

	err := store.MarkUndeployedByID(dep.ID)
	if err != nil {
		t.Fatalf("MarkUndeployedByID failed: %v", err)
	}

	d, err := store.GetActiveDeployment(accountID, "agent-e")
	if err != nil {
		t.Fatalf("GetActiveDeployment failed: %v", err)
	}
	if d != nil {
		t.Errorf("expected no active deployment after MarkUndeployedByID, got %+v", d)
	}

	// MarkUndeployedByID on already-undeployed should not error
	err = store.MarkUndeployedByID(dep.ID)
	if err != nil {
		t.Fatalf("MarkUndeployedByID on already-undeployed should not error: %v", err)
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

	// Update to active with an error message and details
	details := json.RawMessage(`{"reason":"test"}`)
	if err := store.UpdateStatus(d.ID, StatusActive, "all good", details); err != nil {
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
	if dep.ErrorMessage == nil || *dep.ErrorMessage != "all good" {
		t.Errorf("expected error_message 'all good', got %v", dep.ErrorMessage)
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

func TestMarkScaledDown(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "scale-agent",
		DisplayName: "Scale", BuildID: "build-1", Namespace: "ns-scale",
		SpecJSON: `{}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d.ID)

	ns := fmt.Sprintf("ns-scale-test-%s", newID()[:8])
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM scaled_namespaces WHERE namespace = $1", ns)
	})

	if err := store.MarkScaledDown(d.ID, ns); err != nil {
		t.Fatalf("MarkScaledDown failed: %v", err)
	}

	scaled, err := store.IsScaledDown(ns)
	if err != nil {
		t.Fatalf("IsScaledDown failed: %v", err)
	}
	if !scaled {
		t.Error("expected IsScaledDown=true after MarkScaledDown")
	}

	// Verify deployment status changed to scaled_down
	dep, err := store.GetDeploymentByID(d.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID failed: %v", err)
	}
	if dep.Status != StatusScaledDown {
		t.Errorf("expected status 'scaled_down', got %q", dep.Status)
	}
}

func TestClearScaledDown(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d, err := store.SaveDeploymentPending(SaveDeploymentParams{
		ID: newID(), AccountID: accountID, AgentName: "clear-scale",
		DisplayName: "ClearScale", BuildID: "build-1", Namespace: "ns-clear-scale",
		SpecJSON: `{}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending failed: %v", err)
	}
	_, _ = db.Exec("UPDATE deployments SET status = 'active' WHERE id = $1", d.ID)

	ns := fmt.Sprintf("ns-clear-test-%s", newID()[:8])
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM scaled_namespaces WHERE namespace = $1", ns)
	})

	if err := store.MarkScaledDown(d.ID, ns); err != nil {
		t.Fatalf("MarkScaledDown failed: %v", err)
	}

	if err := store.ClearScaledDown(ns); err != nil {
		t.Fatalf("ClearScaledDown failed: %v", err)
	}

	scaled, err := store.IsScaledDown(ns)
	if err != nil {
		t.Fatalf("IsScaledDown failed: %v", err)
	}
	if scaled {
		t.Error("expected IsScaledDown=false after ClearScaledDown")
	}
}

func TestIsScaledDown_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	scaled, err := store.IsScaledDown("nonexistent-namespace-xyz")
	if err != nil {
		t.Fatalf("IsScaledDown failed: %v", err)
	}
	if scaled {
		t.Error("expected IsScaledDown=false for nonexistent namespace")
	}
}
