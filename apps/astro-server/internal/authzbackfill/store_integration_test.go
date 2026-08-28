//go:build integration

package authzbackfill

import (
	"context"
	"database/sql"
	"os"
	"testing"

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

// seedAccount writes an account, its owner membership, one blueprint, and one
// vault variable. The owner FK is deferred, so the account and its member row
// must land in the same transaction.
func seedAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var accountID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO accounts (id, type, name, owner_user_id, created_at, updated_at)
		VALUES (gen_random_uuid(), 'organization', 'authzbackfill-' || substr(gen_random_uuid()::text, 1, 8), 'user_authzbackfill', now(), now())
		RETURNING id::text
	`).Scan(&accountID); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO account_members (account_id, user_id) VALUES ($1::uuid, 'user_authzbackfill')`, accountID); err != nil {
		t.Fatalf("insert account member: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO agents (account_id, name, registry, uid, created_at, updated_at)
		VALUES ($1::uuid, 'support', 'ecr', gen_random_uuid(), now(), now())
	`, accountID); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO account_variables (account_id, name) VALUES ($1::uuid, 'API_KEY')`, accountID); err != nil {
		t.Fatalf("insert account variable: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM accounts WHERE id = $1::uuid`, accountID)
	})
	return accountID
}

func TestListResourcesSkipsAccountVariables(t *testing.T) {
	db := testDB(t)
	accountID := seedAccount(t, db)

	resources, err := NewStore(db).ListResources(context.Background(), []string{accountID})
	if err != nil {
		t.Fatalf("ListResources() error = %v", err)
	}

	got := resources[accountID]
	if len(got) != 1 {
		t.Fatalf("resources = %+v, want one blueprint", got)
	}
	if got[0].Ref.Type != "blueprint" || got[0].Name != "support" {
		t.Fatalf("resource = %+v, want the blueprint", got[0])
	}
}
