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

type organizationResolverFunc func(context.Context, authz.ResourceRef) (string, bool, error)

func (f organizationResolverFunc) OrganizationForResource(ctx context.Context, resource authz.ResourceRef) (string, bool, error) {
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
		nil,
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
		nil,
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
		nil,
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

func TestConditionalResourceGateKeepsGlobalRolloutOff(t *testing.T) {
	t.Parallel()

	nextCalled := false
	gate := authz.NewConditionalResourceGate(false, resourceGateFunc(func(context.Context, authz.ResourceRef) (bool, error) {
		nextCalled = true
		return true, nil
	}))
	enabled, err := gate.Enabled(context.Background(), authz.DeploymentResource("dep_123"))
	if err != nil || enabled || nextCalled {
		t.Fatalf("Enabled() = (%v, %v), nextCalled=%v; want globally disabled", enabled, err, nextCalled)
	}
}

func TestFGACheckerDeniesCrossOrganizationSubjectBeforeWorkOS(t *testing.T) {
	t.Parallel()

	checker := authz.NewFGAChecker(
		&authz.FakeFGA{CheckFunc: func(context.Context, string, authz.Action, authz.ResourceRef) (bool, error) {
			t.Fatal("cross-organization subject reached WorkOS")
			return false, nil
		}},
		enabledResourceGate,
		organizationResolverFunc(func(context.Context, authz.ResourceRef) (string, bool, error) {
			return "org_resource", false, nil
		}),
	)
	allowed, err := checker.Authorize(
		context.Background(),
		authz.Subject{OrgID: "org_session", MembershipID: "om_123"},
		authz.ActionDeploymentRead,
		authz.DeploymentResource("dep_123"),
	)
	if err != nil || allowed {
		t.Fatalf("Authorize() = (%v, %v), want concealed denial", allowed, err)
	}
}

func TestFGACheckerListsEffectivePermissionsAfterTenantValidation(t *testing.T) {
	t.Parallel()

	checker := authz.NewFGAChecker(
		&authz.FakeFGA{ListPermissionsFunc: func(_ context.Context, membershipID string, resource authz.ResourceRef) ([]authz.Action, error) {
			if membershipID != "om_123" || resource != authz.DeploymentResource("dep_123") {
				t.Fatalf("unexpected request: membership=%q resource=%+v", membershipID, resource)
			}
			return []authz.Action{authz.ActionDeploymentRead, authz.ActionDeploymentOperate}, nil
		}},
		enabledResourceGate,
		organizationResolverFunc(func(context.Context, authz.ResourceRef) (string, bool, error) {
			return "org_123", false, nil
		}),
	)

	permissions, err := checker.EffectivePermissions(
		context.Background(),
		authz.Subject{OrgID: "org_123", MembershipID: "om_123"},
		authz.DeploymentResource("dep_123"),
	)
	if err != nil || len(permissions) != 2 {
		t.Fatalf("EffectivePermissions() = (%v, %v)", permissions, err)
	}
}

func TestFGACheckerConcealsCrossOrganizationEffectivePermissions(t *testing.T) {
	t.Parallel()

	checker := authz.NewFGAChecker(
		&authz.FakeFGA{ListPermissionsFunc: func(context.Context, string, authz.ResourceRef) ([]authz.Action, error) {
			t.Fatal("cross-organization subject reached WorkOS")
			return nil, nil
		}},
		enabledResourceGate,
		organizationResolverFunc(func(context.Context, authz.ResourceRef) (string, bool, error) {
			return "org_resource", false, nil
		}),
	)

	_, err := checker.EffectivePermissions(
		context.Background(),
		authz.Subject{OrgID: "org_session", MembershipID: "om_123"},
		authz.DeploymentResource("dep_123"),
	)
	if !errors.Is(err, authz.ErrResourceNotVisible) {
		t.Fatalf("EffectivePermissions() error = %v, want ErrResourceNotVisible", err)
	}
}
