package account

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
)

func bindingsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping database: %v", err)
	}
	return db
}

func registerTestCluster(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO clusters (id, region, eks_cluster_name, eks_cluster_endpoint)
		VALUES ($1, 'region-a', $1, 'https://eks.example')
		ON CONFLICT (id) DO NOTHING`, id)
	if err != nil {
		t.Fatalf("register cluster: %v", err)
	}
}

func boundClusters(t *testing.T, db *sql.DB, accountID string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT cluster_id FROM account_clusters WHERE account_id = $1 ORDER BY cluster_id`, accountID)
	if err != nil {
		t.Fatalf("read bindings: %v", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan binding: %v", err)
		}
		out = append(out, id)
	}
	return out
}

func TestCreateBindsThePrimaryCluster(t *testing.T) {
	db := bindingsTestDB(t)
	registerTestCluster(t, db, "itest-create-primary")
	store := NewAccountStoreWithClusters(db, clusterid.New("itest-create-primary"))

	name := fmt.Sprintf("itest-bind-%d", os.Getpid())
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM accounts WHERE name = $1`, name) })

	acct, err := store.Create(name, "personal", "user-itest", "Bind Test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got := boundClusters(t, db, acct.ID)
	if len(got) != 1 || got[0] != "itest-create-primary" {
		t.Fatalf("bindings = %v, want the primary bound at creation", got)
	}
}

func TestCreateWithoutAPrimaryLeavesTheAccountUnbound(t *testing.T) {
	db := bindingsTestDB(t)
	store := NewAccountStore(db)

	name := fmt.Sprintf("itest-nobind-%d", os.Getpid())
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM accounts WHERE name = $1`, name) })

	acct, err := store.Create(name, "personal", "user-itest", "No Bind Test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got := boundClusters(t, db, acct.ID); len(got) != 0 {
		t.Fatalf("bindings = %v, want none without a configured primary", got)
	}
}

func TestBackfillPrimaryBindingsCoversExistingAccounts(t *testing.T) {
	db := bindingsTestDB(t)
	registerTestCluster(t, db, "itest-backfill-primary")

	name := fmt.Sprintf("itest-backfill-%d", os.Getpid())
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM accounts WHERE name = $1`, name) })

	acct, err := NewAccountStore(db).Create(name, "personal", "user-itest", "Backfill Test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := boundClusters(t, db, acct.ID); len(got) != 0 {
		t.Fatalf("bindings = %v, want the account to start unbound", got)
	}

	bindings := NewClusterBindings(db, clusterid.New("itest-backfill-primary"))
	if _, err := bindings.BackfillPrimaryBindings(context.Background()); err != nil {
		t.Fatalf("BackfillPrimaryBindings: %v", err)
	}

	got := boundClusters(t, db, acct.ID)
	if len(got) != 1 || got[0] != "itest-backfill-primary" {
		t.Fatalf("bindings = %v, want the primary backfilled", got)
	}

	before := boundClusters(t, db, acct.ID)
	if _, err := bindings.BackfillPrimaryBindings(context.Background()); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if after := boundClusters(t, db, acct.ID); len(after) != len(before) {
		t.Errorf("second pass changed bindings: %v then %v", before, after)
	}
}

func TestBackfillPrimaryBindingsLeavesABoundAccountAlone(t *testing.T) {
	db := bindingsTestDB(t)
	registerTestCluster(t, db, "itest-keep-primary")
	registerTestCluster(t, db, "itest-keep-eu")

	name := fmt.Sprintf("itest-keep-%d", os.Getpid())
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM accounts WHERE name = $1`, name) })

	acct, err := NewAccountStore(db).Create(name, "personal", "user-itest", "Keep Test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO account_clusters (account_id, cluster_id, is_default) VALUES ($1, 'itest-keep-eu', true)
	`, acct.ID); err != nil {
		t.Fatalf("seed binding: %v", err)
	}

	bindings := NewClusterBindings(db, clusterid.New("itest-keep-primary"))
	if _, err := bindings.BackfillPrimaryBindings(context.Background()); err != nil {
		t.Fatalf("BackfillPrimaryBindings: %v", err)
	}

	got := boundClusters(t, db, acct.ID)
	if len(got) != 1 || got[0] != "itest-keep-eu" {
		t.Fatalf("bindings = %v, want an account confined to eu left alone", got)
	}
}
