package authz

// Action values are WorkOS permission slugs; changes require matching WorkOS configuration.
type Action string

// Account actions govern the account itself and the surfaces it owns. A
// `<type>:create` action is an account permission because a resource that does
// not exist yet has nothing to hold a role.
const (
	ActionAccountRead          Action = "account:read"
	ActionAccountEdit          Action = "account:edit"
	ActionAccountDelete        Action = "account:delete"
	ActionMemberRead           Action = "member:read"
	ActionMemberManage         Action = "member:manage"
	ActionGroupRead            Action = "group:read"
	ActionGroupManage          Action = "group:manage"
	ActionAppRead              Action = "app:read"
	ActionAppManage            Action = "app:manage"
	ActionVariableRead         Action = "variable:read"
	ActionVariableManage       Action = "variable:manage"
	ActionDataSourceRead       Action = "data_source:read"
	ActionDataSourceManage     Action = "data_source:manage"
	ActionInsightsReadSummary  Action = "insights:read_summary"
	ActionInsightsReadMembers  Action = "insights:read_members"
	ActionBillingRead          Action = "billing:read"
	ActionBillingManage        Action = "billing:manage"
	ActionAuditLogRead         Action = "audit_log:read"
	ActionIntegrationRead      Action = "integration:read"
	ActionIntegrationManage    Action = "integration:manage"
	ActionClusterRead          Action = "cluster:read"
	ActionBlueprintCreate      Action = "blueprint:create"
	ActionDeploymentCreate     Action = "deployment:create"
	ActionAudienceCreate       Action = "audience:create"
	ActionKnowledgeStoreCreate Action = "knowledge_store:create"
)

// Blueprint actions cover the packaged agent, its versions, and its builds.
const (
	ActionBlueprintRead         Action = "blueprint:read"
	ActionBlueprintEdit         Action = "blueprint:edit"
	ActionBlueprintOperate      Action = "blueprint:operate"
	ActionBlueprintDelete       Action = "blueprint:delete"
	ActionBlueprintManageAccess Action = "blueprint:manage_access"
	ActionBlueprintTransfer     Action = "blueprint:transfer"
)

// Audience and knowledge-store actions have no Astro roles yet. They are here
// because account-admin inherits them, so the catalog can state that bundle in
// full rather than in part.
const (
	ActionAudienceRead          Action = "audience:read"
	ActionAudienceEdit          Action = "audience:edit"
	ActionAudienceManageMembers Action = "audience:manage_members"
	ActionAudienceDelete        Action = "audience:delete"
	ActionAudienceManageAccess  Action = "audience:manage_access"

	ActionKnowledgeStoreRead         Action = "knowledge_store:read"
	ActionKnowledgeStoreEdit         Action = "knowledge_store:edit"
	ActionKnowledgeStoreOperate      Action = "knowledge_store:operate"
	ActionKnowledgeStoreDelete       Action = "knowledge_store:delete"
	ActionKnowledgeStoreManageAccess Action = "knowledge_store:manage_access"
)

// Deployment actions cover Astro's control plane. They do not grant access to
// invoke or chat with the running agent.
const (
	ActionDeploymentRead         Action = "deployment:read"
	ActionDeploymentEdit         Action = "deployment:edit"
	ActionDeploymentOperate      Action = "deployment:operate"
	ActionDeploymentDelete       Action = "deployment:delete"
	ActionDeploymentManageAccess Action = "deployment:manage_access"
)

var deploymentActions = [...]Action{
	ActionDeploymentRead,
	ActionDeploymentEdit,
	ActionDeploymentOperate,
	ActionDeploymentDelete,
	ActionDeploymentManageAccess,
}

// DeploymentActions returns a copy of the actions available for deployment
// role composition and effective-capability responses.
func DeploymentActions() []Action {
	actions := make([]Action, len(deploymentActions))
	copy(actions, deploymentActions[:])
	return actions
}
