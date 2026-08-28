package authz_test

import (
	"reflect"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

func TestDeploymentAccessCatalog(t *testing.T) {
	t.Parallel()

	roles := authz.ResourceRoles(authz.ResourceDeployment)
	want := []authz.ResourceRole{
		{Level: authz.AccessLevelViewer, Slug: authz.RoleDeploymentViewer, Actions: []authz.Action{authz.ActionDeploymentRead}},
		{Level: authz.AccessLevelWriter, Slug: authz.RoleDeploymentWriter, Actions: []authz.Action{
			authz.ActionDeploymentRead, authz.ActionDeploymentEdit,
		}},
		{Level: authz.AccessLevelMaintainer, Slug: authz.RoleDeploymentMaintainer, Actions: []authz.Action{
			authz.ActionDeploymentRead, authz.ActionDeploymentEdit, authz.ActionDeploymentOperate,
		}},
		{Level: authz.AccessLevelAdmin, Slug: authz.RoleDeploymentAdmin, Actions: authz.DeploymentActions()},
	}
	if !reflect.DeepEqual(roles, want) {
		t.Fatalf("roles = %#v", roles)
	}
	roles[0].Actions[0] = authz.ActionDeploymentDelete
	if got := authz.ResourceRoles(authz.ResourceDeployment)[0].Actions[0]; got != authz.ActionDeploymentRead {
		t.Fatalf("catalog mutated through result: %q", got)
	}
}

func TestAccessCatalogRegistersAssignableRoles(t *testing.T) {
	t.Parallel()

	// Reconciliation removes a stale direct role only for a registered slug, so
	// every role Astro assigns resolves both ways.
	assignable := map[authz.ResourceType][]authz.RoleSlug{
		authz.ResourceAccount: {
			authz.RoleAccountMember, authz.RoleAccountMaintainer, authz.RoleAccountAdmin,
		},
		authz.ResourceBlueprint: {
			authz.RoleBlueprintViewer, authz.RoleBlueprintWriter,
			authz.RoleBlueprintMaintainer, authz.RoleBlueprintAdmin,
		},
		authz.ResourceDeployment: {
			authz.RoleDeploymentViewer, authz.RoleDeploymentWriter,
			authz.RoleDeploymentMaintainer, authz.RoleDeploymentAdmin,
		},
	}
	for resourceType, slugs := range assignable {
		for _, slug := range slugs {
			level, ok := authz.AccessLevelForRole(resourceType, slug)
			if !ok {
				t.Fatalf("AccessLevelForRole(%q, %q) not registered", resourceType, slug)
			}
			role, err := authz.RoleForAccessLevel(resourceType, level)
			if err != nil || role != slug {
				t.Fatalf("RoleForAccessLevel(%q, %q) = %q, %v", resourceType, level, role, err)
			}
		}
	}
}

func TestDeploymentAccessCatalogRejectsUnknownValues(t *testing.T) {
	t.Parallel()

	if roles := authz.ResourceRoles(authz.ResourceKnowledge); roles != nil {
		t.Fatalf("unsupported resource roles = %#v", roles)
	}
	if _, err := authz.RoleForAccessLevel(authz.ResourceDeployment, authz.AccessLevel("operator")); err == nil {
		t.Fatal("RoleForAccessLevel() error = nil")
	}
	if _, ok := authz.AccessLevelForRole(authz.ResourceDeployment, authz.RoleSlug("custom")); ok {
		t.Fatal("AccessLevelForRole() accepted custom role")
	}
}
