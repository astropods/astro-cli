package org

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/workos/workos-go/v6/pkg/organizations"
	"github.com/workos/workos-go/v6/pkg/usermanagement"
	"github.com/workos/workos-go/v6/pkg/workos_errors"
)

// ErrOrganizationNotFound reports that WorkOS no longer has the organization.
var ErrOrganizationNotFound = errors.New("WorkOS organization not found")

// Client wraps WorkOS Organizations + Membership + Invitation SDK calls.
// Keeps auth.WorkOSClient focused on authentication.
type Client struct {
	orgs *organizations.Client
	um   *usermanagement.Client
}

// NewClient creates a new org client using the given WorkOS API key.
func NewClient(apiKey string) *Client {
	return &Client{
		orgs: &organizations.Client{APIKey: apiKey},
		um:   usermanagement.NewClient(apiKey),
	}
}

// --- Organizations ---

// CreateOrganization creates a WorkOS organization with the given name and external ID (Astro account ID).
func (c *Client) CreateOrganization(ctx context.Context, name, externalID string) (Organization, error) {
	org, err := c.orgs.CreateOrganization(ctx, organizations.CreateOrganizationOpts{
		Name:       name,
		ExternalID: externalID,
	})
	if err != nil {
		return Organization{}, fmt.Errorf("workos: create organization: %w", err)
	}
	return Organization{ID: org.ID, Name: org.Name, ExternalID: externalID}, nil
}

// GetOrganization retrieves a WorkOS organization by ID.
func (c *Client) GetOrganization(ctx context.Context, workosOrgID string) (Organization, error) {
	org, err := c.orgs.GetOrganization(ctx, organizations.GetOrganizationOpts{
		Organization: workosOrgID,
	})
	if err != nil {
		return Organization{}, fmt.Errorf("workos: get organization: %w", classifyOrganizationError(err))
	}
	return Organization{ID: org.ID, Name: org.Name}, nil
}

func (c *Client) GetOrganizationByExternalID(ctx context.Context, externalID string) (Organization, error) {
	org, err := c.orgs.GetOrganizationByExternalID(ctx, organizations.GetOrganizationByExternalIDOpts{
		ExternalID: externalID,
	})
	if err != nil {
		return Organization{}, fmt.Errorf("workos: get organization by external id: %w", classifyOrganizationError(err))
	}
	return Organization{ID: org.ID, Name: org.Name, ExternalID: externalID}, nil
}

func classifyOrganizationError(err error) error {
	var httpErr workos_errors.HTTPError
	if errors.As(err, &httpErr) && httpErr.Code == http.StatusNotFound {
		return errors.Join(ErrOrganizationNotFound, err)
	}
	return err
}

// DeleteOrganization deletes a WorkOS organization.
func (c *Client) DeleteOrganization(ctx context.Context, workosOrgID string) error {
	err := c.orgs.DeleteOrganization(ctx, organizations.DeleteOrganizationOpts{
		Organization: workosOrgID,
	})
	if err != nil {
		return fmt.Errorf("workos: delete organization: %w", err)
	}
	return nil
}

// UpdateOrganizationName renames a WorkOS organization.
func (c *Client) UpdateOrganizationName(ctx context.Context, workosOrgID, name string) error {
	_, err := c.orgs.UpdateOrganization(ctx, organizations.UpdateOrganizationOpts{
		Organization: workosOrgID,
		Name:         name,
	})
	if err != nil {
		return fmt.Errorf("workos: update organization name: %w", err)
	}
	return nil
}

// UpdateOrganizationExternalID sets the external_id on a WorkOS organization.
func (c *Client) UpdateOrganizationExternalID(ctx context.Context, workosOrgID, externalID string) error {
	_, err := c.orgs.UpdateOrganization(ctx, organizations.UpdateOrganizationOpts{
		Organization: workosOrgID,
		ExternalID:   externalID,
	})
	if err != nil {
		return fmt.Errorf("workos: update organization external_id: %w", err)
	}
	return nil
}

// --- Memberships ---

// CreateMembership creates a WorkOS organization membership.
func (c *Client) CreateMembership(ctx context.Context, workosOrgID, userID, roleSlug string) (Membership, error) {
	m, err := c.um.CreateOrganizationMembership(ctx, usermanagement.CreateOrganizationMembershipOpts{
		UserID:         userID,
		OrganizationID: workosOrgID,
		RoleSlug:       roleSlug,
	})
	if err != nil {
		return Membership{}, fmt.Errorf("workos: create membership: %w", err)
	}
	return membershipFromWorkOS(m), nil
}

const membershipPageSize = 100

// ListMemberships lists memberships for an organization.
// Includes both active and pending memberships so that invited users
// whose membership hasn't been activated yet are still visible.
func (c *Client) ListMemberships(ctx context.Context, workosOrgID string, opts ListOpts) ([]Membership, error) {
	page, err := c.ListMembershipsPage(ctx, workosOrgID, opts)
	return page.Memberships, err
}

// ListMembershipsPage lists one page of organization memberships with the
// cursor required to continue without truncating large organizations.
func (c *Client) ListMembershipsPage(ctx context.Context, workosOrgID string, opts ListOpts) (MembershipPage, error) {
	resp, err := c.um.ListOrganizationMemberships(ctx, usermanagement.ListOrganizationMembershipsOpts{
		OrganizationID: workosOrgID,
		Statuses:       []usermanagement.OrganizationMembershipStatus{usermanagement.Active, usermanagement.PendingOrganizationMembership},
		Limit:          opts.Limit,
		After:          opts.After,
		Before:         opts.Before,
	})
	if err != nil {
		return MembershipPage{}, fmt.Errorf("workos: list memberships: %w", err)
	}
	result := make([]Membership, 0, len(resp.Data))
	for _, m := range resp.Data {
		result = append(result, membershipFromWorkOS(m))
	}
	return MembershipPage{Memberships: result, NextCursor: resp.ListMetadata.After}, nil
}

func (c *Client) ListAllMemberships(ctx context.Context, workosOrgID string) ([]Membership, error) {
	var all []Membership
	var after string
	for {
		resp, err := c.um.ListOrganizationMemberships(ctx, usermanagement.ListOrganizationMembershipsOpts{
			OrganizationID: workosOrgID,
			Statuses:       []usermanagement.OrganizationMembershipStatus{usermanagement.Active, usermanagement.PendingOrganizationMembership},
			Limit:          membershipPageSize,
			After:          after,
		})
		if err != nil {
			return nil, fmt.Errorf("workos: list memberships: %w", err)
		}
		for _, m := range resp.Data {
			all = append(all, membershipFromWorkOS(m))
		}
		if len(resp.Data) == 0 || resp.ListMetadata.After == "" {
			return all, nil
		}
		after = resp.ListMetadata.After
	}
}

// ListMembershipsForUser lists all organization memberships for a user.
func (c *Client) ListMembershipsForUser(ctx context.Context, userID string) ([]Membership, error) {
	resp, err := c.um.ListOrganizationMemberships(ctx, usermanagement.ListOrganizationMembershipsOpts{
		UserID: userID,
		Limit:  100,
	})
	if err != nil {
		return nil, fmt.Errorf("workos: list memberships for user: %w", err)
	}
	result := make([]Membership, 0, len(resp.Data))
	for _, m := range resp.Data {
		result = append(result, membershipFromWorkOS(m))
	}
	return result, nil
}

// GetMembership retrieves a specific membership by ID.
func (c *Client) GetMembership(ctx context.Context, membershipID string) (Membership, error) {
	m, err := c.um.GetOrganizationMembership(ctx, usermanagement.GetOrganizationMembershipOpts{
		OrganizationMembership: membershipID,
	})
	if err != nil {
		return Membership{}, fmt.Errorf("workos: get membership: %w", err)
	}
	return membershipFromWorkOS(m), nil
}

// UpdateMembershipRole updates the role of a membership.
func (c *Client) UpdateMembershipRole(ctx context.Context, membershipID, roleSlug string) (Membership, error) {
	m, err := c.um.UpdateOrganizationMembership(ctx, membershipID, usermanagement.UpdateOrganizationMembershipOpts{
		RoleSlug: roleSlug,
	})
	if err != nil {
		return Membership{}, fmt.Errorf("workos: update membership role: %w", err)
	}
	return membershipFromWorkOS(m), nil
}

// DeleteMembership deletes a membership.
func (c *Client) DeleteMembership(ctx context.Context, membershipID string) error {
	err := c.um.DeleteOrganizationMembership(ctx, usermanagement.DeleteOrganizationMembershipOpts{
		OrganizationMembership: membershipID,
	})
	if err != nil {
		return fmt.Errorf("workos: delete membership: %w", err)
	}
	return nil
}

// DeactivateMembership deactivates a membership.
func (c *Client) DeactivateMembership(ctx context.Context, membershipID string) (Membership, error) {
	m, err := c.um.DeactivateOrganizationMembership(ctx, usermanagement.DeactivateOrganizationMembershipOpts{
		OrganizationMembership: membershipID,
	})
	if err != nil {
		return Membership{}, fmt.Errorf("workos: deactivate membership: %w", err)
	}
	return membershipFromWorkOS(m), nil
}

// --- Invitations ---

// SendInvitation sends an organization invitation.
func (c *Client) SendInvitation(ctx context.Context, workosOrgID, email, inviterUserID, roleSlug string) (Invitation, error) {
	inv, err := c.um.SendInvitation(ctx, usermanagement.SendInvitationOpts{
		Email:          email,
		OrganizationID: workosOrgID,
		InviterUserID:  inviterUserID,
		RoleSlug:       roleSlug,
	})
	if err != nil {
		return Invitation{}, fmt.Errorf("workos: send invitation: %w", err)
	}
	return invitationFromWorkOS(inv), nil
}

// ListInvitations lists pending invitations for an organization.
func (c *Client) ListInvitations(ctx context.Context, workosOrgID string) ([]Invitation, error) {
	resp, err := c.um.ListInvitations(ctx, usermanagement.ListInvitationsOpts{
		OrganizationID: workosOrgID,
		Limit:          100,
	})
	if err != nil {
		return nil, fmt.Errorf("workos: list invitations: %w", err)
	}
	result := make([]Invitation, 0, len(resp.Data))
	for _, inv := range resp.Data {
		result = append(result, invitationFromWorkOS(inv))
	}
	return result, nil
}

// GetInvitation retrieves an invitation by ID.
func (c *Client) GetInvitation(ctx context.Context, invitationID string) (Invitation, error) {
	inv, err := c.um.GetInvitation(ctx, usermanagement.GetInvitationOpts{
		Invitation: invitationID,
	})
	if err != nil {
		return Invitation{}, fmt.Errorf("workos: get invitation: %w", err)
	}
	return invitationFromWorkOS(inv), nil
}

// RevokeInvitation revokes an invitation.
func (c *Client) RevokeInvitation(ctx context.Context, invitationID string) error {
	_, err := c.um.RevokeInvitation(ctx, usermanagement.RevokeInvitationOpts{
		Invitation: invitationID,
	})
	if err != nil {
		return fmt.Errorf("workos: revoke invitation: %w", err)
	}
	return nil
}

// --- Roles ---

// ListOrganizationRoles lists roles available in an organization.
func (c *Client) ListOrganizationRoles(ctx context.Context, workosOrgID string) ([]Role, error) {
	resp, err := c.orgs.ListOrganizationRoles(ctx, organizations.ListOrganizationRolesOpts{
		OrganizationID: workosOrgID,
	})
	if err != nil {
		return nil, fmt.Errorf("workos: list organization roles: %w", err)
	}
	result := make([]Role, 0, len(resp.Data))
	for _, r := range resp.Data {
		result = append(result, Role{
			ID:          r.ID,
			Name:        r.Name,
			Slug:        r.Slug,
			Description: r.Description,
		})
	}
	return result, nil
}

// --- Helpers ---

func membershipFromWorkOS(m usermanagement.OrganizationMembership) Membership {
	roleSlugs := make([]string, 0, len(m.Roles))
	for _, role := range m.Roles {
		roleSlugs = append(roleSlugs, role.Slug)
	}
	if len(roleSlugs) == 0 && m.Role.Slug != "" {
		roleSlugs = append(roleSlugs, m.Role.Slug)
	}
	return Membership{
		ID:             m.ID,
		UserID:         m.UserID,
		OrganizationID: m.OrganizationID,
		RoleSlug:       m.Role.Slug,
		RoleSlugs:      roleSlugs,
		Status:         string(m.Status),
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

func invitationFromWorkOS(inv usermanagement.Invitation) Invitation {
	return Invitation{
		ID:             inv.ID,
		Email:          inv.Email,
		State:          string(inv.State),
		OrganizationID: inv.OrganizationID,
		InviterUserID:  inv.InviterUserID,
		ExpiresAt:      inv.ExpiresAt,
		CreatedAt:      inv.CreatedAt,
	}
}
