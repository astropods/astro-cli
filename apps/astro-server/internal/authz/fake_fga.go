package authz

import (
	"context"
	"errors"
)

// FakeFGA is a strict programmable fake for downstream resource lifecycle and
// enforcement tests. Unconfigured calls fail instead of silently authorizing.
type FakeFGA struct {
	RegisterResourceFunc           func(context.Context, string, ResourceRef, string) error
	RegisterResourceWithParentFunc func(context.Context, string, ResourceRef, ResourceRef, string) error
	GetResourceFunc                func(context.Context, string, ResourceRef) (AuthorizationResource, error)
	UpdateResourceNameFunc         func(context.Context, string, ResourceRef, string) error
	DeleteResourceFunc             func(context.Context, string, ResourceRef) error
	AssignRoleFunc                 func(context.Context, AssignmentSubject, RoleSlug, ResourceRef) error
	RemoveRoleFunc                 func(context.Context, AssignmentSubject, RoleSlug, ResourceRef) error
	ListRoleAssignmentsFunc        func(context.Context, string, ResourceRef) ([]RoleAssignment, error)
	ListGroupRoleAssignmentsFunc   func(context.Context, string) ([]RoleAssignment, error)
	ListResourcesFunc              func(context.Context, string, Action, ResourceRef) ([]ResourceRef, error)
	ListMembershipsFunc            func(context.Context, string, ResourceRef, Action) ([]string, error)
	CheckFunc                      func(context.Context, string, Action, ResourceRef) (bool, error)
	ListPermissionsFunc            func(context.Context, string, ResourceRef) ([]Action, error)
}

var _ FGA = (*FakeFGA)(nil)
var _ AccessAssignments = (*FakeFGA)(nil)
var _ ResourceDiscovery = (*FakeFGA)(nil)
var _ ResourceMembershipDiscovery = (*FakeFGA)(nil)
var _ ResourceLifecycle = (*FakeFGA)(nil)

func (f *FakeFGA) RegisterResource(ctx context.Context, organizationID string, resource ResourceRef, name string) error {
	if f.RegisterResourceFunc == nil {
		return errors.New("unexpected FGA resource registration")
	}
	return f.RegisterResourceFunc(ctx, organizationID, resource, name)
}

func (f *FakeFGA) RegisterResourceWithParent(ctx context.Context, organizationID string, resource, parent ResourceRef, name string) error {
	if f.RegisterResourceWithParentFunc == nil {
		return errors.New("unexpected FGA child resource registration")
	}
	return f.RegisterResourceWithParentFunc(ctx, organizationID, resource, parent, name)
}

func (f *FakeFGA) GetResource(ctx context.Context, organizationID string, resource ResourceRef) (AuthorizationResource, error) {
	if f.GetResourceFunc == nil {
		return AuthorizationResource{}, errors.New("unexpected FGA resource read")
	}
	return f.GetResourceFunc(ctx, organizationID, resource)
}

func (f *FakeFGA) UpdateResourceName(ctx context.Context, organizationID string, resource ResourceRef, name string) error {
	if f.UpdateResourceNameFunc == nil {
		return errors.New("unexpected FGA resource name update")
	}
	return f.UpdateResourceNameFunc(ctx, organizationID, resource, name)
}

func (f *FakeFGA) DeleteResource(ctx context.Context, organizationID string, resource ResourceRef) error {
	if f.DeleteResourceFunc == nil {
		return errors.New("unexpected FGA resource deletion")
	}
	return f.DeleteResourceFunc(ctx, organizationID, resource)
}

func (f *FakeFGA) AssignRole(ctx context.Context, subject AssignmentSubject, role RoleSlug, resource ResourceRef) error {
	if f.AssignRoleFunc == nil {
		return errors.New("unexpected FGA role assignment")
	}
	return f.AssignRoleFunc(ctx, subject, role, resource)
}

func (f *FakeFGA) RemoveRole(ctx context.Context, subject AssignmentSubject, role RoleSlug, resource ResourceRef) error {
	if f.RemoveRoleFunc == nil {
		return errors.New("unexpected FGA role removal")
	}
	return f.RemoveRoleFunc(ctx, subject, role, resource)
}

func (f *FakeFGA) ListRoleAssignments(ctx context.Context, organizationID string, resource ResourceRef) ([]RoleAssignment, error) {
	if f.ListRoleAssignmentsFunc == nil {
		return nil, errors.New("unexpected FGA role-assignment list")
	}
	return f.ListRoleAssignmentsFunc(ctx, organizationID, resource)
}

func (f *FakeFGA) ListGroupRoleAssignments(ctx context.Context, groupID string) ([]RoleAssignment, error) {
	if f.ListGroupRoleAssignmentsFunc == nil {
		return nil, errors.New("unexpected FGA group role-assignment list")
	}
	return f.ListGroupRoleAssignmentsFunc(ctx, groupID)
}

func (f *FakeFGA) ListResources(ctx context.Context, membershipID string, action Action, parent ResourceRef) ([]ResourceRef, error) {
	if f.ListResourcesFunc == nil {
		return nil, errors.New("unexpected FGA resource discovery")
	}
	return f.ListResourcesFunc(ctx, membershipID, action, parent)
}

func (f *FakeFGA) ListMemberships(ctx context.Context, organizationID string, resource ResourceRef, action Action) ([]string, error) {
	if f.ListMembershipsFunc == nil {
		return nil, errors.New("unexpected FGA membership discovery")
	}
	return f.ListMembershipsFunc(ctx, organizationID, resource, action)
}

func (f *FakeFGA) Check(ctx context.Context, membershipID string, action Action, resource ResourceRef) (bool, error) {
	if f.CheckFunc == nil {
		return false, errors.New("unexpected FGA permission check")
	}
	return f.CheckFunc(ctx, membershipID, action, resource)
}

func (f *FakeFGA) ListEffectivePermissions(ctx context.Context, membershipID string, resource ResourceRef) ([]Action, error) {
	if f.ListPermissionsFunc == nil {
		return nil, errors.New("unexpected FGA effective-permissions list")
	}
	return f.ListPermissionsFunc(ctx, membershipID, resource)
}
