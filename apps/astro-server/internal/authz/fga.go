package authz

import "context"

// RoleSlug values are WorkOS role slugs; changes require matching WorkOS configuration.
type RoleSlug string

const (
	RoleDeploymentReader RoleSlug = "deployment-reader"
	RoleDeploymentEditor RoleSlug = "deployment-editor"
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
}
