package deploymentstore

import (
	"database/sql"
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
