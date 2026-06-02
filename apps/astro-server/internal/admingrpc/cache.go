package admingrpc

import (
	"context"
	"fmt"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/insightscache"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
)

// InvalidateAccountCaches busts the agents-page deploy cache for one account
// plus the per-deployment obs summary cache for each of its active deployments.
// Used by queen as a manual escape hatch when an admin needs to clear a single
// account's cached payload without waiting on SafetyTTL.
func (s *Server) InvalidateAccountCaches(ctx context.Context, req *adminv1.InvalidateAccountCachesRequest) (*adminv1.InvalidateCachesResponse, error) {
	if req.AccountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}

	// Bust the per-account agents-page payload. Safe to call even when the
	// underlying cache is nil (NoopCache when REDIS_URL is unset).
	_ = deploycache.Invalidate(ctx, s.cache, req.AccountID)

	// Bust the Insights endpoint cache for this account. Forces the next
	// page-load to fall through to a live Langfuse fetch (which then
	// repopulates the cache on success) instead of waiting on the 6h cron.
	insightscache.InvalidateAccount(ctx, s.cache, req.AccountID)

	// Bust each active deployment's obs summary. We only know about active
	// rows here; undeployed entries are already cleared by the undeploy
	// worker, so iterating active deployments covers the live cache surface.
	deps, err := s.deployStore.GetActiveDeploymentsByAccount(req.AccountID)
	if err != nil {
		return nil, fmt.Errorf("list active deployments: %w", err)
	}
	for _, d := range deps {
		_ = obssummary.Delete(ctx, s.cache, d.ID)
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.CacheInvalidateAccount
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		evt.Description = fmt.Sprintf("Admin invalidated agents-page caches for account (1 account, %d deployments)", len(deps))
		evt.Metadata = map[string]any{"deployments_busted": len(deps)}
		s.auditStore.LogAsync(s.log, evt)
	}

	s.log.Info("Admin invalidated account caches",
		"account_id", req.AccountID,
		"deployments", len(deps),
	)

	return &adminv1.InvalidateCachesResponse{
		AccountsBusted:    1,
		DeploymentsBusted: int32(len(deps)), //nolint:gosec // bounded by DB rows
	}, nil
}

// InvalidateAllCaches busts every account's deploy cache + every active
// deployment's obs summary cache. Failsafe — call when something has gone
// systemically wrong with the agents-page caches and SafetyTTL is too long
// to wait. Expensive at large scale; not for routine use.
func (s *Server) InvalidateAllCaches(ctx context.Context, _ *adminv1.InvalidateAllCachesRequest) (*adminv1.InvalidateCachesResponse, error) {
	// List every account we know about. We pull from the existing ListAccounts
	// RPC so the bust set tracks whatever ListAccounts considers "an account
	// that could have a cached payload" (including soft-deleted, since their
	// rows linger in Redis until TTL).
	accountsResp, err := s.ListAccounts(ctx, &adminv1.ListAccountsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	for _, a := range accountsResp.Accounts {
		_ = deploycache.Invalidate(ctx, s.cache, a.ID)
		insightscache.InvalidateAccount(ctx, s.cache, a.ID)
	}

	// Iterate every active deployment for obs summary. ListAllActive is what
	// the periodic worker uses, so the bust set matches the populate set.
	deps, err := s.deployStore.ListAllActive()
	if err != nil {
		return nil, fmt.Errorf("list all active deployments: %w", err)
	}
	for _, d := range deps {
		_ = obssummary.Delete(ctx, s.cache, d.ID)
	}

	if s.auditStore != nil {
		// No account scope for "everyone" — log against empty account id; the
		// audit row still carries the actor (admin user) via ForAdmin.
		evt := auditlog.ForAdmin("", "grpc")
		evt.Action = auditlog.CacheInvalidateAll
		evt.ResourceType = "cache"
		evt.Description = fmt.Sprintf("Admin invalidated agents-page caches globally (%d accounts, %d deployments)", len(accountsResp.Accounts), len(deps))
		evt.Metadata = map[string]any{
			"accounts_busted":    len(accountsResp.Accounts),
			"deployments_busted": len(deps),
		}
		s.auditStore.LogAsync(s.log, evt)
	}

	s.log.Warn("Admin invalidated ALL caches",
		"accounts", len(accountsResp.Accounts),
		"deployments", len(deps),
	)

	return &adminv1.InvalidateCachesResponse{
		AccountsBusted:    int32(len(accountsResp.Accounts)), //nolint:gosec // bounded by DB rows
		DeploymentsBusted: int32(len(deps)),                  //nolint:gosec // bounded by DB rows
	}, nil
}
