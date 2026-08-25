//go:build integration

package account

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func testDB(t *testing.T) *sql.DB {
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

// seedGatewayAccount inserts one account and returns its id. sweptAt nil leaves
// the account never-swept.
func seedGatewayAccount(t *testing.T, db *sql.DB, name, bifrostID string, sweptAt *time.Time, deleted bool) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
		INSERT INTO accounts (name, bifrost_customer_id, gateway_budget_swept_at, deleted_at)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		name, bifrostID, sweptAt, func() any {
			if deleted {
				return time.Now()
			}
			return nil
		}()).Scan(&id)
	if err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM accounts WHERE id = $1`, id) })
	return id
}

// The ordering is the whole anti-starvation design and it lives in SQL, so a
// worklist built from a fake proves nothing about it.
func TestListStaleGatewayBudgetAccounts_OrdersByStaleness(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	s := NewAccountStore(db)

	old := time.Now().Add(-72 * time.Hour)
	recent := time.Now().Add(-1 * time.Minute)

	never := seedGatewayAccount(t, db, "itest-gw-never", "bf-never", nil, false)
	stale := seedGatewayAccount(t, db, "itest-gw-stale", "bf-stale", &old, false)
	fresh := seedGatewayAccount(t, db, "itest-gw-fresh", "bf-fresh", &recent, false)
	noGateway := seedGatewayAccount(t, db, "itest-gw-none", "", nil, false)
	gone := seedGatewayAccount(t, db, "itest-gw-deleted", "bf-deleted", nil, true)

	ids, err := s.ListStaleGatewayBudgetAccounts(ctx, 500)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	pos := map[string]int{}
	for i, id := range ids {
		pos[id] = i
	}
	for _, id := range []string{never, stale, fresh} {
		if _, ok := pos[id]; !ok {
			t.Fatalf("account %s missing from the worklist", id)
		}
	}
	if pos[never] > pos[stale] || pos[stale] > pos[fresh] {
		t.Errorf("order never=%d stale=%d fresh=%d, want never before stale before fresh",
			pos[never], pos[stale], pos[fresh])
	}
	// An account with no gateway customer has no budget to hold, and a deleted
	// one must not be swept at all.
	if _, ok := pos[noGateway]; ok {
		t.Error("an account with no gateway customer is in the worklist")
	}
	if _, ok := pos[gone]; ok {
		t.Error("a soft-deleted account is in the worklist")
	}
}

// Stamping is what moves an account off the front. If it did not, a bounded
// sweep would re-read the same head every tick and never reach the rest.
func TestMarkGatewayBudgetSwept_MovesAnAccountOffTheFront(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	s := NewAccountStore(db)

	older := time.Now().Add(-48 * time.Hour)
	head := seedGatewayAccount(t, db, "itest-gw-head", "bf-head", nil, false)
	next := seedGatewayAccount(t, db, "itest-gw-next", "bf-next", &older, false)

	first, err := s.ListStaleGatewayBudgetAccounts(ctx, 500)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if indexOf(first, head) > indexOf(first, next) {
		t.Fatalf("never-swept account did not lead: %v", first)
	}

	if err := s.MarkGatewayBudgetSwept(ctx, head); err != nil {
		t.Fatalf("mark: %v", err)
	}

	second, err := s.ListStaleGatewayBudgetAccounts(ctx, 500)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if indexOf(second, head) < indexOf(second, next) {
		t.Errorf("the stamped account still leads: %v", second)
	}
}

// A bounded tick must actually be bounded, or the job timeout becomes the bound.
func TestListStaleGatewayBudgetAccounts_HonorsTheLimit(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	s := NewAccountStore(db)

	for _, n := range []string{"itest-gw-l1", "itest-gw-l2", "itest-gw-l3"} {
		seedGatewayAccount(t, db, n, "bf-"+n, nil, false)
	}
	ids, err := s.ListStaleGatewayBudgetAccounts(ctx, 2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("returned %d accounts, want the limit of 2", len(ids))
	}
}

func indexOf(ids []string, want string) int {
	for i, id := range ids {
		if id == want {
			return i
		}
	}
	return len(ids) + 1
}
