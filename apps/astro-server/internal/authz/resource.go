package authz

// ResourceType values are WorkOS resource type slugs; changes require matching WorkOS configuration.
type ResourceType string

const (
	ResourceOrganization ResourceType = "organization"
	ResourceAccount      ResourceType = "account"
	ResourceAudience     ResourceType = "audience"
	ResourceBlueprint    ResourceType = "blueprint"
	ResourceDeployment   ResourceType = "deployment"
	ResourceKnowledge    ResourceType = "knowledge_store"
)

// ResourceRef identifies one resource instance. ExternalID is the Astro id
// (e.g. deployments.id) used as the WorkOS FGA external_id later.
type ResourceRef struct {
	Type       ResourceType
	ExternalID string
}

func DeploymentResource(id string) ResourceRef {
	return ResourceRef{Type: ResourceDeployment, ExternalID: id}
}

func AccountResource(id string) ResourceRef {
	return ResourceRef{Type: ResourceAccount, ExternalID: id}
}

func AudienceResource(id string) ResourceRef {
	return ResourceRef{Type: ResourceAudience, ExternalID: id}
}

func BlueprintResource(id string) ResourceRef {
	return ResourceRef{Type: ResourceBlueprint, ExternalID: id}
}

func KnowledgeStoreResource(id string) ResourceRef {
	return ResourceRef{Type: ResourceKnowledge, ExternalID: id}
}

func OrganizationResource(id string) ResourceRef {
	return ResourceRef{Type: ResourceOrganization, ExternalID: id}
}
