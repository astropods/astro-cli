package deploymentstore

import (
	"database/sql"
	"os"
	"testing"

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

func TestSaveDeployment_FirstDeploy(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d, err := store.SaveDeployment(accountID, "agent-a", "build-1", "ns-test", `{"spec":"v1"}`)
	if err != nil {
		t.Fatalf("SaveDeployment failed: %v", err)
	}
	if d.Status != "active" {
		t.Errorf("expected status 'active', got %q", d.Status)
	}
	if d.AgentName != "agent-a" {
		t.Errorf("expected agent_name 'agent-a', got %q", d.AgentName)
	}
}

func TestSaveDeployment_Redeploy(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	d1, err := store.SaveDeployment(accountID, "agent-b", "build-1", "ns-test", `{"spec":"v1"}`)
	if err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}

	d2, err := store.SaveDeployment(accountID, "agent-b", "build-2", "ns-test", `{"spec":"v2"}`)
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
	_, err = store.SaveDeployment(accountID, "agent-c", "build-1", "ns-test", `{"spec":"v1"}`)
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

func TestGetDeploymentHistory(t *testing.T) {
	db := testDB(t)
	accountID := ensureTestAccount(t, db)
	store := NewStore(db)

	_, _ = store.SaveDeployment(accountID, "agent-d", "build-1", "ns-test", `{"v":1}`)
	_, _ = store.SaveDeployment(accountID, "agent-d", "build-2", "ns-test", `{"v":2}`)
	_, _ = store.SaveDeployment(accountID, "agent-d", "build-3", "ns-test", `{"v":3}`)

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

	_, _ = store.SaveDeployment(accountID, "agent-e", "build-1", "ns-test", `{"spec":"v1"}`)

	err := store.MarkUndeployed(accountID, "agent-e")
	if err != nil {
		t.Fatalf("MarkUndeployed failed: %v", err)
	}

	d, err := store.GetActiveDeployment(accountID, "agent-e")
	if err != nil {
		t.Fatalf("GetActiveDeployment failed: %v", err)
	}
	if d != nil {
		t.Errorf("expected no active deployment after MarkUndeployed, got %+v", d)
	}

	// MarkUndeployed on already-undeployed agent should not error
	err = store.MarkUndeployed(accountID, "agent-e")
	if err != nil {
		t.Fatalf("MarkUndeployed on already-undeployed should not error: %v", err)
	}
}
