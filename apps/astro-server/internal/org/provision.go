package org

import (
	"context"
	"errors"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

const workosAdminRole = "admin"

type directory interface {
	GetOrganizationByExternalID(ctx context.Context, externalID string) (Organization, error)
	CreateOrganization(ctx context.Context, name, externalID string) (Organization, error)
	DeleteOrganization(ctx context.Context, workosOrgID string) error
	ListAllMemberships(ctx context.Context, workosOrgID string) ([]Membership, error)
	CreateMembership(ctx context.Context, workosOrgID, userID, roleSlug string) (Membership, error)
}

type Provisioner struct {
	directory    directory
	accounts     *account.AccountStore
	log          *logger.Logger
	accountRoles *authz.RoleProjector
}

func NewProvisioner(client *Client, accounts *account.AccountStore, log *logger.Logger) *Provisioner {
	if client == nil || accounts == nil {
		return nil
	}
	return &Provisioner{directory: client, accounts: accounts, log: log}
}

func (p *Provisioner) EnsureOrganization(ctx context.Context, accountID string) (string, error) {
	acct, err := p.accounts.GetByID(accountID)
	if err != nil {
		return "", err
	}

	orgID := acct.WorkOSOrganizationID
	if orgID == "" {
		orgID, err = p.resolveOrganization(ctx, acct)
		if err != nil {
			return "", err
		}
		if err := p.accounts.SetWorkOSOrganizationID(acct.ID, orgID); err != nil {
			return "", fmt.Errorf("link WorkOS organization: %w", err)
		}
	}

	if err := p.ensureOwnerMembership(ctx, acct, orgID); err != nil {
		return "", err
	}
	return orgID, nil
}

func (p *Provisioner) DiscardOrganization(ctx context.Context, accountID string) {
	orgID := ""
	if acct, err := p.accounts.GetByID(accountID); err == nil {
		orgID = acct.WorkOSOrganizationID
	}
	if orgID == "" {
		existing, err := p.directory.GetOrganizationByExternalID(ctx, accountID)
		if err != nil {
			return
		}
		orgID = existing.ID
	}
	if err := p.directory.DeleteOrganization(ctx, orgID); err != nil {
		p.log.Warn("org provision: discard WorkOS organization failed", "account_id", accountID, "workos_org_id", orgID, "error", err)
	}
}

func (p *Provisioner) resolveOrganization(ctx context.Context, acct *account.Account) (string, error) {
	existing, err := p.directory.GetOrganizationByExternalID(ctx, acct.ID)
	switch {
	case err == nil:
		return existing.ID, nil
	case !errors.Is(err, ErrOrganizationNotFound):
		return "", fmt.Errorf("look up WorkOS organization: %w", err)
	}

	created, err := p.directory.CreateOrganization(ctx, acct.Name, acct.ID)
	if err != nil {
		return "", fmt.Errorf("create WorkOS organization: %w", err)
	}
	return created.ID, nil
}

// SetAccountRoles wires the projector that records the owner's account role.
// Both the create path and the hourly sweep run through here, so a first
// attempt that failed to reach WorkOS is repaired on the next sweep.
func (p *Provisioner) SetAccountRoles(projector *authz.RoleProjector) {
	p.accountRoles = projector
}

func (p *Provisioner) ensureOwnerMembership(ctx context.Context, acct *account.Account, orgID string) error {
	owner, err := p.accounts.OwnerUserID(acct.ID)
	if err != nil {
		return fmt.Errorf("resolve account owner: %w", err)
	}

	memberships, err := p.directory.ListAllMemberships(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list WorkOS memberships: %w", err)
	}
	membershipID := ""
	for _, m := range memberships {
		if m.UserID == owner {
			membershipID = m.ID
			break
		}
	}
	if membershipID == "" {
		m, err := p.directory.CreateMembership(ctx, orgID, owner, workosAdminRole)
		if err != nil {
			return fmt.Errorf("create owner membership: %w", err)
		}
		membershipID = m.ID
	}

	if err := p.accounts.UpsertMemberByWorkosMembershipID(acct.ID, owner, membershipID); err != nil {
		p.log.Warn("org provision: record owner membership failed, healed at next login",
			"account_id", acct.ID, "workos_membership_id", membershipID, "error", err)
	}
	if acct.Type == "organization" {
		if err := p.accountRoles.ProjectAccountRole(ctx, acct.ID, orgID, owner, membershipID, workosAdminRole); err != nil {
			p.log.Warn("org provision: project owner account role failed",
				"account_id", acct.ID, "workos_membership_id", membershipID, "error", err)
		}
	}
	return nil
}
