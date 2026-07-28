package riverqueue

import (
	"context"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/org"
)

// managerRoleSlugs are the org roles that count as managers (the org:manage
// permission): the account owner and admins.
var managerRoleSlugs = map[string]bool{"owner": true, "admin": true}

// managerResolver resolves an account's org managers to WorkOS user ids by
// querying WorkOS org memberships. It satisfies notify's managerLookup. A
// personal account (no WorkOS org) returns an empty slice, so the Deliverer
// falls back to the owner.
type managerResolver struct {
	accounts *account.AccountStore
	org      *org.Client
}

// ManagerUserIDs returns the user ids of the account's owner + admins.
func (m *managerResolver) ManagerUserIDs(ctx context.Context, accountID string) ([]string, error) {
	acct, err := m.accounts.GetByID(accountID)
	if err != nil {
		return nil, err
	}
	if acct.WorkOSOrganizationID == "" {
		return nil, nil // personal account — no org roles; caller uses the owner
	}
	// Managers are few; one page of 100 covers any real org.
	mems, err := m.org.ListMemberships(ctx, acct.WorkOSOrganizationID, org.ListOpts{Limit: 100})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(mems))
	for _, mem := range mems {
		if managerRoleSlugs[mem.RoleSlug] {
			ids = append(ids, mem.UserID)
		}
	}
	return ids, nil
}
