package auth

import (
	"errors"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/account"
)

// ErrWorkOSMembershipIDNotFound means the local member has no WorkOS membership link yet.
var ErrWorkOSMembershipIDNotFound = errors.New("workos organization membership id not found")

// MembershipIDResolver resolves WorkOS org membership ids from local account state.
type MembershipIDResolver interface {
	GetByWorkOSOrganizationID(orgID string) (*account.Account, error)
	GetMember(accountID, userID string) (*account.AccountMember, error)
}

// ResolveWorkOSMembershipID returns the caller's WorkOS org membership id (om_*).
// JWT claim wins when present; otherwise falls back to account_member_workos for org-scoped sessions.
func ResolveWorkOSMembershipID(resolver MembershipIDResolver, userID, orgID, fromJWT string) (string, error) {
	if orgID == "" {
		return "", nil
	}
	if fromJWT != "" {
		return fromJWT, nil
	}
	if resolver == nil {
		return "", errors.New("membership resolver is required for an organization-scoped session")
	}

	acct, err := resolver.GetByWorkOSOrganizationID(orgID)
	if err != nil {
		return "", fmt.Errorf("resolve account for WorkOS organization %q: %w", orgID, err)
	}

	member, err := resolver.GetMember(acct.ID, userID)
	if err != nil {
		return "", fmt.Errorf("resolve member %q for account %q: %w", userID, acct.ID, err)
	}
	if member.WorkOSMembershipID == "" {
		return "", ErrWorkOSMembershipIDNotFound
	}

	return member.WorkOSMembershipID, nil
}
