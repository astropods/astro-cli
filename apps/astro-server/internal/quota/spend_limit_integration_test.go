//go:build integration

package quota

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

func ceilingTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must be set for integration tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// The owner has to be a member in the same statement: accounts_owner_member_fkey
// points back at account_members.
func seedCeilingAccount(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`
		WITH acct AS (
			INSERT INTO accounts (name, owner_user_id)
			VALUES ($1, 'test-owner')
			RETURNING id
		), member AS (
			INSERT INTO account_members (account_id, user_id) SELECT id, 'test-owner' FROM acct
		)
		SELECT id FROM acct`, name,
	).Scan(&id); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM accounts WHERE id = $1`, id) })
	return id
}

func TestSpendCeilingUSD_AgainstPostgres(t *testing.T) {
	db := ceilingTestDB(t)
	accountID := seedCeilingAccount(t, db, "ceiling-read-test")

	got, err := SpendCeilingUSD(context.Background(), db, accountID)
	if err != nil {
		t.Fatalf("SpendCeilingUSD with no grant: %v", err)
	}
	if got != billing.MaxSelfServeSpendUSD {
		t.Fatalf("ceiling = %v, want the default %v", got, billing.MaxSelfServeSpendUSD)
	}

	if _, err := db.Exec(
		`INSERT INTO account_limits (account_id, resource, limit_value) VALUES ($1, $2, $3)`,
		accountID, KeySpendLimit, 5000,
	); err != nil {
		t.Fatalf("write grant: %v", err)
	}

	got, err = SpendCeilingUSD(context.Background(), db, accountID)
	if err != nil {
		t.Fatalf("SpendCeilingUSD with a grant: %v", err)
	}
	if got != 5000 {
		t.Fatalf("ceiling = %v, want the granted 5000", got)
	}
}

func TestSpendCeilingUSD_CoexistsWithAResourceOverride(t *testing.T) {
	db := ceilingTestDB(t)
	accountID := seedCeilingAccount(t, db, "ceiling-coexist-test")

	for _, row := range []struct {
		resource string
		value    int64
	}{{ResourceBlueprints, 25}, {KeySpendLimit, 7500}} {
		if _, err := db.Exec(
			`INSERT INTO account_limits (account_id, resource, limit_value) VALUES ($1, $2, $3)`,
			accountID, row.resource, row.value,
		); err != nil {
			t.Fatalf("write %s: %v", row.resource, err)
		}
	}

	got, err := SpendCeilingUSD(context.Background(), db, accountID)
	if err != nil {
		t.Fatalf("SpendCeilingUSD: %v", err)
	}
	if got != 7500 {
		t.Errorf("ceiling = %v, want 7500: the blueprint override was read instead", got)
	}

	c := NewDBChecker(db, nil, map[string]int64{ResourceBlueprints: 5}, true)
	limit, err := c.effectiveLimit(context.Background(), accountID, ResourceBlueprints)
	if err != nil {
		t.Fatalf("effectiveLimit: %v", err)
	}
	if limit != 25 {
		t.Errorf("blueprint limit = %d, want 25: the spend-limit row was read instead", limit)
	}
}

func TestQuotaIncreaseRequest_AcceptsTheSpendLimitKey(t *testing.T) {
	db := ceilingTestDB(t)
	accountID := seedCeilingAccount(t, db, "ceiling-request-test")

	var id string
	if err := db.QueryRow(
		`INSERT INTO quota_increase_requests
		   (account_id, feature_key, current_usage, current_quota, requested_amount, reason, requested_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		accountID, KeySpendLimit, 812.4, 1000, 5000, "monthly batch run", "test-owner",
	).Scan(&id); err != nil {
		t.Fatalf("insert request: %v", err)
	}

	var status, featureKey string
	if err := db.QueryRow(
		`SELECT status, feature_key FROM quota_increase_requests WHERE id = $1`, id,
	).Scan(&status, &featureKey); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if status != "pending" {
		t.Errorf("status = %q, want pending", status)
	}
	if featureKey != KeySpendLimit {
		t.Errorf("feature_key = %q, want %q", featureKey, KeySpendLimit)
	}
}
