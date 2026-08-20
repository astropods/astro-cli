package authz

// Action values are WorkOS permission slugs; changes require matching WorkOS configuration.
type Action string

const (
	// Deployment actions cover Astro's control plane. They do not grant access to
	// invoke or chat with the running agent.
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
