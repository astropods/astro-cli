//go:build integration

package e2e

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
)

func bindingFixture(t *testing.T, db *sql.DB) (*account.ClusterBindings, string) {
	t.Helper()

	var accountID string
	err := db.QueryRow(`
		WITH acct AS (
			INSERT INTO accounts (name, type, owner_user_id) VALUES ('bt-' || substr(gen_random_uuid()::text, 1, 8), 'personal', 'test-owner')
			RETURNING id
		), member AS (
			INSERT INTO account_members (account_id, user_id) SELECT id, 'test-owner' FROM acct
			ON CONFLICT DO NOTHING
		)
		SELECT id FROM acct`).Scan(&accountID)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}

	for _, id := range []string{"itest-primary", "itest-eu"} {
		_, err := db.Exec(`
			INSERT INTO clusters (id, region, eks_cluster_name, eks_cluster_endpoint)
			VALUES ($1, 'region-a', $1, 'https://eks.example')
			ON CONFLICT (id) DO NOTHING`, id)
		if err != nil {
			t.Fatalf("insert cluster %s: %v", id, err)
		}
	}

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM account_clusters WHERE account_id = $1`, accountID)
		_, _ = db.Exec(`DELETE FROM accounts WHERE id = $1`, accountID)
	})

	return account.NewClusterBindings(db, clusterid.New("itest-primary")), accountID
}

func boundClusters(t *testing.T, b *account.ClusterBindings, accountID string) map[string]bool {
	t.Helper()
	list, err := b.List(accountID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	out := make(map[string]bool, len(list))
	for _, binding := range list {
		out[binding.ClusterID] = binding.IsDefault
	}
	return out
}

// Account creation and the startup backfill own the binding set. A read that
// also wrote is what made an empty set ambiguous between "no cluster is
// registered" and "nobody has read this account yet".
func TestClusterBindingsListDoesNotBind(t *testing.T) {
	db := testDB(t)
	bindings, accountID := bindingFixture(t, db)

	if got := boundClusters(t, bindings, accountID); len(got) != 0 {
		t.Fatalf("bindings = %v, want a read to leave the account unbound", got)
	}
}

func TestClusterBindingsKeepExactlyOneDefault(t *testing.T) {
	db := testDB(t)
	bindings, accountID := bindingFixture(t, db)

	if err := bindings.Add(accountID, "itest-eu", false); err != nil {
		t.Fatalf("Add eu: %v", err)
	}
	got := boundClusters(t, bindings, accountID)
	if len(got) != 2 || !got["itest-primary"] || got["itest-eu"] {
		t.Fatalf("bindings = %v, want the materialized primary still default", got)
	}

	if err := bindings.Add(accountID, "itest-primary", false); err != nil {
		t.Fatalf("re-add primary: %v", err)
	}
	if got := boundClusters(t, bindings, accountID); !got["itest-primary"] {
		t.Fatalf("bindings = %v, want the primary still default", got)
	}

	if err := bindings.SetDefault(accountID, "itest-eu"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	got = boundClusters(t, bindings, accountID)
	if !got["itest-eu"] || got["itest-primary"] {
		t.Fatalf("bindings = %v, want eu default alone", got)
	}

	if err := bindings.Remove(accountID, "itest-eu"); err != nil {
		t.Fatalf("Remove eu: %v", err)
	}
	got = boundClusters(t, bindings, accountID)
	if len(got) != 1 || !got["itest-primary"] {
		t.Fatalf("bindings = %v, want the primary promoted", got)
	}
}

func TestClusterBindingsRejectUnboundCluster(t *testing.T) {
	db := testDB(t)
	bindings, accountID := bindingFixture(t, db)

	if err := bindings.SetDefault(accountID, "itest-eu"); !errors.Is(err, account.ErrClusterNotAllowed) {
		t.Fatalf("SetDefault err = %v, want ErrClusterNotAllowed", err)
	}
	if err := bindings.Remove(accountID, "itest-eu"); !errors.Is(err, account.ErrClusterNotAllowed) {
		t.Fatalf("Remove err = %v, want ErrClusterNotAllowed", err)
	}
}
