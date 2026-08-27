package handlers

import (
	"context"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

const authorizationResourceRegistrationTimeout = 5 * time.Second

func registerAuthorizationResource(
	ctx context.Context,
	log *logger.Logger,
	registrar authz.ResourceRegistrar,
	acct *account.Account,
	resource authz.ResourceRef,
	name string,
) bool {
	if registrar == nil || acct == nil || acct.Type != "organization" || acct.WorkOSOrganizationID == "" {
		return false
	}
	registrationCtx, cancel := context.WithTimeout(ctx, authorizationResourceRegistrationTimeout)
	defer cancel()
	if err := registrar.RegisterResourceWithParent(
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

func registerAccountAuthorizationResources(
	ctx context.Context,
	log *logger.Logger,
	registrar authz.ResourceRegistrar,
	acct *account.Account,
) {
	if registrar == nil || acct == nil || acct.Type != "organization" || acct.WorkOSOrganizationID == "" {
		return
	}
	registrationCtx, cancel := context.WithTimeout(ctx, authorizationResourceRegistrationTimeout)
	defer cancel()
	name := acct.DisplayName
	if name == "" {
		name = acct.Name
	}
	if err := registrar.RegisterResourceWithParent(
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
		return
	}
	registerAuthorizationResource(registrationCtx, log, registrar, acct, authz.InsightsResource(acct.ID), "Insights")
}
