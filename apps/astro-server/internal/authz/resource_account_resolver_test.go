package authz_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

func TestResourceAccountResolverResolvesEveryModelledType(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		resource authz.ResourceRef
		from     string
	}{
		{"account", authz.AccountResource("acct_123"), "FROM accounts a"},
		{"blueprint", authz.BlueprintResource("bp_123"), "FROM agents g"},
		{"deployment", authz.DeploymentResource("dep_123"), "FROM deployments d"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close() //nolint:errcheck

			mock.ExpectQuery(regexp.QuoteMeta(tc.from)).
				WithArgs(tc.resource.ExternalID).
				WillReturnRows(sqlmock.NewRows([]string{"account_id", "type", "workos_org_id"}).
					AddRow("acct_123", "organization", "org_123"))

			resolver := authz.NewResourceAccountResolver(db)
			// One request resolves each resource once, so the three calls below
			// share a single query.
			ctx := authz.WithRequestCache(context.Background())

			accountID, personal, resolveErr := resolver.AccountForResource(ctx, tc.resource)
			if resolveErr != nil || accountID != "acct_123" || personal {
				t.Fatalf("AccountForResource() = (%q, %v, %v)", accountID, personal, resolveErr)
			}
			organizationID, personal, resolveErr := resolver.OrganizationForResource(ctx, tc.resource)
			if resolveErr != nil || organizationID != "org_123" || personal {
				t.Fatalf("OrganizationForResource() = (%q, %v, %v)", organizationID, personal, resolveErr)
			}
			enabled, enableErr := resolver.Enabled(ctx, tc.resource)
			if enableErr != nil || !enabled {
				t.Fatalf("Enabled() = (%v, %v), want (true, nil)", enabled, enableErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestResourceAccountResolverScopesRollout(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		accountType string
		wantEnabled bool
	}{
		{name: "organization account", accountType: "organization", wantEnabled: true},
		{name: "personal account", accountType: "personal", wantEnabled: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close() //nolint:errcheck

			mock.ExpectQuery(regexp.QuoteMeta("FROM agents g")).
				WithArgs("bp_123").
				WillReturnRows(sqlmock.NewRows([]string{"account_id", "type", "workos_org_id"}).
					AddRow("acct_123", tc.accountType, "org_123"))

			enabled, resolveErr := authz.NewResourceAccountResolver(db).
				Enabled(context.Background(), authz.BlueprintResource("bp_123"))
			if resolveErr != nil || enabled != tc.wantEnabled {
				t.Fatalf("Enabled() = (%v, %v), want (%v, nil)", enabled, resolveErr, tc.wantEnabled)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// A type with no query reaches no database, so an unmodelled resource cannot
// resolve to some other account's row.
func TestResourceAccountResolverRejectsUnmodelledType(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	resolver := authz.NewResourceAccountResolver(db)
	if _, _, err := resolver.AccountForResource(context.Background(), authz.KnowledgeStoreResource("ks_123")); err == nil {
		t.Fatal("AccountForResource() error = nil, want unsupported resource error")
	}
	if _, _, err := resolver.AccountForResource(context.Background(), authz.BlueprintResource("")); err == nil {
		t.Fatal("AccountForResource() error = nil, want missing external id error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
