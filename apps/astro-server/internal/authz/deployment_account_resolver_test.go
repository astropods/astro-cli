package authz_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

func TestDeploymentAccountResolverEnablesConvergedOrganizationResource(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT d.account_id,")).
		WithArgs("dep_123").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "type", "ready"}).
			AddRow("acct_123", "organization", true))

	resolver := authz.NewDeploymentAccountResolver(db)
	ctx := authz.WithRequestCache(context.Background())
	accountID, personal, resolveErr := resolver.AccountForResource(ctx, authz.DeploymentResource("dep_123"))
	if resolveErr != nil || accountID != "acct_123" || personal {
		t.Fatalf("AccountForResource() = (%q, %v, %v)", accountID, personal, resolveErr)
	}
	enabled, enableErr := resolver.Enabled(ctx, authz.DeploymentResource("dep_123"))
	if enableErr != nil || !enabled {
		t.Fatalf("Enabled() = (%v, %v), want (true, nil)", enabled, enableErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentAccountResolverScopesRollout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		accountType string
		ready       bool
	}{
		{name: "resource not converged", accountType: "organization", ready: false},
		{name: "personal account", accountType: "personal", ready: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			mock.ExpectQuery(regexp.QuoteMeta("SELECT d.account_id,")).
				WithArgs("dep_123").
				WillReturnRows(sqlmock.NewRows([]string{"account_id", "type", "ready"}).
					AddRow("acct_123", test.accountType, test.ready))

			enabled, resolveErr := authz.NewDeploymentAccountResolver(db).
				Enabled(context.Background(), authz.DeploymentResource("dep_123"))
			if resolveErr != nil || enabled {
				t.Fatalf("Enabled() = (%v, %v), want (false, nil)", enabled, resolveErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDeploymentAccountResolverRejectsUnsupportedResource(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	resolver := authz.NewDeploymentAccountResolver(db)
	if _, _, err := resolver.AccountForResource(context.Background(), authz.ResourceRef{Type: "blueprint", ExternalID: "bp_123"}); err == nil {
		t.Fatal("AccountForResource() error = nil, want unsupported resource error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
