package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
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
