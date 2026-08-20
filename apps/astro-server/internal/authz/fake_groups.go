package authz

import (
	"context"
	"errors"
)

type FakeGroups struct {
	ListGroupsFunc        func(context.Context, string, PageRequest) (GroupPage, error)
	CreateGroupFunc       func(context.Context, string, string, string) (Group, error)
	GetGroupFunc          func(context.Context, string, string) (Group, error)
	UpdateGroupFunc       func(context.Context, string, string, string, string) (Group, error)
	DeleteGroupFunc       func(context.Context, string, string) error
	ListGroupMembersFunc  func(context.Context, string, string, PageRequest) (GroupMemberPage, error)
	AddGroupMemberFunc    func(context.Context, string, string, string) error
	RemoveGroupMemberFunc func(context.Context, string, string, string) error
}

func (f *FakeGroups) ListGroups(ctx context.Context, organizationID string, page PageRequest) (GroupPage, error) {
	if f.ListGroupsFunc == nil {
		return GroupPage{}, errors.New("unexpected group list")
	}
	return f.ListGroupsFunc(ctx, organizationID, page)
}

func (f *FakeGroups) CreateGroup(ctx context.Context, organizationID, name, description string) (Group, error) {
	if f.CreateGroupFunc == nil {
		return Group{}, errors.New("unexpected group creation")
	}
	return f.CreateGroupFunc(ctx, organizationID, name, description)
}

func (f *FakeGroups) GetGroup(ctx context.Context, organizationID, groupID string) (Group, error) {
	if f.GetGroupFunc == nil {
		return Group{}, errors.New("unexpected group lookup")
	}
	return f.GetGroupFunc(ctx, organizationID, groupID)
}

func (f *FakeGroups) UpdateGroup(ctx context.Context, organizationID, groupID, name, description string) (Group, error) {
	if f.UpdateGroupFunc == nil {
		return Group{}, errors.New("unexpected group update")
	}
	return f.UpdateGroupFunc(ctx, organizationID, groupID, name, description)
}

func (f *FakeGroups) DeleteGroup(ctx context.Context, organizationID, groupID string) error {
	if f.DeleteGroupFunc == nil {
		return errors.New("unexpected group deletion")
	}
	return f.DeleteGroupFunc(ctx, organizationID, groupID)
}

func (f *FakeGroups) ListGroupMembers(ctx context.Context, organizationID, groupID string, page PageRequest) (GroupMemberPage, error) {
	if f.ListGroupMembersFunc == nil {
		return GroupMemberPage{}, errors.New("unexpected group member list")
	}
	return f.ListGroupMembersFunc(ctx, organizationID, groupID, page)
}

func (f *FakeGroups) AddGroupMember(ctx context.Context, organizationID, groupID, membershipID string) error {
	if f.AddGroupMemberFunc == nil {
		return errors.New("unexpected group member addition")
	}
	return f.AddGroupMemberFunc(ctx, organizationID, groupID, membershipID)
}

func (f *FakeGroups) RemoveGroupMember(ctx context.Context, organizationID, groupID, membershipID string) error {
	if f.RemoveGroupMemberFunc == nil {
		return errors.New("unexpected group member removal")
	}
	return f.RemoveGroupMemberFunc(ctx, organizationID, groupID, membershipID)
}

var _ Groups = (*FakeGroups)(nil)
