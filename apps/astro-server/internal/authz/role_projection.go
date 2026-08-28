package authz

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/astropods/astro/apps/astro-server/internal/account"
)

// WorkOS organization roles that run an Astro account.
const (
	organizationRoleOwner = "owner"
	organizationRoleAdmin = "admin"
)

// AccountRoleForOrganizationRole maps a WorkOS organization role to the account
// role Astro projects onto the Account resource. Owners and admins run the
// account; every other role becomes a member, which holds no child-resource
// permission.
func AccountRoleForOrganizationRole(organizationRole string) RoleSlug {
	switch organizationRole {
	case organizationRoleOwner, organizationRoleAdmin:
		return RoleAccountAdmin
	default:
		return RoleAccountMember
	}
}

type intentRecorder interface {
	Record(context.Context, AccessIntent) (AccessIntent, bool, error)
}

type memberIdentityStore interface {
	GetMemberContext(ctx context.Context, accountID, userID string) (*account.AccountMember, error)
}

type accessReconcileQueue interface {
	InsertResourceAccessFGAReconcileJob(context.Context, AccessIntentKey) error
}

// RoleProjector records the role intent Astro derives rather than a user
// choosing it: the creator's admin role on a new resource, and each member's
// account role. It writes the same ledger as the access API, so the reconciler
// converges it and a WorkOS failure retries. A nil projector is a no-op, which
// is how a deployment without WorkOS configured behaves.
type RoleProjector struct {
	intents intentRecorder
	members memberIdentityStore
	queue   accessReconcileQueue
}

func NewRoleProjector(intents intentRecorder, members memberIdentityStore, queue accessReconcileQueue) *RoleProjector {
	return &RoleProjector{intents: intents, members: members, queue: queue}
}

// GrantCreatorAdmin gives the user who created a resource its admin role.
// Without it a new resource reaches only the account admins, including for the
// person who just created it.
func (p *RoleProjector) GrantCreatorAdmin(ctx context.Context, accountID, organizationID, userID string, resource ResourceRef) error {
	if p == nil {
		return nil
	}
	if p.members == nil {
		return errors.New("role projector is not configured")
	}
	role, err := RoleForAccessLevel(resource.Type, AccessLevelAdmin)
	if err != nil {
		return err
	}
	member, err := p.members.GetMemberContext(ctx, accountID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAccessSubjectNotProvisioned
		}
		return fmt.Errorf("resolve resource creator: %w", err)
	}
	if member == nil || member.WorkOSMembershipID == "" {
		return ErrAccessSubjectNotProvisioned
	}
	return p.record(ctx, accountID, organizationID, resource, userID, member.WorkOSMembershipID, role)
}

// ProjectAccountRole records the account role a member's WorkOS organization
// role implies.
func (p *RoleProjector) ProjectAccountRole(ctx context.Context, accountID, organizationID, userID, membershipID, organizationRole string) error {
	if p == nil {
		return nil
	}
	return p.record(ctx, accountID, organizationID, AccountResource(accountID),
		userID, membershipID, AccountRoleForOrganizationRole(organizationRole))
}

// RevokeAccountRole removes a former member's direct account role.
func (p *RoleProjector) RevokeAccountRole(ctx context.Context, accountID, organizationID, userID, membershipID string) error {
	if p == nil {
		return nil
	}
	return p.record(ctx, accountID, organizationID, AccountResource(accountID), userID, membershipID, "")
}

func (p *RoleProjector) record(
	ctx context.Context,
	accountID, organizationID string,
	resource ResourceRef,
	userID, membershipID string,
	role RoleSlug,
) error {
	if p.intents == nil {
		return errors.New("role projector is not configured")
	}
	if membershipID == "" {
		return ErrAccessSubjectNotProvisioned
	}
	intent, changed, err := p.intents.Record(ctx, AccessIntent{
		AccountID: accountID, OrganizationID: organizationID, Resource: resource,
		Subject: MembershipAssignmentSubject(membershipID), SubjectID: userID, DesiredRole: role,
	})
	if err != nil {
		return fmt.Errorf("record %s role intent: %w", resource.Type, err)
	}
	if !changed && intent.Status() == AccessSyncSynced {
		return nil
	}
	if p.queue == nil {
		return nil
	}
	if err := p.queue.InsertResourceAccessFGAReconcileJob(ctx, intent.Key()); err != nil {
		slog.WarnContext(ctx, "role projection: enqueue reconciliation failed, periodic sweep will retry",
			"account_id", accountID,
			"resource_type", resource.Type,
			"resource_id", resource.ExternalID,
			"user_id", userID,
			"error", err,
		)
	}
	return nil
}
