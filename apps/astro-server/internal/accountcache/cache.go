package accountcache

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/insightscache"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
)

type DeploymentStore interface {
	GetActiveDeploymentsByAccount(accountID string) ([]*deploymentstore.Deployment, error)
}

type InvalidateResult struct {
	AccountsBusted    int
	DeploymentsBusted int
}

// InvalidateAccount mirrors Queen's account-cache invalidation surface:
// the agents-page payload, Insights endpoint cache, and per-deployment
// observability summaries for active deployments.
func InvalidateAccount(ctx context.Context, cache k8scache.Cache, deployStore DeploymentStore, accountID string) (InvalidateResult, error) {
	if accountID == "" {
		return InvalidateResult{}, fmt.Errorf("account_id is required")
	}

	_ = deploycache.Invalidate(ctx, cache, accountID)
	insightscache.InvalidateAccount(ctx, cache, accountID)

	if deployStore == nil {
		return InvalidateResult{AccountsBusted: 1}, nil
	}
	deps, err := deployStore.GetActiveDeploymentsByAccount(accountID)
	if err != nil {
		return InvalidateResult{AccountsBusted: 1}, fmt.Errorf("list active deployments: %w", err)
	}
	for _, d := range deps {
		_ = obssummary.Delete(ctx, cache, d.ID)
	}

	return InvalidateResult{
		AccountsBusted:    1,
		DeploymentsBusted: len(deps),
	}, nil
}
