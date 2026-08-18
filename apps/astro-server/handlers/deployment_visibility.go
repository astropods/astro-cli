package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

var errDeploymentVisibilityUnavailable = errors.New("deployment visibility is unavailable")

type deploymentVisibilityResolver interface {
	Active() bool
	Visible(context.Context, string, []account.AccountWithRole) (authz.DeploymentVisibility, error)
}

func resolveDeploymentVisibility(
	ctx context.Context,
	resolver deploymentVisibilityResolver,
	userID string,
	accounts []account.AccountWithRole,
) (authz.DeploymentVisibility, error) {
	if resolver == nil || !resolver.Active() {
		return authz.DeploymentVisibility{}, nil
	}
	visibility, err := resolver.Visible(ctx, userID, accounts)
	if err != nil {
		return authz.DeploymentVisibility{}, fmt.Errorf("%w: %w", errDeploymentVisibilityUnavailable, err)
	}
	return visibility, nil
}

func singleAccountVisibilityScope(acct *account.Account) []account.AccountWithRole {
	return []account.AccountWithRole{{
		ID:                   acct.ID,
		Name:                 acct.Name,
		Type:                 acct.Type,
		WorkOSOrganizationID: acct.WorkOSOrganizationID,
	}}
}

func writeDeploymentVisibilityError(c *gin.Context, log *logger.Logger, err error) {
	log.Warn("Failed to resolve deployment visibility", "error", err)
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authorization temporarily unavailable"})
}

type readableDeploymentIDs func(context.Context, string, []string, []string, []string) ([]string, error)

func filterReadableDeployments[T any](
	ctx context.Context,
	listReadableIDs readableDeploymentIDs,
	userID string,
	items []T,
	visibility authz.DeploymentVisibility,
	deploymentID func(T) string,
) ([]T, error) {
	if len(visibility.FGAAccountIDs) == 0 || len(items) == 0 {
		return items, nil
	}
	deploymentIDs := make([]string, 0, len(items))
	for _, item := range items {
		deploymentIDs = append(deploymentIDs, deploymentID(item))
	}
	ids, err := listReadableIDs(
		ctx,
		userID,
		deploymentIDs,
		visibility.FGAAccountIDs,
		visibility.ReadableDeploymentIDs,
	)
	if err != nil {
		return nil, err
	}
	readable := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		readable[id] = struct{}{}
	}
	filtered := items[:0]
	for _, item := range items {
		if _, ok := readable[deploymentID(item)]; ok {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}
