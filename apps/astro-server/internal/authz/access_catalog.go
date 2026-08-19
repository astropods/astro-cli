package authz

import "fmt"

// AccessLevel is Astro's product-facing name for a built-in resource role.
type AccessLevel string

const (
	AccessLevelViewer  AccessLevel = "viewer"
	AccessLevelBuilder AccessLevel = "builder"
	AccessLevelAdmin   AccessLevel = "admin"
)

// ResourceRole describes one built-in role. Permission checks continue to use
// actions; WorkOS remains authoritative for the configured role bundle.
type ResourceRole struct {
	Level   AccessLevel
	Slug    RoleSlug
	Actions []Action
}

// resourceRoleCatalog is the registration point for each FGA-managed resource.
var resourceRoleCatalog = map[ResourceType][]ResourceRole{
	ResourceDeployment: {
		{Level: AccessLevelViewer, Slug: RoleDeploymentViewer, Actions: []Action{ActionDeploymentRead}},
		{Level: AccessLevelBuilder, Slug: RoleDeploymentBuilder, Actions: []Action{
			ActionDeploymentRead,
			ActionDeploymentEdit,
			ActionDeploymentOperate,
			ActionDeploymentDelete,
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
