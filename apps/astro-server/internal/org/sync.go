package org

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
)

// Sync provides write-path sync logic (Astro → WorkOS) for organization memberships.
type Sync struct {
	client       *Client
	accountStore *account.AccountStore
	workos       *auth.WorkOSClient
	db           *sql.DB
}

// NewSync creates a new Sync instance.
func NewSync(client *Client, accountStore *account.AccountStore, workos *auth.WorkOSClient, db *sql.DB) *Sync {
	return &Sync{client: client, accountStore: accountStore, workos: workos, db: db}
}

// ownerGuardLockKey returns a stable int64 advisory lock key derived from the
// WorkOS org ID. This serializes owner-count checks so concurrent requests
// cannot race past the last-owner guard.
func ownerGuardLockKey(workosOrgID string) int64 {
	h := fnv.New64a()
	h.Write([]byte("owner-guard:" + workosOrgID))
	return int64(h.Sum64())
}

// withOwnerGuardLock acquires a Postgres advisory lock scoped to a transaction
// for the given org, executes fn, and commits. This serializes owner-count
// checks against concurrent role changes or removals.
func (s *Sync) withOwnerGuardLock(ctx context.Context, workosOrgID string, fn func() error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin owner-guard tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", ownerGuardLockKey(workosOrgID)); err != nil {
		return fmt.Errorf("acquire owner-guard lock: %w", err)
	}

	if err := fn(); err != nil {
		return err
	}

	return tx.Commit()
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
// Returns the previous role slug so callers can include it in audit logs.
func (s *Sync) ChangeMemberRole(ctx context.Context, accountID, userID, newRole string) (previousRole string, err error) {
	acct, err := s.accountStore.GetByID(accountID)
	if err != nil {
		return "", fmt.Errorf("account not found: %w", err)
	}
	if acct.Type == "personal" {
		return "", fmt.Errorf("cannot change role on personal account")
	}
	if acct.WorkOSOrganizationID == "" {
		return "", fmt.Errorf("organization has no WorkOS link")
	}

	// Verify the WorkOS organization is accessible
	if _, err := s.client.GetOrganization(ctx, acct.WorkOSOrganizationID); err != nil {
		return "", fmt.Errorf("WorkOS organization not reachable: %w", err)
	}

	member, err := s.accountStore.GetMember(accountID, userID)
	if err != nil {
		return "", fmt.Errorf("member not found: %w", err)
	}
	if member.WorkOSMembershipID == "" {
		return "", fmt.Errorf("member has no WorkOS membership")
	}

	currentMembership, err := s.client.GetMembership(ctx, member.WorkOSMembershipID)
	if err != nil {
		return "", fmt.Errorf("failed to get WorkOS membership: %w", err)
	}

	previousRole = currentMembership.RoleSlug

	// Serialize owner-guard check + mutation to prevent TOCTOU races.
	err = s.withOwnerGuardLock(ctx, acct.WorkOSOrganizationID, func() error {
		if currentMembership.RoleSlug == "owner" && newRole != "owner" {
			memberships, err := s.client.ListMemberships(ctx, acct.WorkOSOrganizationID, ListOpts{Limit: 100})
			if err != nil {
				return fmt.Errorf("failed to list WorkOS memberships: %w", err)
			}
			ownerCount := 0
			for _, m := range memberships {
				if m.RoleSlug == "owner" && m.Status == "active" {
					ownerCount++
				}
			}
			if ownerCount <= 1 {
				return fmt.Errorf("cannot change role: account must have at least one owner")
			}
		}

		updated, err := s.client.UpdateMembershipRole(ctx, member.WorkOSMembershipID, newRole)
		if err != nil {
			return fmt.Errorf("failed to update WorkOS membership role: %w", err)
		}

		if updated.RoleSlug != newRole {
			return fmt.Errorf("role update was not applied: expected %q but got %q", newRole, updated.RoleSlug)
		}

		return nil
	})
	return previousRole, err
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
	if acct.WorkOSOrganizationID == "" {
		return fmt.Errorf("organization has no WorkOS link")
	}

	member, err := s.accountStore.GetMember(accountID, userID)
	if err != nil {
		// No local DB entry — the member may only exist in WorkOS (e.g. pending
		// invitation shown via include_pending). Find their membership directly.
		membership, membershipErr := s.findMembershipForUser(ctx, acct.WorkOSOrganizationID, userID)
		if membershipErr != nil {
			return fmt.Errorf("member not found locally or in WorkOS: %w", membershipErr)
		}
		if err := s.client.DeleteMembership(ctx, membership.ID); err != nil {
			return fmt.Errorf("failed to delete WorkOS membership: %w", err)
		}
		if membership.Status == "pending" {
			s.revokeInvitationsForUser(ctx, acct.WorkOSOrganizationID, userID)
		}
		return nil
	}
	if member.WorkOSMembershipID == "" {
		// No WorkOS membership — just remove locally
		return s.accountStore.RemoveMember(accountID, userID)
	}

	// Last-owner guard via WorkOS
	membership, err := s.client.GetMembership(ctx, member.WorkOSMembershipID)
	if err != nil {
		return fmt.Errorf("failed to get WorkOS membership: %w", err)
	}

	// Serialize owner-guard check + deletion to prevent TOCTOU races.
	return s.withOwnerGuardLock(ctx, acct.WorkOSOrganizationID, func() error {
		if membership.RoleSlug == "owner" {
			memberships, err := s.client.ListMemberships(ctx, acct.WorkOSOrganizationID, ListOpts{Limit: 100})
			if err != nil {
				return fmt.Errorf("failed to list WorkOS memberships: %w", err)
			}
			ownerCount := 0
			for _, m := range memberships {
				if m.RoleSlug == "owner" && m.Status == "active" {
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

		if membership.Status == "pending" {
			s.revokeInvitationsForUser(ctx, acct.WorkOSOrganizationID, userID)
		}

		return s.accountStore.RemoveMember(accountID, userID)
	})
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

// SendBulkInvitations resolves each InviteRequest to an email and sends a
// WorkOS invitation. Results are returned per-entry; individual failures do
// not abort the batch.
func (s *Sync) SendBulkInvitations(ctx context.Context, workosOrgID, inviterUserID string, reqs []InviteRequest) []InviteResult {
	results := make([]InviteResult, 0, len(reqs))
	for _, r := range reqs {
		res := InviteResult{Value: r.Value, Kind: r.Kind}
		role := r.RoleSlug
		if role == "" {
			role = "member"
		}

		var email string
		switch r.Kind {
		case "email":
			email = r.Value
		case "account":
			resolved, err := s.resolveAccountEmail(ctx, r.Value)
			if err != nil {
				res.Error = err.Error()
				results = append(results, res)
				continue
			}
			email = resolved
		default:
			res.Error = fmt.Sprintf("unknown invite kind %q", r.Kind)
			results = append(results, res)
			continue
		}

		res.Email = email
		inv, err := s.client.SendInvitation(ctx, workosOrgID, email, inviterUserID, role)
		if err != nil {
			res.Error = err.Error()
		} else {
			res.Success = true
			res.Invitation = &inv
		}
		results = append(results, res)
	}
	return results
}

// findMembershipForUser searches WorkOS memberships for the given org and returns
// the one matching the specified user ID.
func (s *Sync) findMembershipForUser(ctx context.Context, workosOrgID, userID string) (Membership, error) {
	memberships, err := s.client.ListMemberships(ctx, workosOrgID, ListOpts{Limit: 100})
	if err != nil {
		return Membership{}, fmt.Errorf("failed to list memberships: %w", err)
	}
	for _, m := range memberships {
		if m.UserID == userID {
			return m, nil
		}
	}
	return Membership{}, fmt.Errorf("no membership found for user %s in org %s", userID, workosOrgID)
}

// revokeInvitationsForUser revokes any pending WorkOS invitations matching the
// given user's email in the specified organization. Best-effort: failures are
// silently ignored because the membership has already been deleted.
func (s *Sync) revokeInvitationsForUser(ctx context.Context, workosOrgID, userID string) {
	user, err := s.workos.GetUser(ctx, userID)
	if err != nil || user.Email == "" {
		return
	}
	invitations, err := s.client.ListInvitations(ctx, workosOrgID)
	if err != nil {
		return
	}
	for _, inv := range invitations {
		if inv.Email == user.Email {
			_ = s.client.RevokeInvitation(ctx, inv.ID)
		}
	}
}

// resolveAccountEmail looks up the email address of an account's first member
// by going through accountStore → WorkOS user.
func (s *Sync) resolveAccountEmail(ctx context.Context, accountName string) (string, error) {
	acct, err := s.accountStore.GetByName(accountName)
	if err != nil {
		return "", fmt.Errorf("account %q not found: %w", accountName, err)
	}
	userID, err := s.accountStore.GetFirstMemberUserID(acct.ID)
	if err != nil {
		return "", fmt.Errorf("no member found for account %q: %w", accountName, err)
	}
	user, err := s.workos.GetUser(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve user for account %q: %w", accountName, err)
	}
	return user.Email, nil
}
