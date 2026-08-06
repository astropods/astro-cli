package authz

// ResourceType values are WorkOS resource type slugs; changes require matching WorkOS configuration.
type ResourceType string

const ResourceDeployment ResourceType = "deployment"

// ResourceRef identifies one resource instance. ExternalID is the Astro id
// (e.g. deployments.id) used as the WorkOS FGA external_id later.
type ResourceRef struct {
	Type       ResourceType
	ExternalID string
}

func DeploymentResource(id string) ResourceRef {
	return ResourceRef{Type: ResourceDeployment, ExternalID: id}
}
