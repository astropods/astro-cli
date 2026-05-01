package knowledgestore

import (
	"context"
	"database/sql"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployid"
)

// ensureTestDeployment inserts a minimal deployment row owned by accountID and
// returns its id. The row is removed in t.Cleanup. Used as the FK target for
// knowledge_store_bindings tests.
func ensureTestDeployment(t *testing.T, db *sql.DB, accountID string) string {
	t.Helper()
	id := deployid.New()
	_, err := db.Exec(`
		INSERT INTO deployments (id, account_id, agent_name, build_id, namespace,
		    deployment_spec_json, status, status_changed_at, deployed_at)
		VALUES ($1, $2, 'test-agent', 'b1', $3, '{}', 'active', NOW(), NOW())
	`, id, accountID, "ns-"+id)
	if err != nil {
		t.Fatalf("insert deployment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM deployments WHERE id = $1`, id)
	})
	return id
}

// ensureTestKnowledgeStore creates a ready knowledge store under accountID and
// returns its id.
func ensureTestKnowledgeStore(t *testing.T, s *Store, accountID, name string) string {
	t.Helper()
	id := deployid.New()
	if _, err := s.Create(CreateParams{
		ID:        id,
		AccountID: accountID,
		Name:      name,
		ARN:       "arn:knowledge:test-ks-account:" + name,
		Provider:  "postgres",
		Storage:   "10Gi",
	}); err != nil {
		t.Fatalf("create store %q: %v", name, err)
	}
	if err := s.SetStatus(id, StatusReady); err != nil {
		t.Fatalf("set ready: %v", err)
	}
	return id
}

// withTx runs fn inside a transaction so SaveBindings (which requires a *sql.Tx)
// can be called from a test, and commits on success.
func withTx(t *testing.T, db *sql.DB, fn func(tx *sql.Tx)) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() {
		if t.Failed() {
			_ = tx.Rollback()
		}
	}()
	fn(tx)
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// SaveBindings called with a non-empty map should replace existing rows for
// the deployment.
func TestSaveBindings_ReplacesExisting(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)
	accountID := ensureTestKSAccount(t, db)
	depID := ensureTestDeployment(t, db, accountID)
	storeA := ensureTestKnowledgeStore(t, s, accountID, "store-a")
	storeB := ensureTestKnowledgeStore(t, s, accountID, "store-b")

	withTx(t, db, func(tx *sql.Tx) {
		if err := s.SaveBindings(context.Background(), tx, depID, map[string]string{
			"users": storeA,
		}); err != nil {
			t.Fatalf("first SaveBindings: %v", err)
		}
	})

	withTx(t, db, func(tx *sql.Tx) {
		if err := s.SaveBindings(context.Background(), tx, depID, map[string]string{
			"users": storeB,
		}); err != nil {
			t.Fatalf("second SaveBindings: %v", err)
		}
	})

	got, err := s.GetBindingsForDeployment(context.Background(), depID)
	if err != nil {
		t.Fatalf("GetBindingsForDeployment: %v", err)
	}
	if got["users"] != storeB {
		t.Errorf("users: got %q, want %q (replacement)", got["users"], storeB)
	}
}

// SaveBindings called with an empty map on a deployment that already has
// bindings should clear all of them.
func TestSaveBindings_EmptyMapClearsAll(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)
	accountID := ensureTestKSAccount(t, db)
	depID := ensureTestDeployment(t, db, accountID)
	storeA := ensureTestKnowledgeStore(t, s, accountID, "store-a")
	storeB := ensureTestKnowledgeStore(t, s, accountID, "store-b")

	withTx(t, db, func(tx *sql.Tx) {
		if err := s.SaveBindings(context.Background(), tx, depID, map[string]string{
			"users":    storeA,
			"sessions": storeB,
		}); err != nil {
			t.Fatalf("seed SaveBindings: %v", err)
		}
	})

	// Sanity: rows exist.
	if got, _ := s.GetBindingsForDeployment(context.Background(), depID); len(got) != 2 {
		t.Fatalf("seed: expected 2 bindings, got %d", len(got))
	}

	// Clear with an empty map.
	withTx(t, db, func(tx *sql.Tx) {
		if err := s.SaveBindings(context.Background(), tx, depID, map[string]string{}); err != nil {
			t.Fatalf("clear SaveBindings: %v", err)
		}
	})

	got, err := s.GetBindingsForDeployment(context.Background(), depID)
	if err != nil {
		t.Fatalf("GetBindingsForDeployment: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 bindings after clear, got %d: %v", len(got), got)
	}
}

// SaveBindings called with a nil map should also clear — same contract as the
// empty map. The deploy handler currently passes nil when the submitted spec
// has no bound entries, so the persistence layer must treat both cases
// identically.
func TestSaveBindings_NilMapClearsAll(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)
	accountID := ensureTestKSAccount(t, db)
	depID := ensureTestDeployment(t, db, accountID)
	storeA := ensureTestKnowledgeStore(t, s, accountID, "store-a")

	withTx(t, db, func(tx *sql.Tx) {
		if err := s.SaveBindings(context.Background(), tx, depID, map[string]string{
			"users": storeA,
		}); err != nil {
			t.Fatalf("seed SaveBindings: %v", err)
		}
	})

	withTx(t, db, func(tx *sql.Tx) {
		if err := s.SaveBindings(context.Background(), tx, depID, nil); err != nil {
			t.Fatalf("clear SaveBindings: %v", err)
		}
	})

	got, err := s.GetBindingsForDeployment(context.Background(), depID)
	if err != nil {
		t.Fatalf("GetBindingsForDeployment: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 bindings after nil-map clear, got %d", len(got))
	}
}
