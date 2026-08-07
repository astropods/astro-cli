package authz

import (
	"context"
	"errors"
)

// FakeFGA is a strict programmable fake for downstream resource lifecycle and
// enforcement tests. Unconfigured calls fail instead of silently authorizing.
type FakeFGA struct {
	RegisterResourceFunc   func(context.Context, string, ResourceRef, string) error
	UpdateResourceNameFunc func(context.Context, string, ResourceRef, string) error
	DeleteResourceFunc     func(context.Context, string, ResourceRef) error
	AssignRoleFunc         func(context.Context, AssignmentSubject, RoleSlug, ResourceRef) error
	RemoveRoleFunc         func(context.Context, AssignmentSubject, RoleSlug, ResourceRef) error
	CheckFunc              func(context.Context, string, Action, ResourceRef) (bool, error)
}

var _ FGA = (*FakeFGA)(nil)

func (f *FakeFGA) RegisterResource(ctx context.Context, organizationID string, resource ResourceRef, name string) error {
	if f.RegisterResourceFunc == nil {
		return errors.New("unexpected FGA resource registration")
	}
	return f.RegisterResourceFunc(ctx, organizationID, resource, name)
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

func (f *FakeFGA) Check(ctx context.Context, membershipID string, action Action, resource ResourceRef) (bool, error) {
	if f.CheckFunc == nil {
		return false, errors.New("unexpected FGA permission check")
	}
	return f.CheckFunc(ctx, membershipID, action, resource)
}
