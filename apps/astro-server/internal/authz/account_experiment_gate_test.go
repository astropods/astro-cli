package authz_test

import (
	"context"
	"errors"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

type experimentGateFunc func(context.Context, string) (bool, error)

func (f experimentGateFunc) Enabled(ctx context.Context, accountID string) (bool, error) {
	return f(ctx, accountID)
}

type accountResolverFunc func(context.Context, authz.ResourceRef) (string, bool, error)

func (f accountResolverFunc) AccountForResource(ctx context.Context, resource authz.ResourceRef) (string, bool, error) {
	return f(ctx, resource)
}

func TestAccountExperimentResourceGateRequiresReadyOptedInOrganization(t *testing.T) {
	t.Parallel()

	resource := authz.DeploymentResource("dep_123")
	tests := []struct {
		name       string
		ready      bool
		personal   bool
		optedIn    bool
		want       bool
		wantChecks int
	}{
		{name: "ready organization opted in", ready: true, optedIn: true, want: true, wantChecks: 1},
		{name: "organization opted out", ready: true, wantChecks: 1},
		{name: "personal account", ready: true, personal: true},
		{name: "resource not synchronized"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checks := 0
			gate := authz.NewAccountExperimentResourceGate(
				resourceGateFunc(func(context.Context, authz.ResourceRef) (bool, error) { return test.ready, nil }),
				accountResolverFunc(func(context.Context, authz.ResourceRef) (string, bool, error) {
					return "acct_123", test.personal, nil
				}),
				experimentGateFunc(func(_ context.Context, accountID string) (bool, error) {
					checks++
					if accountID != "acct_123" {
						t.Fatalf("account id = %q", accountID)
					}
					return test.optedIn, nil
				}),
			)

			got, err := gate.Enabled(context.Background(), resource)
			if err != nil || got != test.want || checks != test.wantChecks {
				t.Fatalf("Enabled() = (%v, %v), checks=%d; want (%v, nil), checks=%d", got, err, checks, test.want, test.wantChecks)
			}
		})
	}
}

func TestAccountExperimentResourceGatePropagatesExperimentFailure(t *testing.T) {
	t.Parallel()

	gate := authz.NewAccountExperimentResourceGate(
		resourceGateFunc(func(context.Context, authz.ResourceRef) (bool, error) { return true, nil }),
		accountResolverFunc(func(context.Context, authz.ResourceRef) (string, bool, error) { return "acct_123", false, nil }),
		experimentGateFunc(func(context.Context, string) (bool, error) { return false, errors.New("db unavailable") }),
	)

	if _, err := gate.Enabled(context.Background(), authz.DeploymentResource("dep_123")); err == nil {
		t.Fatal("Enabled() error = nil")
	}
}
