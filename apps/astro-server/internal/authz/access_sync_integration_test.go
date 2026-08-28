//go:build integration

package authz

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func integrationDB(t *testing.T) *sql.DB {
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

// seedOrganizationAccount writes an account with its owner membership and one
// blueprint. The owner FK is deferred, so both rows land in one transaction.
func seedOrganizationAccount(t *testing.T, db *sql.DB) (accountID, blueprintID string) {
	t.Helper()
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := tx.QueryRowContext(ctx, `
		INSERT INTO accounts (id, type, name, owner_user_id, created_at, updated_at)
		VALUES (gen_random_uuid(), 'organization', 'authz-' || substr(gen_random_uuid()::text, 1, 8), 'user_authz', now(), now())
		RETURNING id::text
	`).Scan(&accountID); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO account_members (account_id, user_id, created_at) VALUES ($1, 'user_authz', now())
	`, accountID); err != nil {
		t.Fatalf("insert member: %v", err)
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO agents (account_id, uid, name, registry, created_at, updated_at)
		VALUES ($1, gen_random_uuid(), 'authz-blueprint', 'ecr', now(), now())
		RETURNING uid::text
	`, accountID).Scan(&blueprintID); err != nil {
		t.Fatalf("insert blueprint: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM accounts WHERE id = $1`, accountID)
	})
	return accountID, blueprintID
}

// ResourceDeleted compares an Astro external id against a uuid column for two
// of the three types, so it needs a real driver and a real schema.
func TestResourceDeletedAgainstPostgres(t *testing.T) {
	db := integrationDB(t)
	accountID, blueprintID := seedOrganizationAccount(t, db)
	store := NewResourceAccessSyncStore(db)
	ctx := context.Background()

	for _, tc := range []struct {
		name     string
		resource ResourceRef
		want     bool
	}{
		{"live account", AccountResource(accountID), false},
		{"live blueprint", BlueprintResource(blueprintID), false},
		{"unknown account", AccountResource("00000000-0000-0000-0000-000000000000"), true},
		{"unknown blueprint", BlueprintResource("00000000-0000-0000-0000-000000000000"), true},
		{"unknown deployment", DeploymentResource("dep_absent"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deleted, err := store.ResourceDeleted(ctx, tc.resource)
			if err != nil {
				t.Fatalf("ResourceDeleted() error = %v", err)
			}
			if deleted != tc.want {
				t.Fatalf("ResourceDeleted() = %v, want %v", deleted, tc.want)
			}
		})
	}

	if _, err := db.ExecContext(ctx, `UPDATE agents SET archived_at = now() WHERE uid = $1`, blueprintID); err != nil {
		t.Fatalf("archive blueprint: %v", err)
	}
	deleted, err := store.ResourceDeleted(ctx, BlueprintResource(blueprintID))
	if err != nil || !deleted {
		t.Fatalf("archived blueprint ResourceDeleted() = %v, %v", deleted, err)
	}
}

// The derived roles this ledger now carries are new values in existing columns.
func TestRecordDerivedRolesAgainstPostgres(t *testing.T) {
	db := integrationDB(t)
	accountID, blueprintID := seedOrganizationAccount(t, db)
	store := NewResourceAccessSyncStore(db)
	ctx := context.Background()

	for _, tc := range []struct {
		name     string
		resource ResourceRef
		role     RoleSlug
	}{
		{"account role", AccountResource(accountID), RoleAccountAdmin},
		{"blueprint creator", BlueprintResource(blueprintID), RoleBlueprintAdmin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			intent := AccessIntent{
				AccountID: accountID, OrganizationID: "org_authz", Resource: tc.resource,
				Subject: MembershipAssignmentSubject("om_authz"), SubjectID: "user_authz", DesiredRole: tc.role,
			}
			recorded, changed, err := store.Record(ctx, intent)
			if err != nil {
				t.Fatalf("Record() error = %v", err)
			}
			if !changed || recorded.DesiredRole != tc.role || recorded.DesiredVersion != 1 {
				t.Fatalf("Record() = %+v, changed = %v", recorded, changed)
			}

			// Re-recording the same role is a no-op; the version only moves on change.
			_, changed, err = store.Record(ctx, intent)
			if err != nil || changed {
				t.Fatalf("second Record() changed = %v, err = %v", changed, err)
			}

			pending, err := store.PendingForResource(ctx, "org_authz", tc.resource)
			if err != nil || len(pending) != 1 {
				t.Fatalf("PendingForResource() = %+v, %v", pending, err)
			}
		})
	}
}
