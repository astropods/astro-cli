//go:build integration

package e2e

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	ds "github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	_ "github.com/lib/pq"
)

// createDeploymentInStatus is a helper that creates a pending deployment then
// transitions it to the target status via UpdateStatus.
func createDeploymentInStatus(t *testing.T, store *ds.Store, accountID, name, status string) *ds.Deployment {
	t.Helper()
	dep, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: deployid.New(), AccountID: accountID, AgentName: name,
		DisplayName: name, BuildID: "b1",
		Namespace: fmt.Sprintf("ns-%s-%s", name, status),
		SpecJSON:  `{"spec":"deployment/v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending(%s/%s): %v", name, status, err)
	}
	if status != ds.StatusPending {
		if err := store.UpdateStatus(dep.ID, status, "", nil); err != nil {
			t.Fatalf("UpdateStatus(%s → %s): %v", name, status, err)
		}
	}
	return dep
}

// TestGetVisibleDeploymentsByAccount verifies that GetVisibleDeploymentsByAccount
// returns active, failed, pending, provisioning, and scaled_down deployments
// but excludes undeployed and undeploying ones.
func TestGetVisibleDeploymentsByAccount(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	accountID := ensureTestAccount(t, db)

	// Create one deployment in each status
	statuses := []string{
		ds.StatusActive,
		ds.StatusFailed,
		ds.StatusPending,
		ds.StatusProvisioning,
		ds.StatusScaledDown,
		ds.StatusUndeploying,
		ds.StatusUndeployed,
	}
	created := make(map[string]*ds.Deployment, len(statuses))
	for _, s := range statuses {
		created[s] = createDeploymentInStatus(t, store, accountID, "vis-"+s, s)
	}

	visible, err := store.GetVisibleDeploymentsByAccount(accountID)
	if err != nil {
		t.Fatalf("GetVisibleDeploymentsByAccount: %v", err)
	}

	visibleIDs := map[string]bool{}
	for _, d := range visible {
		visibleIDs[d.ID] = true
	}

	// Every status except undeployed SHOULD be visible
	for _, s := range []string{ds.StatusActive, ds.StatusFailed, ds.StatusPending, ds.StatusProvisioning, ds.StatusScaledDown, ds.StatusUndeploying} {
		if !visibleIDs[created[s].ID] {
			t.Errorf("deployment in status %q should be visible but was not returned", s)
		}
	}

	// Only fully undeployed should be hidden
	if visibleIDs[created[ds.StatusUndeployed].ID] {
		t.Error("deployment in status 'undeployed' should NOT be visible but was returned")
	}
}

// TestGetVisibleDeploymentsByAccount_FailedHasError verifies that failed
// deployments retain their error message when loaded via GetVisibleDeploymentsByAccount.
func TestGetVisibleDeploymentsByAccount_FailedHasError(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	accountID := ensureTestAccount(t, db)

	dep, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
		ID: deployid.New(), AccountID: accountID, AgentName: "vis-err",
		DisplayName: "Error Agent", BuildID: "b1",
		Namespace: "ns-vis-err", SpecJSON: `{"spec":"deployment/v1"}`,
	}, nil)
	if err != nil {
		t.Fatalf("SaveDeploymentPending: %v", err)
	}

	errDetails, _ := json.Marshal([]map[string]string{
		{"resource": "my-agent-agent", "kind": "Deployment", "error": "image pull backoff"},
	})
	if err := store.UpdateStatus(dep.ID, ds.StatusFailed, "partial failure", errDetails); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	visible, err := store.GetVisibleDeploymentsByAccount(accountID)
	if err != nil {
		t.Fatalf("GetVisibleDeploymentsByAccount: %v", err)
	}

	var found *ds.Deployment
	for _, d := range visible {
		if d.ID == dep.ID {
			found = d
			break
		}
	}
	if found == nil {
		t.Fatal("failed deployment not returned by GetVisibleDeploymentsByAccount")
	}
	if found.Status != ds.StatusFailed {
		t.Errorf("status: got %q, want %q", found.Status, ds.StatusFailed)
	}
	if found.ErrorMessage == nil || *found.ErrorMessage != "partial failure" {
		t.Errorf("error_message: got %v, want 'partial failure'", found.ErrorMessage)
	}
	if len(found.ErrorDetails) == 0 {
		t.Error("error_details should be populated")
	}
}

// TestGetVisibleDeploymentsByAccount_Empty verifies that an account with no
// deployments returns an empty slice (not nil/error).
func TestGetVisibleDeploymentsByAccount_Empty(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	accountID := ensureTestAccount(t, db)

	visible, err := store.GetVisibleDeploymentsByAccount(accountID)
	if err != nil {
		t.Fatalf("GetVisibleDeploymentsByAccount: %v", err)
	}
	if len(visible) != 0 {
		t.Errorf("expected 0 visible deployments, got %d", len(visible))
	}
}
