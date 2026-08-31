//go:build integration

package accessgroup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func accessGroupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must be set for integration tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func createAccessGroupTestAccount(t *testing.T, db *sql.DB, userID string) string {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin fixture transaction: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var accountID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO accounts (name, type, owner_user_id)
		VALUES ($1, 'organization', $2)
		RETURNING id
	`, fmt.Sprintf("group-test-%d", time.Now().UnixNano()), userID).Scan(&accountID); err != nil {
		t.Fatalf("insert fixture account: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO account_members (account_id, user_id) VALUES ($1, $2)`, accountID, userID); err != nil {
		t.Fatalf("insert fixture member: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit fixture transaction: %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID) })
	return accountID
}

func TestStoreLifecycleIntegration(t *testing.T) {
	db := accessGroupTestDB(t)
	accountID := createAccessGroupTestAccount(t, db, "creator-1")
	store := NewStore(db)
	ctx := context.Background()

	group, err := store.Create(ctx, CreateParams{AccountID: accountID, Name: "Support", CreatedByUserID: "creator-1"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if group.CreatedByUserID != "creator-1" {
		t.Fatalf("creator = %q, want creator-1", group.CreatedByUserID)
	}
	if _, err := store.Create(ctx, CreateParams{AccountID: accountID, Name: "support", CreatedByUserID: "creator-1"}); !errors.Is(err, ErrNameExists) {
		t.Fatalf("duplicate name error = %v, want ErrNameExists", err)
	}
	if err := store.SetStatus(ctx, accountID, group.ID, "creator-1", StatusArchived); err != nil {
		t.Fatalf("archive group: %v", err)
	}
	if _, err := store.Create(ctx, CreateParams{AccountID: accountID, Name: "support", CreatedByUserID: "creator-1"}); err != nil {
		t.Fatalf("reuse archived name: %v", err)
	}
	if err := store.SetStatus(ctx, accountID, group.ID, "creator-1", StatusRestoring); !errors.Is(err, ErrNameExists) {
		t.Fatalf("restore collision error = %v, want ErrNameExists", err)
	}
}
