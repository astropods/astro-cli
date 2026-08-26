package riverqueue

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/org"
)

// managerRoleSlugs are the org roles that count as managers (the org:manage
// permission): the account owner and admins.
var managerRoleSlugs = map[string]bool{"owner": true, "admin": true}

// managerResolver resolves an account's org managers to WorkOS user ids by
// querying WorkOS org memberships. It satisfies notify's managerLookup. A
// personal account returns an empty slice, so the Deliverer falls back to the
// owner.
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
	if acct.Type != "organization" {
		return nil, nil
	}
	if acct.WorkOSOrganizationID == "" {
		return nil, fmt.Errorf("account %s has no WorkOS organization", accountID)
	}
	mems, err := m.org.ListAllMemberships(ctx, acct.WorkOSOrganizationID)
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
