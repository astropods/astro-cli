//go:build integration

package authz

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

type resolvedFixture struct {
	accountID      string
	organizationID string
	blueprintID    string
	deploymentID   string
}

func seedResolverFixture(t *testing.T, db *sql.DB, accountType string) resolvedFixture {
	t.Helper()
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	fixture := resolvedFixture{organizationID: "org_resolver_" + accountType}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO accounts (id, type, name, owner_user_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, 'resolver-' || substr(gen_random_uuid()::text, 1, 8), 'user_resolver', now(), now())
		RETURNING id::text
	`, accountType).Scan(&fixture.accountID); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO account_members (account_id, user_id, created_at) VALUES ($1::uuid, 'user_resolver', now())
	`, fixture.accountID); err != nil {
		t.Fatalf("insert member: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO account_organizations (account_id, workos_org_id) VALUES ($1::uuid, $2)
	`, fixture.accountID, fixture.organizationID); err != nil {
		t.Fatalf("link organization: %v", err)
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO agents (account_id, name, registry, created_at, updated_at)
		VALUES ($1::uuid, 'resolver-blueprint', 'ecr', now(), now())
		RETURNING uid::text
	`, fixture.accountID).Scan(&fixture.blueprintID); err != nil {
		t.Fatalf("insert blueprint: %v", err)
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO deployments (id, account_id, source_account_id, agent_name, build_id, namespace, deployment_spec_json, status)
		VALUES (substr(md5(gen_random_uuid()::text), 1, 11), $1::uuid, $1::uuid, 'resolver-blueprint', 'build-1', 'ns', '{}', 'active')
		RETURNING id
	`, fixture.accountID).Scan(&fixture.deploymentID); err != nil {
		t.Fatalf("insert deployment: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM accounts WHERE id = $1::uuid`, fixture.accountID) })
	return fixture
}

func resolverDB(t *testing.T) *sql.DB {
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

// Each type reaches its owning account through a different join, so the three
// queries are exercised against the real schema.
func TestResolveEveryTypeAgainstPostgres(t *testing.T) {
	db := resolverDB(t)
	fixture := seedResolverFixture(t, db, "organization")
	resolver := NewResourceAccountResolver(db)
	ctx := context.Background()

	for _, tc := range []struct {
		name     string
		resource ResourceRef
	}{
		{"account", AccountResource(fixture.accountID)},
		{"blueprint", BlueprintResource(fixture.blueprintID)},
		{"deployment", DeploymentResource(fixture.deploymentID)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accountID, personal, err := resolver.AccountForResource(ctx, tc.resource)
			if err != nil || accountID != fixture.accountID || personal {
				t.Fatalf("AccountForResource() = (%q, %v, %v), want %q", accountID, personal, err, fixture.accountID)
			}
			organizationID, _, err := resolver.OrganizationForResource(ctx, tc.resource)
			if err != nil || organizationID != fixture.organizationID {
				t.Fatalf("OrganizationForResource() = (%q, %v), want %q", organizationID, err, fixture.organizationID)
			}
			enabled, err := resolver.Enabled(ctx, tc.resource)
			if err != nil || !enabled {
				t.Fatalf("Enabled() = (%v, %v), want (true, nil)", enabled, err)
			}
		})
	}
}

func TestResolvePersonalAccountStaysOutOfRollout(t *testing.T) {
	db := resolverDB(t)
	fixture := seedResolverFixture(t, db, "personal")
	resolver := NewResourceAccountResolver(db)

	for _, resource := range []ResourceRef{
		AccountResource(fixture.accountID),
		BlueprintResource(fixture.blueprintID),
		DeploymentResource(fixture.deploymentID),
	} {
		enabled, err := resolver.Enabled(context.Background(), resource)
		if err != nil || enabled {
			t.Fatalf("Enabled(%s) = (%v, %v), want (false, nil)", resource.Type, enabled, err)
		}
	}
}

// A soft-deleted account takes its resources out of authorization entirely.
func TestResolveSoftDeletedAccountReturnsNoRows(t *testing.T) {
	db := resolverDB(t)
	fixture := seedResolverFixture(t, db, "organization")
	if _, err := db.Exec(`UPDATE accounts SET deleted_at = now() WHERE id = $1::uuid`, fixture.accountID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	resolver := NewResourceAccountResolver(db)

	for _, resource := range []ResourceRef{
		AccountResource(fixture.accountID),
		BlueprintResource(fixture.blueprintID),
		DeploymentResource(fixture.deploymentID),
	} {
		if _, _, err := resolver.AccountForResource(context.Background(), resource); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("AccountForResource(%s) error = %v, want sql.ErrNoRows", resource.Type, err)
		}
	}
}
