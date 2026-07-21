package deploymentstore

const (
	StatusPending      = "pending"
	StatusProvisioning = "provisioning"
	// StatusDeploying: the worker finished applying manifests; the deployment
	// controller now owns the transition to active/failed based on observed
	// workload health. The worker never sets active directly.
	StatusDeploying   = "deploying"
	StatusActive      = "active"
	StatusFailed      = "failed"
	StatusUndeploying = "undeploying"
	StatusUndeployed  = "undeployed"
	StatusStopped     = "stopped"
	// StatusSuspended is a billing-initiated scale-to-zero, distinct from a
	// user-initiated StatusStopped so resume restores only what billing stopped.
	StatusSuspended = "suspended"
)
