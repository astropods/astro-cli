package admingrpc

import (
	"context"
	"fmt"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/accountcache"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
)

// InvalidateAccountCaches busts every per-account cache Queen's trash-can
// action owns: the agents-page deploy payload and the per-deployment obs summary
// cache for active deployments.
func (s *Server) InvalidateAccountCaches(ctx context.Context, req *adminv1.InvalidateAccountCachesRequest) (*adminv1.InvalidateCachesResponse, error) {
	if req.AccountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}

	result, err := accountcache.InvalidateAccount(ctx, s.cache, s.deployStore, req.AccountID)
	if err != nil {
		return nil, err
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.CacheInvalidateAccount
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		evt.Description = fmt.Sprintf("Admin invalidated agents-page caches for account (1 account, %d deployments)", result.DeploymentsBusted)
		evt.Metadata = map[string]any{"deployments_busted": result.DeploymentsBusted}
		s.auditStore.LogAsync(s.log, evt)
	}

	s.log.Info("admin cache: invalidated account caches",
		"account_id", req.AccountID,
		"deployments", result.DeploymentsBusted,
	)

	return &adminv1.InvalidateCachesResponse{
		AccountsBusted:    int32(result.AccountsBusted),    //nolint:gosec // bounded by DB rows
		DeploymentsBusted: int32(result.DeploymentsBusted), //nolint:gosec // bounded by DB rows
	}, nil
}

// InvalidateAllCaches busts every account's deploy cache plus every active
// deployment's obs summary cache. Failsafe: call it when something has gone
// systemically wrong with cached page data and SafetyTTL is too long to wait.
// Expensive at large scale; not for routine use.
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

	s.log.Warn("admin cache: invalidated all caches",
		"accounts", len(accountsResp.Accounts),
		"deployments", len(deps),
	)

	return &adminv1.InvalidateCachesResponse{
		AccountsBusted:    int32(len(accountsResp.Accounts)), //nolint:gosec // bounded by DB rows
		DeploymentsBusted: int32(len(deps)),                  //nolint:gosec // bounded by DB rows
	}, nil
}
