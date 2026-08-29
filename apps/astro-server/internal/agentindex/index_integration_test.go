//go:build integration

package agentindex

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func integrationIndex(t *testing.T) (*Index, string) {
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

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var accountID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO accounts (id, type, name, owner_user_id, created_at, updated_at)
		VALUES (gen_random_uuid(), 'organization', 'agentindex-' || substr(gen_random_uuid()::text, 1, 8), 'user_agentindex', now(), now())
		RETURNING id::text
	`).Scan(&accountID); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO account_members (account_id, user_id, created_at) VALUES ($1, 'user_agentindex', now())
	`, accountID); err != nil {
		t.Fatalf("insert member: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM accounts WHERE id = $1`, accountID) })

	return NewIndexWithDB(db), accountID
}

// agents.uid is NOT NULL, so a caller that supplies no id has to be given one
// rather than writing a null.
func TestCreateWithoutResourceIDStillGetsOne(t *testing.T) {
	idx, accountID := integrationIndex(t)

	uid, err := idx.CreateWithResourceID(accountID, "no-id-supplied", "")
	if err != nil {
		t.Fatalf("CreateWithResourceID() error = %v", err)
	}
	if uid == "" {
		t.Fatal("CreateWithResourceID() returned an empty resource id")
	}
}

func TestCreateKeepsTheSuppliedResourceID(t *testing.T) {
	idx, accountID := integrationIndex(t)

	const supplied = "9f0b2a44-6f1c-4d2e-9a3b-2f5c6d7e8a90"
	uid, err := idx.CreateWithResourceID(accountID, "id-supplied", supplied)
	if err != nil {
		t.Fatalf("CreateWithResourceID() error = %v", err)
	}
	if uid != supplied {
		t.Fatalf("resource id = %q, want %q", uid, supplied)
	}
}

// The id is the blueprint's authorization identity, so re-registering an
// existing blueprint must not move it.
func TestRegisterKeepsTheOriginalResourceID(t *testing.T) {
	idx, accountID := integrationIndex(t)

	first, err := idx.CreateWithResourceID(accountID, "reregistered", "")
	if err != nil {
		t.Fatalf("CreateWithResourceID() error = %v", err)
	}
	second, err := idx.RegisterWithResourceID(accountID, "reregistered", "build-1", "ecr", accountID,
		map[string]any{}, "", "", "", "3d1f4c55-7a2b-4e6f-8c9d-1b2a3c4d5e6f")
	if err != nil {
		t.Fatalf("RegisterWithResourceID() error = %v", err)
	}
	if second != first {
		t.Fatalf("resource id moved from %q to %q", first, second)
	}
}
