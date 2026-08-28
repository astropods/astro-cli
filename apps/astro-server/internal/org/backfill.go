package org

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/account"
)

// OwnerRoleBackfill counts the outcome of one BackfillOwnerRoles pass.
type OwnerRoleBackfill struct {
	Accounts     int
	Owners       int
	Repaired     int
	Unchanged    int
	Failed       int
	NoMembership int
}

// BackfillOwnerRoles promotes every owner whose WorkOS role cannot administer
// their account. Safe to re-run. A dry run counts the writes it skips as Repaired.
func (s *Sync) BackfillOwnerRoles(ctx context.Context, dryRun bool) (OwnerRoleBackfill, error) {
	var result OwnerRoleBackfill

	links, err := s.accountStore.OwnedOrganizationLinks()
	if err != nil {
		return result, fmt.Errorf("failed to list owned organization links: %w", err)
	}
	result.Accounts = len(links)

	owners, byOwner := groupLinksByOwner(links)
	result.Owners = len(owners)

	for _, owner := range owners {
		memberships, err := s.client.ListMembershipsForUser(ctx, owner)
		if err != nil {
			result.Failed += len(byOwner[owner])
			s.warn("org backfill: list memberships for owner failed", owner, err)
			continue
		}
		byOrg := make(map[string]Membership, len(memberships))
		for _, m := range memberships {
			byOrg[m.OrganizationID] = m
		}

		for _, l := range byOwner[owner] {
			m, ok := byOrg[l.WorkOSOrganizationID]
			switch {
			case !ok:
				result.NoMembership++
			case adminRoleSlugs[m.RoleSlug]:
				result.Unchanged++
			case dryRun:
				result.Repaired++
			case s.repairOwnerRole(ctx, l.AccountID, m):
				result.Repaired++
			default:
				result.Failed++
			}
		}
	}
	return result, nil
}

// Map iteration is random, so a stable order makes two runs log one sequence.
func groupLinksByOwner(links []account.OwnedOrganizationLink) ([]string, map[string][]account.OwnedOrganizationLink) {
	owners := make([]string, 0, len(links))
	byOwner := make(map[string][]account.OwnedOrganizationLink, len(links))
	for _, l := range links {
		if _, seen := byOwner[l.OwnerUserID]; !seen {
			owners = append(owners, l.OwnerUserID)
		}
		byOwner[l.OwnerUserID] = append(byOwner[l.OwnerUserID], l)
	}
	return owners, byOwner
}
