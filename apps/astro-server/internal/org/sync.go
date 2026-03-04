package org

import (
	"context"
	"fmt"

	"github.com/postman/astro/apps/astro-server/internal/account"
)

// Sync provides write-path sync logic (Astro → WorkOS) for organization memberships.
type Sync struct {
	client       *Client
	accountStore *account.AccountStore
}

// NewSync creates a new Sync instance.
func NewSync(client *Client, accountStore *account.AccountStore) *Sync {
	return &Sync{client: client, accountStore: accountStore}
}

// AddMember adds a member to an org account, writing to WorkOS first then locally.
func (s *Sync) AddMember(ctx context.Context, accountID, userID, role string) (*account.AccountMember, error) {
	acct, err := s.accountStore.GetByID(accountID)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}
	if acct.Type == "personal" {
		return nil, fmt.Errorf("cannot add members to personal accounts")
	}

	m, err := s.client.CreateMembership(ctx, acct.WorkOSOrganizationID, userID, role)
	if err != nil {
		return nil, fmt.Errorf("failed to create WorkOS membership: %w", err)
	}

	if err := s.accountStore.AddMember(accountID, userID, m.ID); err != nil {
		_ = s.client.DeleteMembership(ctx, m.ID)
		return nil, fmt.Errorf("failed to add local member: %w", err)
	}

	return &account.AccountMember{
		AccountID:          accountID,
		UserID:             userID,
		WorkOSMembershipID: m.ID,
	}, nil
}

// ChangeMemberRole updates a member's role in WorkOS. No local role to update.
func (s *Sync) ChangeMemberRole(ctx context.Context, accountID, userID, newRole string) error {
	acct, err := s.accountStore.GetByID(accountID)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}
	if acct.Type == "personal" {
		return fmt.Errorf("cannot change role on personal account")
	}

	member, err := s.accountStore.GetMember(accountID, userID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}

	currentMembership, err := s.client.GetMembership(ctx, member.WorkOSMembershipID)
	if err != nil {
		return fmt.Errorf("failed to get WorkOS membership: %w", err)
	}

	if currentMembership.RoleSlug == "owner" && newRole != "owner" {
		memberships, err := s.client.ListMemberships(ctx, acct.WorkOSOrganizationID, ListOpts{Limit: 100})
		if err != nil {
			return fmt.Errorf("failed to list WorkOS memberships: %w", err)
		}
		ownerCount := 0
		for _, m := range memberships {
			if m.RoleSlug == "owner" {
				ownerCount++
			}
		}
		if ownerCount <= 1 {
			return fmt.Errorf("cannot change role: account must have at least one owner")
		}
	}

	if _, err := s.client.UpdateMembershipRole(ctx, member.WorkOSMembershipID, newRole); err != nil {
		return fmt.Errorf("failed to update WorkOS membership role: %w", err)
	}

	return nil
}

// RemoveMember removes a member from an org account, deleting from WorkOS first then locally.
func (s *Sync) RemoveMember(ctx context.Context, accountID, userID string) error {
	acct, err := s.accountStore.GetByID(accountID)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}
	if acct.Type == "personal" {
		return fmt.Errorf("cannot remove members from personal accounts")
	}

	member, err := s.accountStore.GetMember(accountID, userID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}

	// Last-owner guard via WorkOS
	membership, err := s.client.GetMembership(ctx, member.WorkOSMembershipID)
	if err != nil {
		return fmt.Errorf("failed to get WorkOS membership: %w", err)
	}
	if membership.RoleSlug == "owner" {
		memberships, err := s.client.ListMemberships(ctx, acct.WorkOSOrganizationID, ListOpts{Limit: 100})
		if err != nil {
			return fmt.Errorf("failed to list WorkOS memberships: %w", err)
		}
		ownerCount := 0
		for _, m := range memberships {
			if m.RoleSlug == "owner" {
				ownerCount++
			}
		}
		if ownerCount <= 1 {
			return fmt.Errorf("cannot remove member: account must have at least one owner")
		}
	}

	if err := s.client.DeleteMembership(ctx, member.WorkOSMembershipID); err != nil {
		return fmt.Errorf("failed to delete WorkOS membership: %w", err)
	}

	return s.accountStore.RemoveMember(accountID, userID)
}

// SyncMembershipsForUser reconciles local account_members with WorkOS memberships
// for a specific user. Called on login as a fallback sync mechanism.
func (s *Sync) SyncMembershipsForUser(ctx context.Context, userID string) error {
	memberships, err := s.client.ListMembershipsForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to list WorkOS memberships: %w", err)
	}

	for _, m := range memberships {
		if m.Status != "active" {
			continue
		}

		// Find the local account linked to this WorkOS org
		acct, err := s.accountStore.GetByWorkOSOrganizationID(m.OrganizationID)
		if err != nil {
			// No local account for this WorkOS org — skip
			continue
		}

		// Upsert the membership locally
		if err := s.accountStore.UpsertMemberByWorkosMembershipID(
			acct.ID, m.UserID, m.ID,
		); err != nil {
			return fmt.Errorf("failed to upsert member for account %s: %w", acct.ID, err)
		}
	}

	return nil
}
