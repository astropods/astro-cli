package authz

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	workos "github.com/workos/workos-go/v10"
)

// WorkOSFGA delegates Astro's small FGA contract to the official WorkOS SDK.
type WorkOSFGA struct {
	authorization *workos.AuthorizationService
}

var (
	// ErrResourceExists means WorkOS already has the resource being registered.
	ErrResourceExists = errors.New("resource already exists")
	// ErrResourceNotFound means WorkOS does not have the resource being deleted.
	ErrResourceNotFound = errors.New("resource not found")
	// ErrRoleAssignmentExists means WorkOS already has the requested role assignment.
	ErrRoleAssignmentExists = errors.New("role assignment already exists")
)

var _ FGA = (*WorkOSFGA)(nil)
var _ AccessAssignments = (*WorkOSFGA)(nil)
var _ ResourceDiscovery = (*WorkOSFGA)(nil)

// NewWorkOSFGA creates the process-wide client from cfg.Auth.WorkOSAPIKey.
// Server wiring should construct this once and share it with consumers.
func NewWorkOSFGA(apiKey string) *WorkOSFGA {
	return newWorkOSFGA(workos.NewClient(apiKey))
}

func newWorkOSFGA(client *workos.Client) *WorkOSFGA {
	return &WorkOSFGA{authorization: client.Authorization()}
}

func (f *WorkOSFGA) RegisterResource(ctx context.Context, organizationID string, resource ResourceRef, name string) error {
	if organizationID == "" {
		return errors.New("organization id is required")
	}
	if name == "" {
		return errors.New("resource name is required")
	}
	if err := validateResource(resource); err != nil {
		return err
	}

	_, err := f.authorization.CreateResource(ctx, &workos.AuthorizationCreateResourceParams{
		OrganizationID:   organizationID,
		ResourceTypeSlug: string(resource.Type),
		ExternalID:       resource.ExternalID,
		Name:             name,
	})
	if err != nil {
		return fmt.Errorf("register WorkOS resource %s:%s: %w", resource.Type, resource.ExternalID, classifyAPIError(err, http.StatusConflict, ErrResourceExists))
	}
	return nil
}

func (f *WorkOSFGA) UpdateResourceName(ctx context.Context, organizationID string, resource ResourceRef, name string) error {
	if organizationID == "" {
		return errors.New("organization id is required")
	}
	if name == "" {
		return errors.New("resource name is required")
	}
	if err := validateResource(resource); err != nil {
		return err
	}

	_, err := f.authorization.UpdateResourceByExternalID(
		ctx,
		organizationID,
		string(resource.Type),
		resource.ExternalID,
		&workos.AuthorizationUpdateResourceByExternalIDParams{Name: &name},
	)
	if err != nil {
		return fmt.Errorf("update WorkOS resource %s:%s name: %w", resource.Type, resource.ExternalID, err)
	}
	return nil
}

func (f *WorkOSFGA) DeleteResource(ctx context.Context, organizationID string, resource ResourceRef) error {
	if organizationID == "" {
		return errors.New("organization id is required")
	}
	if err := validateResource(resource); err != nil {
		return err
	}

	cascade := true
	if err := f.authorization.DeleteResourceByExternalID(
		ctx,
		organizationID,
		string(resource.Type),
		resource.ExternalID,
		&workos.AuthorizationDeleteResourceByExternalIDParams{CascadeDelete: &cascade},
	); err != nil {
		return fmt.Errorf("delete WorkOS resource %s:%s: %w", resource.Type, resource.ExternalID, classifyAPIError(err, http.StatusNotFound, ErrResourceNotFound))
	}
	return nil
}

func (f *WorkOSFGA) AssignRole(ctx context.Context, subject AssignmentSubject, role RoleSlug, resource ResourceRef) error {
	if err := validateAssignment(subject, role, resource); err != nil {
		return err
	}

	switch subject.Type {
	case AssignmentSubjectMembership:
		_, err := f.authorization.AssignRole(ctx, subject.ID, &workos.AuthorizationAssignRoleParams{
			RoleSlug:       string(role),
			ResourceTarget: workOSResourceTarget(resource),
		})
		if err != nil {
			return fmt.Errorf("assign WorkOS role %q to membership on %s:%s: %w", role, resource.Type, resource.ExternalID, classifyAPIError(err, http.StatusConflict, ErrRoleAssignmentExists))
		}
	case AssignmentSubjectGroup:
		externalID := resource.ExternalID
		resourceType := string(resource.Type)
		_, err := f.authorization.CreateGroupRoleAssignment(ctx, subject.ID, &workos.AuthorizationCreateGroupRoleAssignmentParams{
			RoleSlug:           string(role),
			ResourceExternalID: &externalID,
			ResourceTypeSlug:   &resourceType,
		})
		if err != nil {
			return fmt.Errorf("assign WorkOS role %q to group on %s:%s: %w", role, resource.Type, resource.ExternalID, classifyAPIError(err, http.StatusConflict, ErrRoleAssignmentExists))
		}
	default:
		return fmt.Errorf("unsupported assignment subject type %q", subject.Type)
	}
	return nil
}

func (f *WorkOSFGA) RemoveRole(ctx context.Context, subject AssignmentSubject, role RoleSlug, resource ResourceRef) error {
	if err := validateAssignment(subject, role, resource); err != nil {
		return err
	}

	switch subject.Type {
	case AssignmentSubjectMembership:
		if err := f.authorization.RemoveRole(ctx, subject.ID, &workos.AuthorizationRemoveRoleParams{
			RoleSlug:       string(role),
			ResourceTarget: workOSResourceTarget(resource),
		}); err != nil {
			return fmt.Errorf("remove WorkOS role %q from membership on %s:%s: %w", role, resource.Type, resource.ExternalID, err)
		}
	case AssignmentSubjectGroup:
		externalID := resource.ExternalID
		resourceType := string(resource.Type)
		if err := f.authorization.DeleteGroupRoleAssignments(ctx, subject.ID, &workos.AuthorizationDeleteGroupRoleAssignmentsParams{
			RoleSlug:           string(role),
			ResourceExternalID: &externalID,
			ResourceTypeSlug:   &resourceType,
		}); err != nil {
			return fmt.Errorf("remove WorkOS role %q from group on %s:%s: %w", role, resource.Type, resource.ExternalID, err)
		}
	default:
		return fmt.Errorf("unsupported assignment subject type %q", subject.Type)
	}
	return nil
}

func (f *WorkOSFGA) ListRoleAssignments(ctx context.Context, organizationID string, resource ResourceRef) ([]RoleAssignment, error) {
	if organizationID == "" {
		return nil, errors.New("organization id is required")
	}
	if err := validateResource(resource); err != nil {
		return nil, err
	}

	iterator := f.authorization.ListRoleAssignmentsForResourceByExternalID(
		ctx,
		organizationID,
		string(resource.Type),
		resource.ExternalID,
		&workos.AuthorizationListRoleAssignmentsForResourceByExternalIDParams{},
	)
	assignments := make([]RoleAssignment, 0)
	for iterator.Next() {
		current := iterator.Current()
		if current.OrganizationMembershipID == "" || current.Role == nil || current.Source == nil || current.Resource == nil {
			return nil, fmt.Errorf("list WorkOS role assignments on %s:%s: assignment %q is missing organization membership, role, source, or resource", resource.Type, resource.ExternalID, current.ID)
		}
		assignment := RoleAssignment{
			ID:       current.ID,
			Subject:  MembershipAssignmentSubject(current.OrganizationMembershipID),
			Role:     RoleSlug(current.Role.Slug),
			Source:   string(current.Source.Type),
			Resource: resource,
		}
		if current.Source.GroupRoleAssignmentID != nil {
			assignment.GroupRoleAssignmentID = *current.Source.GroupRoleAssignmentID
		}
		assignments = append(assignments, assignment)
	}
	if err := iterator.Err(); err != nil {
		return nil, fmt.Errorf("list WorkOS role assignments on %s:%s: %w", resource.Type, resource.ExternalID, err)
	}
	return assignments, nil
}

func (f *WorkOSFGA) ListGroupRoleAssignments(ctx context.Context, groupID string) ([]RoleAssignment, error) {
	if groupID == "" {
		return nil, errors.New("group id is required")
	}
	iterator := f.authorization.ListGroupRoleAssignments(ctx, groupID, &workos.AuthorizationListGroupRoleAssignmentsParams{})
	assignments := make([]RoleAssignment, 0)
	for iterator.Next() {
		current := iterator.Current()
		if current.Role == nil || current.Resource == nil {
			return nil, fmt.Errorf("list WorkOS group role assignments: assignment %q is missing role or resource", current.ID)
		}
		assignments = append(assignments, RoleAssignment{
			ID:       current.ID,
			Subject:  GroupAssignmentSubject(current.GroupID),
			Role:     RoleSlug(current.Role.Slug),
			Source:   "direct",
			Resource: ResourceRef{Type: ResourceType(current.Resource.ResourceTypeSlug), ExternalID: current.Resource.ExternalID},
		})
	}
	if err := iterator.Err(); err != nil {
		return nil, fmt.Errorf("list WorkOS group role assignments: %w", err)
	}
	return assignments, nil
}

func (f *WorkOSFGA) ListResources(ctx context.Context, membershipID string, action Action, parent ResourceRef) ([]ResourceRef, error) {
	if membershipID == "" {
		return nil, errors.New("membership id is required")
	}
	if action == "" {
		return nil, errors.New("action is required")
	}
	if err := validateResource(parent); err != nil {
		return nil, fmt.Errorf("parent %w", err)
	}
	iterator := f.authorization.ListResourcesForMembership(
		ctx,
		membershipID,
		&workos.AuthorizationListResourcesForMembershipParams{
			PermissionSlug: string(action),
			ParentResource: workos.AuthorizationParentResourceByExternalID{
				TypeSlug: string(parent.Type), ExternalID: parent.ExternalID,
			},
		},
	)
	resources := make([]ResourceRef, 0)
	for iterator.Next() {
		current := iterator.Current()
		resources = append(resources, ResourceRef{Type: ResourceType(current.ResourceTypeSlug), ExternalID: current.ExternalID})
	}
	if err := iterator.Err(); err != nil {
		return nil, fmt.Errorf("list WorkOS resources with permission %q: %w", action, err)
	}
	return resources, nil
}

func (f *WorkOSFGA) Check(ctx context.Context, membershipID string, action Action, resource ResourceRef) (bool, error) {
	if membershipID == "" {
		return false, errors.New("membership id is required")
	}
	if action == "" {
		return false, errors.New("action is required")
	}
	if err := validateResource(resource); err != nil {
		return false, err
	}

	result, err := f.authorization.Check(ctx, membershipID, &workos.AuthorizationCheckParams{
		PermissionSlug: string(action),
		ResourceTarget: workOSResourceTarget(resource),
	})
	if err != nil {
		return false, fmt.Errorf("check WorkOS permission %q on %s:%s: %w", action, resource.Type, resource.ExternalID, err)
	}
	return result.Authorized, nil
}

func (f *WorkOSFGA) ListEffectivePermissions(ctx context.Context, membershipID string, resource ResourceRef) ([]Action, error) {
	if membershipID == "" {
		return nil, errors.New("membership id is required")
	}
	if err := validateResource(resource); err != nil {
		return nil, err
	}

	iterator := f.authorization.ListEffectivePermissionsByExternalID(
		ctx,
		membershipID,
		string(resource.Type),
		resource.ExternalID,
		&workos.AuthorizationListEffectivePermissionsByExternalIDParams{},
	)
	permissions := make([]Action, 0)
	for iterator.Next() {
		permissions = append(permissions, Action(iterator.Current().Slug))
	}
	if err := iterator.Err(); err != nil {
		return nil, fmt.Errorf("list WorkOS effective permissions on %s:%s: %w", resource.Type, resource.ExternalID, err)
	}
	return permissions, nil
}

func validateResource(resource ResourceRef) error {
	if resource.Type == "" || resource.ExternalID == "" {
		return errors.New("resource type and external id are required")
	}
	return nil
}

func validateAssignment(subject AssignmentSubject, role RoleSlug, resource ResourceRef) error {
	if subject.ID == "" {
		return errors.New("assignment subject id is required")
	}
	if subject.Type != AssignmentSubjectMembership && subject.Type != AssignmentSubjectGroup {
		return fmt.Errorf("unsupported assignment subject type %q", subject.Type)
	}
	if role == "" {
		return errors.New("role is required")
	}
	return validateResource(resource)
}

func workOSResourceTarget(resource ResourceRef) workos.AuthorizationResourceTarget {
	return workos.AuthorizationResourceTargetByExternalID{
		ResourceExternalID: resource.ExternalID,
		ResourceTypeSlug:   string(resource.Type),
	}
}

type classifiedAPIError struct {
	kind  error
	cause error
}

func (e *classifiedAPIError) Error() string {
	return e.cause.Error()
}

func (e *classifiedAPIError) Unwrap() error {
	return e.cause
}

func (e *classifiedAPIError) Is(target error) bool {
	return target == e.kind
}

func classifyAPIError(err error, statusCode int, kind error) error {
	var apiErr *workos.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == statusCode {
		return &classifiedAPIError{kind: kind, cause: err}
	}
	return err
}
