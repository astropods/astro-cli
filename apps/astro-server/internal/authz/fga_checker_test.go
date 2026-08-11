package authz_test

import (
	"context"
	"errors"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

type resourceGateFunc func(context.Context, authz.ResourceRef) (bool, error)

func (f resourceGateFunc) Enabled(ctx context.Context, resource authz.ResourceRef) (bool, error) {
	return f(ctx, resource)
}

var enabledResourceGate = resourceGateFunc(func(context.Context, authz.ResourceRef) (bool, error) {
	return true, nil
})

func TestFGACheckerOrganizationUsesLiveCheck(t *testing.T) {
	t.Parallel()

	var checks int
	checker := authz.NewFGAChecker(
		&authz.FakeFGA{CheckFunc: func(_ context.Context, membershipID string, action authz.Action, resource authz.ResourceRef) (bool, error) {
			checks++
			if membershipID != "om_123" || action != authz.ActionDeploymentRead || resource != authz.DeploymentResource("dep_123") {
				t.Fatalf("unexpected check: membership=%q action=%q resource=%+v", membershipID, action, resource)
			}
			return true, nil
		}},
		enabledResourceGate,
	)

	allowed, err := checker.Authorize(
		context.Background(),
		authz.Subject{UserID: "user_123", MembershipID: "om_123"},
		authz.ActionDeploymentRead,
		authz.DeploymentResource("dep_123"),
	)
	if err != nil || !allowed || checks != 1 {
		t.Fatalf("Authorize() = (%v, %v), checks=%d; want (true, nil), checks=1", allowed, err, checks)
	}
}

func TestFGACheckerEmptyMembershipIsUnavailable(t *testing.T) {
	t.Parallel()

	checker := authz.NewFGAChecker(
		&authz.FakeFGA{},
		enabledResourceGate,
	)

	allowed, err := checker.Authorize(
		context.Background(),
		authz.Subject{UserID: "user_123"},
		authz.ActionDeploymentEdit,
		authz.DeploymentResource("dep_123"),
	)
	if allowed || !errors.Is(err, authz.ErrWorkOSMembershipUnavailable) {
		t.Fatalf("Authorize() = (%v, %v), want (false, ErrWorkOSMembershipUnavailable)", allowed, err)
	}
}

func TestFGACheckerSkipsResourcesOutsideRollout(t *testing.T) {
	t.Parallel()

	checker := authz.NewFGAChecker(
		&authz.FakeFGA{CheckFunc: func(context.Context, string, authz.Action, authz.ResourceRef) (bool, error) {
			t.Fatal("WorkOS check should not run outside the rollout")
			return false, nil
		}},
		resourceGateFunc(func(context.Context, authz.ResourceRef) (bool, error) { return false, nil }),
	)

	allowed, err := checker.Authorize(
		context.Background(),
		authz.Subject{MembershipID: "om_123"},
		authz.ActionDeploymentRead,
		authz.DeploymentResource("dep_123"),
	)
	if allowed || !errors.Is(err, authz.ErrFGAResourceNotEnabled) {
		t.Fatalf("Authorize() = (%v, %v), want (false, ErrFGAResourceNotEnabled)", allowed, err)
	}
}
