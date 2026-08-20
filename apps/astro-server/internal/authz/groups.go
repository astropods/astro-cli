package authz

import (
	"context"
	"errors"
)

var (
	ErrInvalidPageCursor   = errors.New("invalid page cursor")
	ErrGroupNotFound       = errors.New("group not found")
	ErrGroupExists         = errors.New("group already exists")
	ErrGroupMemberExists   = errors.New("group member already exists")
	ErrGroupMemberNotFound = errors.New("group member not found")
)

type Group struct {
	ID             string
	OrganizationID string
	Name           string
	Description    string
}

type GroupMember struct {
	MembershipID string
	UserID       string
}

type PageRequest struct {
	After string
	Limit int
}

type GroupPage struct {
	Groups     []Group
	NextCursor string
}

type GroupMemberPage struct {
	Members    []GroupMember
	NextCursor string
}

// Groups owns organization group lifecycle; resource roles remain on AccessAssignments.
type Groups interface {
	ListGroups(context.Context, string, PageRequest) (GroupPage, error)
	CreateGroup(context.Context, string, string, string) (Group, error)
	GetGroup(context.Context, string, string) (Group, error)
	UpdateGroup(context.Context, string, string, string, string) (Group, error)
	DeleteGroup(context.Context, string, string) error
	ListGroupMembers(context.Context, string, string, PageRequest) (GroupMemberPage, error)
	AddGroupMember(context.Context, string, string, string) error
	RemoveGroupMember(context.Context, string, string, string) error
}
