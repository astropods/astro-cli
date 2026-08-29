package authz

import (
	"fmt"
	"slices"
)

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
type ResourceRole struct {
	Level   AccessLevel
	Slug    RoleSlug
	Actions []Action
}

// Each rung holds everything the rung below it holds, so the bundles compose.
// They mirror scripts/workos-fga/model.json exactly; see the contract test.
var (
	accountMemberActions = []Action{
		ActionAccountRead,
		ActionMemberRead,
		ActionGroupRead,
		ActionClusterRead,
		ActionVariableRead,
		ActionInsightsReadSummary,
	}
	accountMaintainerActions = append(slices.Clone(accountMemberActions),
		ActionAccountEdit,
		ActionMemberManage,
		ActionGroupManage,
		ActionAppRead,
		ActionAppManage,
		ActionVariableManage,
		ActionDataSourceRead,
		ActionDataSourceManage,
		ActionInsightsReadMembers,
		ActionIntegrationRead,
		ActionIntegrationManage,
		ActionAuditLogRead,
		ActionBillingRead,
	)
	// Admin is the only account role that reaches the work inside the account.
	// It holds every child permission, which is how WorkOS propagates access
	// down the resource tree without a per-resource assignment.
	accountAdminActions = append(slices.Clone(accountMaintainerActions),
		ActionAccountDelete,
		ActionBillingManage,
		ActionBlueprintCreate,
		ActionDeploymentCreate,
		ActionAudienceCreate,
		ActionKnowledgeStoreCreate,
		ActionBlueprintRead,
		ActionBlueprintEdit,
		ActionBlueprintOperate,
		ActionBlueprintDelete,
		ActionBlueprintManageAccess,
		ActionBlueprintTransfer,
		ActionDeploymentRead,
		ActionDeploymentEdit,
		ActionDeploymentOperate,
		ActionDeploymentDelete,
		ActionDeploymentManageAccess,
		ActionAudienceRead,
		ActionAudienceEdit,
		ActionAudienceManageMembers,
		ActionAudienceDelete,
		ActionAudienceManageAccess,
		ActionKnowledgeStoreRead,
		ActionKnowledgeStoreEdit,
		ActionKnowledgeStoreOperate,
		ActionKnowledgeStoreDelete,
		ActionKnowledgeStoreManageAccess,
	)

	blueprintViewerActions     = []Action{ActionBlueprintRead}
	blueprintWriterActions     = append(slices.Clone(blueprintViewerActions), ActionBlueprintEdit)
	blueprintMaintainerActions = append(slices.Clone(blueprintWriterActions), ActionBlueprintOperate)
	blueprintAdminActions      = append(slices.Clone(blueprintMaintainerActions),
		ActionBlueprintDelete,
		ActionBlueprintManageAccess,
		ActionBlueprintTransfer,
	)
)

// resourceRoleCatalog is the registration point for each FGA-managed resource.
// Reconciliation removes a stale direct role only for a slug listed here, so a
// role Astro assigns belongs in this catalog even before its checks land.
var resourceRoleCatalog = map[ResourceType][]ResourceRole{
	ResourceAccount: {
		{Level: AccessLevelMember, Slug: RoleAccountMember, Actions: accountMemberActions},
		{Level: AccessLevelMaintainer, Slug: RoleAccountMaintainer, Actions: accountMaintainerActions},
		{Level: AccessLevelAdmin, Slug: RoleAccountAdmin, Actions: accountAdminActions},
	},
	ResourceBlueprint: {
		{Level: AccessLevelViewer, Slug: RoleBlueprintViewer, Actions: blueprintViewerActions},
		{Level: AccessLevelWriter, Slug: RoleBlueprintWriter, Actions: blueprintWriterActions},
		{Level: AccessLevelMaintainer, Slug: RoleBlueprintMaintainer, Actions: blueprintMaintainerActions},
		{Level: AccessLevelAdmin, Slug: RoleBlueprintAdmin, Actions: blueprintAdminActions},
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
