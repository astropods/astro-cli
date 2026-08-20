package authz

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	workos "github.com/workos/workos-go/v10"
)

const (
	workOSPageCursorPrefix = "astro-workos-v1:"
	maxGroupPageSize       = 100
)

func (f *WorkOSFGA) ListGroups(ctx context.Context, organizationID string, page PageRequest) (GroupPage, error) {
	if organizationID == "" {
		return GroupPage{}, errors.New("organization id is required")
	}
	cursor, err := decodeWorkOSPageCursor(page)
	if err != nil {
		return GroupPage{}, err
	}
	params := &workos.GroupsListOrganizationGroupsParams{PaginationParams: workos.PaginationParams{Limit: &page.Limit}}
	if cursor.After != "" {
		params.After = &cursor.After
	}
	groups, next, err := collectWorkOSPage(f.groups.ListOrganizationGroups(ctx, organizationID, params), cursor, page.Limit, astroGroup)
	if err != nil {
		return GroupPage{}, fmt.Errorf("list WorkOS groups: %w", err)
	}
	return GroupPage{Groups: groups, NextCursor: next}, nil
}

func (f *WorkOSFGA) CreateGroup(ctx context.Context, organizationID, name, description string) (Group, error) {
	if organizationID == "" || name == "" {
		return Group{}, errors.New("organization id and group name are required")
	}
	params := &workos.GroupsCreateOrganizationGroupParams{Name: name}
	if description != "" {
		params.Description = &description
	}
	group, err := f.groups.CreateOrganizationGroup(ctx, organizationID, params)
	if err != nil {
		return Group{}, fmt.Errorf("create WorkOS group: %w", classifyAPIError(err, http.StatusConflict, ErrGroupExists))
	}
	return astroGroup(group), nil
}

func (f *WorkOSFGA) GetGroup(ctx context.Context, organizationID, groupID string) (Group, error) {
	if organizationID == "" || groupID == "" {
		return Group{}, errors.New("organization id and group id are required")
	}
	group, err := f.groups.GetOrganizationGroup(ctx, organizationID, groupID)
	if err != nil {
		return Group{}, fmt.Errorf("get WorkOS group: %w", classifyAPIError(err, http.StatusNotFound, ErrGroupNotFound))
	}
	return astroGroup(group), nil
}

func (f *WorkOSFGA) UpdateGroup(ctx context.Context, organizationID, groupID, name, description string) (Group, error) {
	if organizationID == "" || groupID == "" || name == "" {
		return Group{}, errors.New("organization id, group id, and group name are required")
	}
	params := &workos.GroupsUpdateOrganizationGroupParams{Name: &name}
	if description == "" {
		params.NullFields = []string{"description"}
	} else {
		params.Description = &description
	}
	group, err := f.groups.UpdateOrganizationGroup(ctx, organizationID, groupID, params)
	if err != nil {
		err = classifyAPIError(err, http.StatusNotFound, ErrGroupNotFound)
		err = classifyAPIError(err, http.StatusConflict, ErrGroupExists)
		return Group{}, fmt.Errorf("update WorkOS group: %w", err)
	}
	return astroGroup(group), nil
}

func (f *WorkOSFGA) DeleteGroup(ctx context.Context, organizationID, groupID string) error {
	if organizationID == "" || groupID == "" {
		return errors.New("organization id and group id are required")
	}
	if err := f.groups.DeleteOrganizationGroup(ctx, organizationID, groupID); err != nil {
		return fmt.Errorf("delete WorkOS group: %w", classifyAPIError(err, http.StatusNotFound, ErrGroupNotFound))
	}
	return nil
}

func (f *WorkOSFGA) ListGroupMembers(ctx context.Context, organizationID, groupID string, page PageRequest) (GroupMemberPage, error) {
	if organizationID == "" || groupID == "" {
		return GroupMemberPage{}, errors.New("organization id and group id are required")
	}
	cursor, err := decodeWorkOSPageCursor(page)
	if err != nil {
		return GroupMemberPage{}, err
	}
	params := &workos.GroupsListOrganizationMembershipsParams{PaginationParams: workos.PaginationParams{Limit: &page.Limit}}
	if cursor.After != "" {
		params.After = &cursor.After
	}
	members, next, err := collectWorkOSPage(
		f.groups.ListOrganizationMemberships(ctx, organizationID, groupID, params),
		cursor,
		page.Limit,
		func(membership *workos.UserOrganizationMembershipBaseListData) GroupMember {
			return GroupMember{MembershipID: membership.ID, UserID: membership.UserID}
		},
	)
	if err != nil {
		return GroupMemberPage{}, fmt.Errorf("list WorkOS group members: %w", classifyAPIError(err, http.StatusNotFound, ErrGroupNotFound))
	}
	return GroupMemberPage{Members: members, NextCursor: next}, nil
}

func (f *WorkOSFGA) AddGroupMember(ctx context.Context, organizationID, groupID, membershipID string) error {
	if organizationID == "" || groupID == "" || membershipID == "" {
		return errors.New("organization id, group id, and membership id are required")
	}
	_, err := f.groups.CreateOrganizationMembership(ctx, organizationID, groupID, &workos.GroupsCreateOrganizationMembershipParams{
		OrganizationMembershipID: membershipID,
	})
	if err != nil {
		return fmt.Errorf("add WorkOS group member: %w", classifyAPIError(err, http.StatusConflict, ErrGroupMemberExists))
	}
	return nil
}

func (f *WorkOSFGA) RemoveGroupMember(ctx context.Context, organizationID, groupID, membershipID string) error {
	if organizationID == "" || groupID == "" || membershipID == "" {
		return errors.New("organization id, group id, and membership id are required")
	}
	if err := f.groups.DeleteOrganizationMembership(ctx, organizationID, groupID, membershipID); err != nil {
		return fmt.Errorf("remove WorkOS group member: %w", classifyAPIError(err, http.StatusNotFound, ErrGroupMemberNotFound))
	}
	return nil
}

func astroGroup(group *workos.Group) Group {
	if group == nil {
		return Group{}
	}
	description := ""
	if group.Description != nil {
		description = *group.Description
	}
	return Group{ID: group.ID, OrganizationID: group.OrganizationID, Name: group.Name, Description: description}
}

type workOSPageCursor struct {
	After string `json:"after,omitempty"`
	Skip  int    `json:"skip,omitempty"`
	Limit int    `json:"limit"`
}

func decodeWorkOSPageCursor(page PageRequest) (workOSPageCursor, error) {
	if page.Limit < 1 || page.Limit > maxGroupPageSize {
		return workOSPageCursor{}, fmt.Errorf("page limit must be between 1 and %d", maxGroupPageSize)
	}
	if page.After == "" {
		return workOSPageCursor{Limit: page.Limit}, nil
	}
	if !strings.HasPrefix(page.After, workOSPageCursorPrefix) {
		return workOSPageCursor{}, ErrInvalidPageCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(page.After, workOSPageCursorPrefix))
	if err != nil {
		return workOSPageCursor{}, ErrInvalidPageCursor
	}
	var cursor workOSPageCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Skip < 0 || cursor.Skip > page.Limit || cursor.Limit != page.Limit {
		return workOSPageCursor{}, ErrInvalidPageCursor
	}
	return cursor, nil
}

func encodeWorkOSPageCursor(cursor workOSPageCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode page cursor: %w", err)
	}
	return workOSPageCursorPrefix + base64.RawURLEncoding.EncodeToString(data), nil
}

// collectWorkOSPage preserves an in-page offset because the SDK iterator auto-fetches vendor pages.
func collectWorkOSPage[T, U any](iterator *workos.Iterator[T], cursor workOSPageCursor, limit int, convert func(*T) U) ([]U, string, error) {
	items := make([]U, 0, limit)
	pageStart := cursor.After
	consumedOnPage := 0
	started := false
	advance := func() (*T, bool) {
		before := cursorValue(iterator.Cursor())
		if !iterator.Next() {
			return nil, false
		}
		after := cursorValue(iterator.Cursor())
		switch {
		case !started:
			started = true
			consumedOnPage = 1
		case before != after:
			pageStart = before
			consumedOnPage = 1
		default:
			consumedOnPage++
		}
		return iterator.Current(), true
	}

	for range cursor.Skip {
		if _, ok := advance(); !ok {
			if err := iterator.Err(); err != nil {
				return nil, "", err
			}
			return nil, "", ErrInvalidPageCursor
		}
	}
	for len(items) < limit {
		current, ok := advance()
		if !ok {
			return items, "", iterator.Err()
		}
		items = append(items, convert(current))
	}

	resume := workOSPageCursor{After: pageStart, Skip: consumedOnPage, Limit: cursor.Limit}
	if _, ok := advance(); !ok {
		return items, "", iterator.Err()
	}
	next, err := encodeWorkOSPageCursor(resume)
	if err != nil {
		return nil, "", err
	}
	return items, next, nil
}

func cursorValue(cursor *string) string {
	if cursor == nil {
		return ""
	}
	return *cursor
}
