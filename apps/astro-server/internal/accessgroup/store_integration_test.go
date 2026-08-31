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

func createAccessGroupTestAccount(t *testing.T, db *sql.DB, users ...string) string {
	t.Helper()
	if len(users) == 0 {
		t.Fatal("at least one account member is required")
	}
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin fixture transaction: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var accountID string
	name := fmt.Sprintf("ag-test-%d", time.Now().UnixNano())
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO accounts (name, type, owner_user_id)
		VALUES ($1, 'organization', $2)
		RETURNING id
	`, name, users[0]).Scan(&accountID); err != nil {
		t.Fatalf("insert fixture account: %v", err)
	}
	for _, userID := range users {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO account_members (account_id, user_id)
			VALUES ($1, $2)
		`, accountID, userID); err != nil {
			t.Fatalf("insert fixture member: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit fixture transaction: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	})
	return accountID
}

func TestStoreCreateTransactionIntegration(t *testing.T) {
	db := accessGroupTestDB(t)
	accountID := createAccessGroupTestAccount(t, db, "creator-1")
	store := NewStore(db)
	ctx := context.Background()

	group, err := store.Create(ctx, CreateParams{
		AccountID:       accountID,
		Name:            "Platform Engineering",
		CreatedByUserID: "creator-1",
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	var role MembershipRole
	var addedBy string
	if err := db.QueryRowContext(ctx, `
		SELECT role, added_by_user_id
		FROM access_group_memberships
		WHERE group_id = $1 AND user_id = 'creator-1'
	`, group.ID).Scan(&role, &addedBy); err != nil {
		t.Fatalf("load creator membership: %v", err)
	}
	if role != MembershipRoleAdmin || addedBy != "creator-1" {
		t.Fatalf("creator membership = (%q, %q), want (admin, creator-1)", role, addedBy)
	}

	if _, err := store.Create(ctx, CreateParams{
		AccountID:       accountID,
		Name:            "Broken group",
		CreatedByUserID: "missing-member",
	}); err == nil {
		t.Fatal("expected creator membership foreign-key failure")
	}
	var groupCount, membershipCount int
	if err := db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM access_groups WHERE account_id = $1 AND name = 'Broken group'),
			(SELECT COUNT(*) FROM access_group_memberships WHERE account_id = $1 AND user_id = 'missing-member')
	`, accountID).Scan(&groupCount, &membershipCount); err != nil {
		t.Fatalf("verify rollback: %v", err)
	}
	if groupCount != 0 || membershipCount != 0 {
		t.Fatalf("failed create left (%d groups, %d memberships), want none", groupCount, membershipCount)
	}
}

func TestStoreCreateRejectsCaseInsensitiveActiveNameIntegration(t *testing.T) {
	db := accessGroupTestDB(t)
	accountID := createAccessGroupTestAccount(t, db, "creator-2")
	store := NewStore(db)
	ctx := context.Background()

	if _, err := store.Create(ctx, CreateParams{
		AccountID: accountID, Name: "Data Platform", CreatedByUserID: "creator-2",
	}); err != nil {
		t.Fatalf("create first group: %v", err)
	}
	_, err := store.Create(ctx, CreateParams{
		AccountID: accountID, Name: "data platform", CreatedByUserID: "creator-2",
	})
	if !errors.Is(err, ErrNameExists) {
		t.Fatalf("duplicate name error = %v, want ErrNameExists", err)
	}
}

func TestStoreRestoreRejectsActiveNameCollisionIntegration(t *testing.T) {
	db := accessGroupTestDB(t)
	accountID := createAccessGroupTestAccount(t, db, "creator-3")
	store := NewStore(db)
	ctx := context.Background()

	archived, err := store.Create(ctx, CreateParams{
		AccountID: accountID, Name: "Support", CreatedByUserID: "creator-3",
	})
	if err != nil {
		t.Fatalf("create group to archive: %v", err)
	}
	if err := store.SetStatus(ctx, accountID, archived.ID, "creator-3", StatusArchived); err != nil {
		t.Fatalf("archive group: %v", err)
	}
	if _, err := store.Create(ctx, CreateParams{
		AccountID: accountID, Name: "support", CreatedByUserID: "creator-3",
	}); err != nil {
		t.Fatalf("reuse archived group name: %v", err)
	}
	if err := store.SetStatus(ctx, accountID, archived.ID, "creator-3", StatusRestoring); !errors.Is(err, ErrNameExists) {
		t.Fatalf("restore collision error = %v, want ErrNameExists", err)
	}
	got, err := store.Get(ctx, accountID, archived.ID)
	if err != nil {
		t.Fatalf("get archived group: %v", err)
	}
	if got.Status != StatusArchived {
		t.Fatalf("status after rejected restore = %q, want archived", got.Status)
	}
}

func TestStoreUpsertMembershipLifecycleIntegration(t *testing.T) {
	db := accessGroupTestDB(t)
	accountID := createAccessGroupTestAccount(t, db, "creator-4", "actor-4", "removed-4", "active-4")
	store := NewStore(db)
	ctx := context.Background()
	group, err := store.Create(ctx, CreateParams{
		AccountID: accountID, Name: "Engineering", CreatedByUserID: "creator-4",
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	removed, err := store.UpsertMembership(ctx, Membership{
		GroupID: group.ID, AccountID: accountID, UserID: "removed-4",
		Role: MembershipRoleMember, AddedByUserID: "creator-4",
	})
	if err != nil {
		t.Fatalf("add membership to remove: %v", err)
	}
	if err := store.RemoveMembership(ctx, accountID, group.ID, "removed-4", "creator-4"); err != nil {
		t.Fatalf("soft-remove membership: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	resurrected, err := store.UpsertMembership(ctx, Membership{
		GroupID: group.ID, AccountID: accountID, UserID: "removed-4",
		Role: MembershipRoleAdmin, AddedByUserID: "actor-4",
	})
	if err != nil {
		t.Fatalf("resurrect membership: %v", err)
	}
	if resurrected.Role != MembershipRoleAdmin || resurrected.AddedByUserID != "actor-4" || resurrected.RemovedAt != nil || !resurrected.AddedAt.After(removed.AddedAt) {
		t.Fatalf("unexpected resurrected membership: %+v", resurrected)
	}

	active, err := store.UpsertMembership(ctx, Membership{
		GroupID: group.ID, AccountID: accountID, UserID: "active-4",
		Role: MembershipRoleAdmin, AddedByUserID: "creator-4",
	})
	if err != nil {
		t.Fatalf("add active admin: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	preserved, err := store.UpsertMembership(ctx, Membership{
		GroupID: group.ID, AccountID: accountID, UserID: "active-4",
		Role: MembershipRoleMember, AddedByUserID: "actor-4",
	})
	if err != nil {
		t.Fatalf("upsert active admin: %v", err)
	}
	if preserved.Role != MembershipRoleAdmin || preserved.AddedByUserID != "creator-4" || !preserved.AddedAt.Equal(active.AddedAt) {
		t.Fatalf("active membership changed: got %+v, original %+v", preserved, active)
	}
}
