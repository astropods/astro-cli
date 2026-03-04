package org

import (
	"context"
	"fmt"
	"time"

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

	if acct.WorkOSOrganizationID == "" {
		// Personal account — local only
		if err := s.accountStore.AddMember(accountID, userID, role, ""); err != nil {
			return nil, err
		}
		return &account.AccountMember{
			AccountID: accountID,
			UserID:    userID,
			Role:      role,
		}, nil
	}

	// Org account — write to WorkOS first
	m, err := s.client.CreateMembership(ctx, acct.WorkOSOrganizationID, userID, role)
	if err != nil {
		return nil, fmt.Errorf("failed to create WorkOS membership: %w", err)
	}

	if err := s.accountStore.AddMember(accountID, userID, role, m.ID); err != nil {
		// Compensating action: clean up WorkOS membership
		_ = s.client.DeleteMembership(ctx, m.ID)
		return nil, fmt.Errorf("failed to add local member: %w", err)
	}

	return &account.AccountMember{
		AccountID:          accountID,
		UserID:             userID,
		Role:               role,
		WorkOSMembershipID: m.ID,
	}, nil
}

// ChangeMemberRole updates a member's role, writing to WorkOS first then locally.
func (s *Sync) ChangeMemberRole(ctx context.Context, accountID, userID, newRole string) error {
	member, err := s.accountStore.GetMember(accountID, userID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}

	// Prevent demoting the last owner
	if member.Role == "owner" && newRole != "owner" {
		count, err := s.accountStore.CountOwners(accountID)
		if err != nil {
			return fmt.Errorf("failed to check owner count: %w", err)
		}
		if count <= 1 {
			return fmt.Errorf("cannot change role: account must have at least one owner")
		}
	}

	if member.WorkOSMembershipID != "" {
		// Org account — update WorkOS first
		if _, err := s.client.UpdateMembershipRole(ctx, member.WorkOSMembershipID, newRole); err != nil {
			return fmt.Errorf("failed to update WorkOS membership role: %w", err)
		}
	}

	return s.accountStore.UpdateMemberRole(accountID, userID, newRole)
}

// RemoveMember removes a member from an account, deleting from WorkOS first then locally.
func (s *Sync) RemoveMember(ctx context.Context, accountID, userID string) error {
	member, err := s.accountStore.GetMember(accountID, userID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}

	// Prevent removing the last owner
	if member.Role == "owner" {
		count, err := s.accountStore.CountOwners(accountID)
		if err != nil {
			return fmt.Errorf("failed to check owner count: %w", err)
		}
		if count <= 1 {
			return fmt.Errorf("cannot remove member: account must have at least one owner")
		}
	}

	if member.WorkOSMembershipID != "" {
		if err := s.client.DeleteMembership(ctx, member.WorkOSMembershipID); err != nil {
			return fmt.Errorf("failed to delete WorkOS membership: %w", err)
		}
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
			acct.ID, m.UserID, m.RoleSlug, m.ID, time.Now(),
		); err != nil {
			return fmt.Errorf("failed to upsert member for account %s: %w", acct.ID, err)
		}
	}

	return nil
}
