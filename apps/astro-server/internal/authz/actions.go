package authz

// Action is a verb the caller wants to perform on a resource.
type Action string

const (
	// ActionDeploymentManage covers all mutating operations on a single deployment
	// (redeploy, restart, stop, rollback, wakeup, undeploy, ingestion trigger, rename, avatar).
	ActionDeploymentManage Action = "deployment:manage"
)
