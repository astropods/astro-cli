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

	d, err := store.SaveDeployment(newID(), accountID, "agent-a", "Agent A", "build-1", "ns-test", `{"spec":"v1"}`)
	if err != nil {
		t.Fatalf("SaveDeployment failed: %v", err)
	}
	if d.Status != "active" {
		t.Errorf("expected status 'active', got %q", d.Status)
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

	d1, err := store.SaveDeployment(newID(), accountID, "agent-b", "Agent B", "build-1", "ns-test", `{"spec":"v1"}`)
	if err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}

	d2, err := store.SaveDeployment(newID(), accountID, "agent-b", "Agent B", "build-2", "ns-test", `{"spec":"v2"}`)
	if err != nil {
		t.Fatalf("second deploy failed: %v", err)
	}

	if d2.Status != "active" {
		t.Errorf("new deployment should be active, got %q", d2.Status)
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

	// Deploy same agent twice with different display names — both should be active
	_, err := store.SaveDeployment(newID(), accountID, "agent-multi", "Production", "build-1", "ns-prod", `{"spec":"v1"}`)
	if err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}

	_, err = store.SaveDeployment(newID(), accountID, "agent-multi", "Staging", "build-1", "ns-staging", `{"spec":"v1"}`)
	if err != nil {
		t.Fatalf("second deploy failed: %v", err)
	}

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
	_, err = store.SaveDeployment(newID(), accountID, "agent-c", "Agent C", "build-1", "ns-test", `{"spec":"v1"}`)
	if err != nil {
		t.Fatalf("SaveDeployment failed: %v", err)
	}
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

	_, err := store.SaveDeployment(newID(), accountID, "agent-dn", "My Agent", "build-1", "ns-test", `{"spec":"v1"}`)
	if err != nil {
		t.Fatalf("SaveDeployment failed: %v", err)
	}

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
		_, _ = store.SaveDeployment(newID(), accountID, "agent-d", fmt.Sprintf("Agent D v%d", i), fmt.Sprintf("build-%d", i), "ns-test", fmt.Sprintf(`{"v":%d}`, i))
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

	dep, _ := store.SaveDeployment(newID(), accountID, "agent-e", "Agent E", "build-1", "ns-test", `{"spec":"v1"}`)

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
