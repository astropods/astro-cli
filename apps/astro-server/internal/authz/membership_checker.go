package authz

import "context"

type memberChecker interface {
	IsMemberContext(context.Context, string, string) (bool, error)
}

// AccountResolver maps a resource to its owning account and whether it is personal.
type AccountResolver interface {
	AccountForResource(ctx context.Context, res ResourceRef) (accountID string, personal bool, err error)
}

// MembershipChecker reproduces today's membership-based authorization: any member
// of the resource's owning account may perform the action. The personal flag from
// AccountResolver is ignored here; org-scope (JWT vs account org) is not checked
// until deployment middleware wires SessionOrgMatchesAccount (see org_scope.go).
type MembershipChecker struct {
	members  memberChecker
	resolver AccountResolver
}

func NewMembershipChecker(m memberChecker, r AccountResolver) *MembershipChecker {
	return &MembershipChecker{members: m, resolver: r}
}

func (c *MembershipChecker) Authorize(ctx context.Context, sub Subject, _ Action, res ResourceRef) (bool, error) {
	accountID, _, err := c.resolver.AccountForResource(ctx, res)
	if err != nil {
		return false, err
	}
	return c.members.IsMemberContext(ctx, accountID, sub.UserID)
}
