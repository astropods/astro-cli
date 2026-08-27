package authz

import "context"

// RoleSlug values are WorkOS role slugs; changes require matching WorkOS configuration.
type RoleSlug string

const (
	RoleDeploymentViewer  RoleSlug = "deployment-viewer"
	RoleDeploymentBuilder RoleSlug = "deployment-builder"
	RoleDeploymentAdmin   RoleSlug = "deployment-admin"
)

// AssignmentSubjectType identifies who receives a resource-scoped role.
type AssignmentSubjectType string

const (
	AssignmentSubjectMembership AssignmentSubjectType = "organization_membership"
	AssignmentSubjectGroup      AssignmentSubjectType = "group"
)

// AssignmentSubject is either one organization membership or a WorkOS group.
type AssignmentSubject struct {
	Type AssignmentSubjectType
	ID   string
}

// AssignmentSource identifies whether access is direct or group-derived.
type AssignmentSource string

const (
	AssignmentSourceDirect AssignmentSource = "direct"
	AssignmentSourceGroup  AssignmentSource = "group"
)

type RoleAssignment struct {
	ID                    string
	Subject               AssignmentSubject
	Role                  RoleSlug
	Source                AssignmentSource
	GroupRoleAssignmentID string
	Resource              ResourceRef
}

func MembershipAssignmentSubject(id string) AssignmentSubject {
	return AssignmentSubject{Type: AssignmentSubjectMembership, ID: id}
}

func GroupAssignmentSubject(id string) AssignmentSubject {
	return AssignmentSubject{Type: AssignmentSubjectGroup, ID: id}
}

// FGA is the small authorization capability Astro needs from WorkOS. The SDK
// owns transport, retries, pagination, and vendor response models.
type FGA interface {
	RegisterResource(ctx context.Context, organizationID string, resource ResourceRef, name string) error
	UpdateResourceName(ctx context.Context, organizationID string, resource ResourceRef, name string) error
	DeleteResource(ctx context.Context, organizationID string, resource ResourceRef) error
	AssignRole(ctx context.Context, subject AssignmentSubject, role RoleSlug, resource ResourceRef) error
	RemoveRole(ctx context.Context, subject AssignmentSubject, role RoleSlug, resource ResourceRef) error
	Check(ctx context.Context, membershipID string, action Action, resource ResourceRef) (bool, error)
	ListEffectivePermissions(ctx context.Context, membershipID string, resource ResourceRef) ([]Action, error)
}

// ResourceRegistrar projects newly created Astro resources into WorkOS.
type ResourceRegistrar interface {
	RegisterResourceWithParent(ctx context.Context, organizationID string, resource, parent ResourceRef, name string) error
}

// AccessAssignments is the resource-sharing slice of WorkOS authorization.
type AccessAssignments interface {
	AssignRole(ctx context.Context, subject AssignmentSubject, role RoleSlug, resource ResourceRef) error
	RemoveRole(ctx context.Context, subject AssignmentSubject, role RoleSlug, resource ResourceRef) error
	ListRoleAssignments(ctx context.Context, organizationID string, resource ResourceRef) ([]RoleAssignment, error)
	ListGroupRoleAssignments(ctx context.Context, groupID string) ([]RoleAssignment, error)
}

// ResourceDiscovery lists the resources on which one membership has a
// permission. It powers list filtering without per-resource checks.
type ResourceDiscovery interface {
	ListResources(ctx context.Context, membershipID string, action Action, parent ResourceRef) ([]ResourceRef, error)
}

// ResourceMembershipDiscovery lists memberships with one effective permission
// on a resource, including inherited access.
type ResourceMembershipDiscovery interface {
	ListMemberships(ctx context.Context, organizationID string, resource ResourceRef, action Action) ([]string, error)
}

// AuthorizationResource is the vendor resource identity Queen needs for
// inventory and controlled cleanup.
type AuthorizationResource struct {
	ID               string
	OrganizationID   string
	ParentResourceID string
	Resource         ResourceRef
	Name             string
	CreatedAt        string
}

// AuthorizationResourceCatalog exposes resource administration that is not
// part of request-time authorization.
type AuthorizationResourceCatalog interface {
	ListAuthorizationResourcesForOrganization(ctx context.Context, organizationID string) ([]AuthorizationResource, error)
	DeleteAuthorizationResource(ctx context.Context, resourceID string) error
}
