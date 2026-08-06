package authz

// Action values are WorkOS permission slugs; changes require matching WorkOS configuration.
type Action string

const (
	// Deployment actions cover Astro's control plane. They do not grant access to
	// invoke or chat with the running agent.
	ActionDeploymentView Action = "deployment:view"
	ActionDeploymentEdit Action = "deployment:edit"
)
