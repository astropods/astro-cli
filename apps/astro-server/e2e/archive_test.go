//go:build integration

package e2e

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	_ "github.com/lib/pq"
)

// TestArchive_HidesFromListings verifies that after Archive, the agent
// disappears from List, ListForAccount, and ListPublicAgents but is still
// retrievable via Get (so existing deployments keep working).
func TestArchive_HidesFromListings(t *testing.T) {
	db := testDB(t)
	index := agentindex.NewIndexWithDB(db)
	accountID := ensureTestAccount(t, db)
	agentName := "archive-e2e-" + deployid.New()

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM agents WHERE account_id = $1 AND name = $2", accountID, agentName)
	})

	// Register an agent with a version
	err := index.Register(accountID, agentName, "build-1", "test-registry", "test-ns",
		map[string]any{"spec": "deployment/v1"}, "readme", "", "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Make it public so we can test ListPublicAgents
	if err := index.SetVisibility(accountID, agentName, "public"); err != nil {
		t.Fatalf("SetVisibility: %v", err)
	}

	// Before archive — agent appears in all listings
	assertAgentInList(t, index, accountID, agentName, true)
	assertAgentInAccountList(t, index, accountID, agentName, true)
	assertAgentInPublicList(t, index, agentName, true)

	// Archive
	if err := index.Archive(accountID, agentName); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	// After archive — agent is hidden from listings
	assertAgentInList(t, index, accountID, agentName, false)
	assertAgentInAccountList(t, index, accountID, agentName, false)
	assertAgentInPublicList(t, index, agentName, false)

	// But still accessible via direct Get (for existing deployments)
	agent, err := index.Get(accountID, agentName)
	if err != nil {
		t.Fatalf("Get should still return archived agent: %v", err)
	}
	if agent.ArchivedAt == nil {
		t.Error("ArchivedAt should be set after archiving")
	}
	if len(agent.Versions) != 1 {
		t.Errorf("expected 1 version, got %d", len(agent.Versions))
	}
}

// TestArchive_AlreadyArchived verifies that archiving an already-archived
// agent returns an error (idempotency guard).
func TestArchive_AlreadyArchived(t *testing.T) {
	db := testDB(t)
	index := agentindex.NewIndexWithDB(db)
	accountID := ensureTestAccount(t, db)
	agentName := "archive-dup-" + deployid.New()

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM agents WHERE account_id = $1 AND name = $2", accountID, agentName)
	})

	err := index.Register(accountID, agentName, "build-1", "test-registry", "test-ns",
		map[string]any{"spec": "deployment/v1"}, "", "", "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := index.Archive(accountID, agentName); err != nil {
		t.Fatalf("first Archive: %v", err)
	}

	// Second archive should fail
	if err := index.Archive(accountID, agentName); err == nil {
		t.Error("second Archive should return error for already-archived agent")
	}
}

// TestArchive_NonexistentAgent verifies that archiving a non-existent agent
// returns an error.
func TestArchive_NonexistentAgent(t *testing.T) {
	db := testDB(t)
	index := agentindex.NewIndexWithDB(db)
	accountID := ensureTestAccount(t, db)

	if err := index.Archive(accountID, "does-not-exist"); err == nil {
		t.Error("Archive of non-existent agent should return error")
	}
}

// TestArchive_RegisterAfterArchive verifies that re-registering (pushing a
// new build to) an archived agent un-archives it by updating the row.
func TestArchive_RegisterAfterArchive(t *testing.T) {
	db := testDB(t)
	index := agentindex.NewIndexWithDB(db)
	accountID := ensureTestAccount(t, db)
	agentName := "archive-rereg-" + deployid.New()

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM agents WHERE account_id = $1 AND name = $2", accountID, agentName)
	})

	// Register, archive, then register a new build
	err := index.Register(accountID, agentName, "build-1", "test-registry", "test-ns",
		map[string]any{"spec": "deployment/v1"}, "", "", "")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := index.Archive(accountID, agentName); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	// Re-register with a new build
	err = index.Register(accountID, agentName, "build-2", "test-registry", "test-ns",
		map[string]any{"spec": "deployment/v1"}, "", "", "")
	if err != nil {
		t.Fatalf("Register after archive: %v", err)
	}

	// Agent should reappear in listings (Register's ON CONFLICT clears archived_at)
	assertAgentInAccountList(t, index, accountID, agentName, true)
}

// --- helpers ---

func assertAgentInList(t *testing.T, index *agentindex.Index, accountID, name string, want bool) {
	t.Helper()
	agents, err := index.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, a := range agents {
		if a.AccountID == accountID && a.Name == name {
			found = true
			break
		}
	}
	if found != want {
		t.Errorf("List: agent %q found=%v, want=%v", name, found, want)
	}
}

func assertAgentInAccountList(t *testing.T, index *agentindex.Index, accountID, name string, want bool) {
	t.Helper()
	page, err := index.ListForAccount(accountID, agentindex.BlueprintListOptions{})
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	found := false
	for _, a := range page.Agents {
		if a.Name == name {
			found = true
			break
		}
	}
	if found != want {
		t.Errorf("ListForAccount: agent %q found=%v, want=%v", name, found, want)
	}
}

func assertAgentInPublicList(t *testing.T, index *agentindex.Index, name string, want bool) {
	t.Helper()
	page, err := index.ListPublicAgents(agentindex.BlueprintListOptions{})
	if err != nil {
		t.Fatalf("ListPublicAgents: %v", err)
	}
	found := false
	for _, a := range page.Agents {
		if a.Name == name {
			found = true
			break
		}
	}
	if found != want {
		t.Errorf("ListPublicAgents: agent %q found=%v, want=%v", name, found, want)
	}
}
