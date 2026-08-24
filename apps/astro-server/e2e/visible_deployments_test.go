//go:build integration

package e2e

import (
	"context"
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
		if err := store.UpdateStatus(dep.ID, ds.StatusUpdate{Status: status}); err != nil {
			t.Fatalf("UpdateStatus(%s → %s): %v", name, status, err)
		}
	}
	return dep
}

// TestGetVisibleDeploymentsByAccount verifies that GetVisibleDeploymentsByAccount
// returns active, failed, pending, and provisioning deployments
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
	for _, s := range []string{ds.StatusActive, ds.StatusFailed, ds.StatusPending, ds.StatusProvisioning, ds.StatusUndeploying} {
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

	errDetails, err := json.Marshal([]map[string]string{
		{"resource": "my-agent-agent", "kind": "Deployment", "error": "image pull backoff"},
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := store.UpdateStatus(dep.ID, ds.StatusUpdate{Status: ds.StatusFailed, ErrorMsg: "partial failure", ErrorDetails: errDetails}); err != nil {
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

// TestGetVisibleDeploymentsByAccount_StorePreservesAgentNameVerbatim proves the
// deployment store does NOT sanitize or transform AgentName. Whatever the handler
// writes is what GetVisibleDeploymentsByAccount returns. This means the handler is
// the sole gatekeeper for name correctness — if it accidentally writes an
// account-qualified K8s label (e.g. "simon.mindcraft"), the store will happily
// persist and return it, breaking downstream API calls.
func TestGetVisibleDeploymentsByAccount_StorePreservesAgentNameVerbatim(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	accountID := ensureTestAccount(t, db)

	cases := []struct {
		name      string
		agentName string
	}{
		{name: "plain name", agentName: "mindcraft"},
		{name: "account-qualified (buggy)", agentName: "simon.mindcraft"},
		{name: "name with special chars", agentName: "my-agent_v2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dep, err := store.SaveDeploymentPending(ds.SaveDeploymentParams{
				ID:          deployid.New(),
				AccountID:   accountID,
				AgentName:   tc.agentName,
				DisplayName: "Test Bot " + tc.agentName,
				BuildID:     "b-" + tc.agentName,
				Namespace:   "ns-" + tc.agentName,
				SpecJSON:    `{"spec":"deployment/v1"}`,
			}, nil)
			if err != nil {
				t.Fatalf("SaveDeploymentPending: %v", err)
			}
			if err := store.UpdateStatus(dep.ID, ds.StatusUpdate{Status: ds.StatusActive}); err != nil {
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
				t.Fatal("deployment not returned")
			}
			if found.AgentName != tc.agentName {
				t.Errorf("AgentName: got %q, want %q (store must preserve verbatim)", found.AgentName, tc.agentName)
			}
		})
	}
}

func TestReadableDeploymentQueriesTreatNilFGAScopeAsLegacy(t *testing.T) {
	db := testDB(t)
	store := ds.NewStore(db)
	ctx := context.Background()

	accountName := "fga-null-scope-" + deployid.New()
	var accountID string
	if err := db.QueryRowContext(ctx, `
		WITH acct AS (
			INSERT INTO accounts (name, type, owner_user_id) VALUES ($1, 'organization', 'test-owner')
			RETURNING id
		), member AS (
			INSERT INTO account_members (account_id, user_id) SELECT id, 'test-owner' FROM acct
			ON CONFLICT DO NOTHING
		)
		SELECT id FROM acct
	`, accountName).Scan(&accountID); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", accountID)
	})

	userID := "fga-null-scope-user-" + deployid.New()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO account_members (account_id, user_id)
		VALUES ($1, $2)
	`, accountID, userID); err != nil {
		t.Fatalf("insert account membership: %v", err)
	}

	deployment := createDeploymentInStatus(t, store, accountID, "fga-null-scope", ds.StatusActive)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO deployment_fga_sync (
			deployment_id, desired_state, desired_version, next_attempt_at, updated_at
		)
		VALUES ($1, 'registered', 1, NOW(), NOW())
	`, deployment.ID); err != nil {
		t.Fatalf("insert deployment FGA sync row: %v", err)
	}

	assertVisible := func(t *testing.T, fgaAccountIDs, readableDeploymentIDs []string, want, wantAccessReady bool) {
		t.Helper()
		ids, err := store.ListReadableDeploymentIDsForUser(
			ctx, userID, []string{deployment.ID}, fgaAccountIDs, readableDeploymentIDs,
		)
		if err != nil {
			t.Fatalf("list readable deployment IDs: %v", err)
		}
		if got := len(ids) == 1 && ids[0] == deployment.ID; got != want {
			t.Fatalf("readable deployment IDs = %v, want visible=%v", ids, want)
		}

		rows, err := store.ListVisibleDeploymentsForUserPage(
			ctx, userID, []string{accountID}, "", 10, nil, fgaAccountIDs, readableDeploymentIDs,
		)
		if err != nil {
			t.Fatalf("list visible deployments: %v", err)
		}
		if got := len(rows) == 1 && rows[0].Deployment.ID == deployment.ID; got != want {
			t.Fatalf("visible deployments = %v, want visible=%v", rows, want)
		}
		if want && rows[0].AccessReady != wantAccessReady {
			t.Fatalf("access ready = %v, want %v", rows[0].AccessReady, wantAccessReady)
		}
	}

	t.Run("legacy nil scope remains visible", func(t *testing.T) {
		assertVisible(t, nil, nil, true, true)
	})
	t.Run("enforced unreadable deployment is hidden", func(t *testing.T) {
		assertVisible(t, []string{accountID}, nil, false, false)
	})
	t.Run("enforced readable deployment reports access provisioning", func(t *testing.T) {
		assertVisible(t, []string{accountID}, []string{deployment.ID}, true, false)
	})
	t.Run("enforced readable deployment reports converged access", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, `
			UPDATE deployment_fga_sync
			SET synced_state = desired_state, synced_version = desired_version
			WHERE deployment_id = $1
		`, deployment.ID); err != nil {
			t.Fatalf("mark deployment FGA sync row converged: %v", err)
		}
		assertVisible(t, []string{accountID}, []string{deployment.ID}, true, true)
	})
	t.Run("non-registered intent uses legacy visibility and is ready", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, `
			UPDATE deployment_fga_sync
			SET desired_state = 'deleted'
			WHERE deployment_id = $1
		`, deployment.ID); err != nil {
			t.Fatalf("mark deployment FGA sync row deleted: %v", err)
		}
		assertVisible(t, []string{accountID}, nil, true, true)
	})
}
