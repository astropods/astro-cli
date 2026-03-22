//go:build integration

package e2e

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	_ "github.com/lib/pq"
)

// ensureDeleteTestAccount creates a fresh account for deletion tests.
// Unlike ensureTestAccount, each call creates a unique account to avoid conflicts.
func ensureDeleteTestAccount(t *testing.T, store *account.AccountStore, name string) *account.Account {
	t.Helper()
	// Create a throwaway user ID — deletion tests don't need real auth
	acct, err := store.Create(name, "personal", "user-delete-test", "")
	if err != nil {
		t.Fatalf("failed to create test account %q: %v", name, err)
	}
	return acct
}

// TestMarkDeleted_FiltersByDeletedAt verifies that after MarkDeleted,
// GetByName, GetByID, and GetAccountsForUser all exclude the account.
func TestMarkDeleted_FiltersByDeletedAt(t *testing.T) {
	db := testDB(t)
	accountStore := account.NewAccountStore(db)

	acct := ensureDeleteTestAccount(t, accountStore, "del-filter-"+deployid.New())
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID) })

	// Before deletion — account is visible
	if _, err := accountStore.GetByName(acct.Name); err != nil {
		t.Fatalf("GetByName before delete: %v", err)
	}
	if _, err := accountStore.GetByID(acct.ID); err != nil {
		t.Fatalf("GetByID before delete: %v", err)
	}
	accounts, err := accountStore.GetAccountsForUser("user-delete-test")
	if err != nil {
		t.Fatalf("GetAccountsForUser before delete: %v", err)
	}
	found := false
	for _, a := range accounts {
		if a.ID == acct.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("account should appear in GetAccountsForUser before deletion")
	}

	// Soft-delete
	if err := accountStore.MarkDeleted(acct.ID); err != nil {
		t.Fatalf("MarkDeleted: %v", err)
	}

	// After deletion — account is hidden
	if _, err := accountStore.GetByName(acct.Name); err == nil {
		t.Error("GetByName should fail after MarkDeleted")
	}
	if _, err := accountStore.GetByID(acct.ID); err == nil {
		t.Error("GetByID should fail after MarkDeleted")
	}
	accounts, err = accountStore.GetAccountsForUser("user-delete-test")
	if err != nil {
		t.Fatalf("GetAccountsForUser after delete: %v", err)
	}
	for _, a := range accounts {
		if a.ID == acct.ID {
			t.Error("deleted account should NOT appear in GetAccountsForUser")
		}
	}
}

// TestMarkDeleted_AlreadyDeleted verifies that calling MarkDeleted on an
// already-deleted account returns an error.
func TestMarkDeleted_AlreadyDeleted(t *testing.T) {
	db := testDB(t)
	accountStore := account.NewAccountStore(db)

	acct := ensureDeleteTestAccount(t, accountStore, "del-dup-"+deployid.New())
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID) })

	if err := accountStore.MarkDeleted(acct.ID); err != nil {
		t.Fatalf("first MarkDeleted: %v", err)
	}

	// Second call should fail
	if err := accountStore.MarkDeleted(acct.ID); err == nil {
		t.Error("second MarkDeleted should return error for already-deleted account")
	}
}

// TestCascadeDeleteAccount verifies that ON DELETE CASCADE removes child rows
// when the account row is hard-deleted.
func TestCascadeDeleteAccount(t *testing.T) {
	db := testDB(t)
	accountStore := account.NewAccountStore(db)
	deployStore := ds.NewStore(db)

	acct := ensureDeleteTestAccount(t, accountStore, "del-cascade-"+deployid.New())
	// No cleanup needed — we hard-delete in this test

	// Create a deployment
	dep, err := deployStore.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID:          deployid.New(),
		AccountID:   acct.ID,
		AgentName:   "cascade-agent",
		DisplayName: "Cascade Agent",
		BuildID:     "b1",
		Namespace:   "ns-cascade-" + deployid.New(),
		SpecJSON:    `{"spec":"deployment/v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	// Create an agent_message_counts row
	_, err = db.Exec(
		`INSERT INTO agent_message_counts (account_id, agent_name) VALUES ($1, $2)`,
		acct.ID, "cascade-agent",
	)
	if err != nil {
		t.Fatalf("insert agent_message_counts: %v", err)
	}

	// Hard-delete the account
	_, err = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID)
	if err != nil {
		t.Fatalf("DELETE account: %v", err)
	}

	// Deployment should be gone (cascaded)
	d, err := deployStore.GetDeploymentByID(dep.ID)
	if err != nil {
		t.Fatalf("GetDeploymentByID: %v", err)
	}
	if d != nil {
		t.Error("deployment should be cascade-deleted with account")
	}

	// agent_message_counts should be gone
	var count int
	err = db.QueryRow(
		`SELECT COUNT(*) FROM agent_message_counts WHERE account_id = $1`, acct.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count agent_message_counts: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 agent_message_counts rows, got %d", count)
	}
}
