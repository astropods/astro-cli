package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

const authorizationResourceRegistrationTimeout = 5 * time.Second

func registerAuthorizationResource(
	ctx context.Context,
	log *logger.Logger,
	resources authz.ResourceLifecycle,
	acct *account.Account,
	resource authz.ResourceRef,
	name string,
) bool {
	if resources == nil || acct == nil || acct.Type != "organization" || acct.WorkOSOrganizationID == "" {
		return false
	}
	registrationCtx, cancel := context.WithTimeout(ctx, authorizationResourceRegistrationTimeout)
	defer cancel()
	if err := resources.RegisterResourceWithParent(
		registrationCtx,
		acct.WorkOSOrganizationID,
		resource,
		authz.AccountResource(acct.ID),
		name,
	); err != nil {
		log.Warn("authorization resource: direct registration failed",
			"account_id", acct.ID,
			"resource_type", resource.Type,
			"resource_id", resource.ExternalID,
			"error", err,
		)
		return false
	}
	return true
}

func deleteAuthorizationResource(
	ctx context.Context,
	log *logger.Logger,
	resources authz.ResourceLifecycle,
	acct *account.Account,
	resource authz.ResourceRef,
) {
	if resources == nil || acct == nil || acct.Type != "organization" || acct.WorkOSOrganizationID == "" {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, authorizationResourceRegistrationTimeout)
	defer cancel()
	if err := resources.DeleteResource(deleteCtx, acct.WorkOSOrganizationID, resource); err != nil && !errors.Is(err, authz.ErrResourceNotFound) {
		log.Warn("authorization resource: direct delete failed",
			"account_id", acct.ID,
			"resource_type", resource.Type,
			"resource_id", resource.ExternalID,
			"error", err,
		)
	}
}

// grantResourceCreatorAccess records the creator's admin role on a resource the
// caller just created. The intent is durable, so it converges even when the
// registration call above failed and the backfill creates the resource later.
func grantResourceCreatorAccess(
	ctx context.Context,
	log *logger.Logger,
	projector *authz.RoleProjector,
	acct *account.Account,
	resource authz.ResourceRef,
	creator *auth.User,
) {
	if projector == nil || acct == nil || creator == nil || acct.Type != "organization" || acct.WorkOSOrganizationID == "" {
		return
	}
	grantCtx, cancel := context.WithTimeout(ctx, authorizationResourceRegistrationTimeout)
	defer cancel()
	err := projector.GrantCreatorAdmin(grantCtx, acct.ID, acct.WorkOSOrganizationID, creator.ID, resource)
	attrs := []any{
		"account_id", acct.ID,
		"resource_type", resource.Type,
		"resource_id", resource.ExternalID,
		"user_id", creator.ID,
	}
	switch {
	case err == nil:
	case errors.Is(err, authz.ErrAccessSubjectNotProvisioned):
		log.Debug("authorization resource: creator has no WorkOS membership mirror", attrs...)
	default:
		log.Warn("authorization resource: grant creator access failed", append(attrs, "error", err)...)
	}
}

func registerAccountAuthorizationResource(
	ctx context.Context,
	log *logger.Logger,
	resources authz.ResourceLifecycle,
	acct *account.Account,
) {
	if resources == nil || acct == nil || acct.Type != "organization" || acct.WorkOSOrganizationID == "" {
		return
	}
	registrationCtx, cancel := context.WithTimeout(ctx, authorizationResourceRegistrationTimeout)
	defer cancel()
	name := acct.DisplayName
	if name == "" {
		name = acct.Name
	}
	if err := resources.RegisterResourceWithParent(
		registrationCtx,
		acct.WorkOSOrganizationID,
		authz.AccountResource(acct.ID),
		authz.OrganizationResource(acct.WorkOSOrganizationID),
		name,
	); err != nil {
		log.Warn("authorization resource: direct registration failed",
			"account_id", acct.ID,
			"resource_type", authz.ResourceAccount,
			"resource_id", acct.ID,
			"error", err,
		)
	}
}
