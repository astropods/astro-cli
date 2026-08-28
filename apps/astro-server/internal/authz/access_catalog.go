package authz

import "fmt"

// AccessLevel is Astro's product-facing name for a built-in resource role.
type AccessLevel string

const (
	AccessLevelMember     AccessLevel = "member"
	AccessLevelViewer     AccessLevel = "viewer"
	AccessLevelWriter     AccessLevel = "writer"
	AccessLevelMaintainer AccessLevel = "maintainer"
	AccessLevelAdmin      AccessLevel = "admin"
)

// ResourceRole describes one built-in role. Permission checks continue to use
// actions; WorkOS remains authoritative for the configured role bundle.
// Actions is empty for a resource type Astro registers and assigns but does not
// yet check.
type ResourceRole struct {
	Level   AccessLevel
	Slug    RoleSlug
	Actions []Action
}

// resourceRoleCatalog is the registration point for each FGA-managed resource.
// Reconciliation removes a stale direct role only for a slug listed here, so a
// role Astro assigns belongs in this catalog even before its checks land.
var resourceRoleCatalog = map[ResourceType][]ResourceRole{
	ResourceAccount: {
		{Level: AccessLevelMember, Slug: RoleAccountMember},
		{Level: AccessLevelMaintainer, Slug: RoleAccountMaintainer},
		{Level: AccessLevelAdmin, Slug: RoleAccountAdmin},
	},
	ResourceBlueprint: {
		{Level: AccessLevelViewer, Slug: RoleBlueprintViewer},
		{Level: AccessLevelWriter, Slug: RoleBlueprintWriter},
		{Level: AccessLevelMaintainer, Slug: RoleBlueprintMaintainer},
		{Level: AccessLevelAdmin, Slug: RoleBlueprintAdmin},
	},
	ResourceDeployment: {
		{Level: AccessLevelViewer, Slug: RoleDeploymentViewer, Actions: []Action{ActionDeploymentRead}},
		{Level: AccessLevelWriter, Slug: RoleDeploymentWriter, Actions: []Action{
			ActionDeploymentRead,
			ActionDeploymentEdit,
		}},
		{Level: AccessLevelMaintainer, Slug: RoleDeploymentMaintainer, Actions: []Action{
			ActionDeploymentRead,
			ActionDeploymentEdit,
			ActionDeploymentOperate,
		}},
		{Level: AccessLevelAdmin, Slug: RoleDeploymentAdmin, Actions: DeploymentActions()},
	},
}

func ResourceRoles(resourceType ResourceType) []ResourceRole {
	registered, ok := resourceRoleCatalog[resourceType]
	if !ok {
		return nil
	}
	roles := make([]ResourceRole, 0, len(registered))
	for _, role := range registered {
		role.Actions = append([]Action(nil), role.Actions...)
		roles = append(roles, role)
	}
	return roles
}

func RoleForAccessLevel(resourceType ResourceType, level AccessLevel) (RoleSlug, error) {
	for _, role := range resourceRoleCatalog[resourceType] {
		if role.Level == level {
			return role.Slug, nil
		}
	}
	return "", fmt.Errorf("unsupported %s access level %q", resourceType, level)
}

func AccessLevelForRole(resourceType ResourceType, slug RoleSlug) (AccessLevel, bool) {
	for _, role := range resourceRoleCatalog[resourceType] {
		if role.Slug == slug {
			return role.Level, true
		}
	}
	return "", false
}
