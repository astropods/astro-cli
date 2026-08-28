package authzbackfill

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

var ErrIncomplete = errors.New("authorization resource backfill incomplete")

type Account struct {
	ID                string
	OrganizationID    string
	Name              string
	OwnerMembershipID string
}

type Resource struct {
	AccountID string
	Ref       authz.ResourceRef
	Name      string
}

type Failure struct {
	AccountID string
	Resource  authz.ResourceRef
	Operation string
	Err       error
}

type Summary struct {
	BlueprintIDsBackfilled int
	ResourcesMissing       int
	ResourcesCreated       int
	ResourcesExisting      int
	AdminsAssigned         int
	AdminsExisting         int
	Failures               []Failure
}

type Source interface {
	BackfillBlueprintIDs(context.Context, int, bool) (int, error)
	ListAccounts(context.Context, string, int) ([]Account, error)
	ListResources(context.Context, []string) (map[string][]Resource, error)
}

type WorkOS interface {
	RegisterResourceWithParent(context.Context, string, authz.ResourceRef, authz.ResourceRef, string) error
	GetResource(context.Context, string, authz.ResourceRef) (authz.AuthorizationResource, error)
	ListAuthorizationResourcesForOrganization(context.Context, string) ([]authz.AuthorizationResource, error)
	AssignRole(context.Context, authz.AssignmentSubject, authz.RoleSlug, authz.ResourceRef) error
}

type Backfiller struct {
	source    Source
	workos    WorkOS
	batchSize int
	dryRun    bool
}

const accountTimeout = 2 * time.Minute

func New(source Source, workos WorkOS, batchSize int, dryRun bool) *Backfiller {
	if batchSize < 1 {
		batchSize = 100
	}
	return &Backfiller{source: source, workos: workos, batchSize: batchSize, dryRun: dryRun}
}

func (b *Backfiller) Run(ctx context.Context) (Summary, error) {
	var summary Summary
	if b.source == nil || b.workos == nil {
		return summary, errors.New("authorization resource backfill is not configured")
	}

	count, err := b.source.BackfillBlueprintIDs(ctx, b.batchSize, b.dryRun)
	if err != nil {
		return summary, fmt.Errorf("backfill blueprint ids: %w", err)
	}
	summary.BlueprintIDsBackfilled = count

	var after string
	for {
		accounts, err := b.source.ListAccounts(ctx, after, b.batchSize)
		if err != nil {
			return summary, fmt.Errorf("list authorization backfill accounts: %w", err)
		}
		if len(accounts) == 0 {
			break
		}
		accountIDs := make([]string, 0, len(accounts))
		for _, account := range accounts {
			accountIDs = append(accountIDs, account.ID)
		}
		resources, err := b.source.ListResources(ctx, accountIDs)
		if err != nil {
			return summary, fmt.Errorf("list authorization backfill resources: %w", err)
		}
		for _, account := range accounts {
			accountCtx, cancel := context.WithTimeout(ctx, accountTimeout)
			b.backfillAccount(accountCtx, account, resources[account.ID], &summary)
			cancel()
		}
		after = accounts[len(accounts)-1].ID
	}

	if len(summary.Failures) != 0 {
		return summary, ErrIncomplete
	}
	return summary, nil
}

func (b *Backfiller) backfillAccount(ctx context.Context, account Account, children []Resource, summary *Summary) {
	existing, err := b.workos.ListAuthorizationResourcesForOrganization(ctx, account.OrganizationID)
	if err != nil {
		b.fail(summary, account.ID, authz.AccountResource(account.ID), "list", err)
		return
	}
	byRef := make(map[authz.ResourceRef]authz.AuthorizationResource, len(existing))
	for _, resource := range existing {
		byRef[resource.Resource] = resource
	}

	accountRef := authz.AccountResource(account.ID)
	accountResource, found := byRef[accountRef]
	if found {
		summary.ResourcesExisting++
		if organizationResource, ok := byRef[authz.OrganizationResource(account.OrganizationID)]; ok && accountResource.ParentResourceID != organizationResource.ID {
			b.fail(summary, account.ID, accountRef, "validate parent", fmt.Errorf("WorkOS parent %q does not match organization resource %q", accountResource.ParentResourceID, organizationResource.ID))
			return
		}
	} else {
		summary.ResourcesMissing++
		if b.dryRun {
			for range children {
				summary.ResourcesMissing++
			}
			b.planAdmin(account, summary)
			return
		}
		if err := b.workos.RegisterResourceWithParent(ctx, account.OrganizationID, accountRef, authz.OrganizationResource(account.OrganizationID), account.Name); err != nil {
			b.fail(summary, account.ID, accountRef, "create", err)
			return
		}
		summary.ResourcesCreated++
		accountResource, err = b.workos.GetResource(ctx, account.OrganizationID, accountRef)
		if err != nil {
			b.fail(summary, account.ID, accountRef, "read after create", err)
			return
		}
	}

	for _, child := range children {
		current, found := byRef[child.Ref]
		if found {
			summary.ResourcesExisting++
			if current.ParentResourceID != accountResource.ID {
				b.fail(summary, account.ID, child.Ref, "validate parent", fmt.Errorf("WorkOS parent %q does not match Account resource %q", current.ParentResourceID, accountResource.ID))
			}
			continue
		}
		summary.ResourcesMissing++
		if b.dryRun {
			continue
		}
		if err := b.workos.RegisterResourceWithParent(ctx, account.OrganizationID, child.Ref, accountRef, child.Name); err != nil {
			b.fail(summary, account.ID, child.Ref, "create", err)
			continue
		}
		summary.ResourcesCreated++
	}
	b.assignAdmin(ctx, account, summary)
}

func (b *Backfiller) planAdmin(account Account, summary *Summary) {
	if account.OwnerMembershipID == "" {
		b.fail(summary, account.ID, authz.AccountResource(account.ID), "assign admin", errors.New("account owner has no WorkOS membership mirror"))
	}
}

func (b *Backfiller) assignAdmin(ctx context.Context, account Account, summary *Summary) {
	if account.OwnerMembershipID == "" {
		b.fail(summary, account.ID, authz.AccountResource(account.ID), "assign admin", errors.New("account owner has no WorkOS membership mirror"))
		return
	}
	if b.dryRun {
		return
	}
	err := b.workos.AssignRole(ctx, authz.MembershipAssignmentSubject(account.OwnerMembershipID), authz.RoleAccountAdmin, authz.AccountResource(account.ID))
	switch {
	case err == nil:
		summary.AdminsAssigned++
	case errors.Is(err, authz.ErrRoleAssignmentExists):
		summary.AdminsExisting++
	default:
		b.fail(summary, account.ID, authz.AccountResource(account.ID), "assign admin", err)
	}
}

func (b *Backfiller) fail(summary *Summary, accountID string, resource authz.ResourceRef, operation string, err error) {
	summary.Failures = append(summary.Failures, Failure{AccountID: accountID, Resource: resource, Operation: operation, Err: err})
}
