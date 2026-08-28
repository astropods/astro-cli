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

func TestDeploymentAccessCatalogRejectsUnknownValues(t *testing.T) {
	t.Parallel()

	if roles := authz.ResourceRoles(authz.ResourceType("blueprint")); roles != nil {
		t.Fatalf("unsupported resource roles = %#v", roles)
	}
	if _, err := authz.RoleForAccessLevel(authz.ResourceDeployment, authz.AccessLevel("operator")); err == nil {
		t.Fatal("RoleForAccessLevel() error = nil")
	}
	if _, ok := authz.AccessLevelForRole(authz.ResourceDeployment, authz.RoleSlug("custom")); ok {
		t.Fatal("AccessLevelForRole() accepted custom role")
	}
}
