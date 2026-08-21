package admingrpc

import (
	"context"
	"fmt"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/clusterplacement"
	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

// SetAccountCluster assigns or clears the additional cluster placement for an
// account. It only updates the account row — existing deployments keep
// routing to whatever cluster they're already on. Use MigrateAccountDeployments
// to move them onto the new cluster, whenever that's wanted.
func (s *Server) SetAccountCluster(ctx context.Context, req *adminv1.SetAccountClusterRequest) (*adminv1.SetAccountClusterResponse, error) {
	if req.AccountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}

	clusterID := req.ClusterID
	if clusterID != "" {
		if s.clusterStore == nil {
			return nil, fmt.Errorf("cluster store not configured")
		}
		if _, err := s.clusterStore.Get(ctx, clusterID); err != nil {
			return nil, clusterStoreErr(err)
		}
	}

	acctStore := account.NewAccountStore(s.db)
	acct, err := acctStore.GetByID(req.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	if acct == nil {
		return nil, fmt.Errorf("account not found: %s", req.AccountID)
	}

	oldClusterID := ""
	if acct.ClusterID != nil {
		oldClusterID = *acct.ClusterID
	}

	if err := acctStore.SetClusterID(req.AccountID, clusterID); err != nil {
		return nil, err
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.AccountSetCluster
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		if clusterID == "" {
			evt.Description = "Admin cleared account cluster placement (primary)"
		} else {
			evt.Description = "Admin set account cluster placement to " + clusterID
		}
		evt.Metadata = map[string]any{
			"cluster_id":     clusterID,
			"old_cluster_id": oldClusterID,
		}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.SetAccountClusterResponse{
		Status:    "updated",
		ClusterID: clusterID,
	}, nil
}

// MigrateAccountDeployments enqueues migration jobs for the account's
// active/failed/pending deployments that still route to a cluster other than
// the account's current one. Safe to call independently of SetAccountCluster,
// and safe to retry if a prior call only partially enqueued.
func (s *Server) MigrateAccountDeployments(ctx context.Context, req *adminv1.MigrateAccountDeploymentsRequest) (*adminv1.MigrateAccountDeploymentsResponse, error) {
	if req.AccountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}
	if s.deployStore == nil {
		return nil, fmt.Errorf("deployment store not configured")
	}
	if s.queue == nil {
		return nil, fmt.Errorf("queue not configured")
	}

	acctStore := account.NewAccountStore(s.db)
	acct, err := acctStore.GetByID(req.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	if acct == nil {
		return nil, fmt.Errorf("account not found: %s", req.AccountID)
	}
	clusterID := ""
	if acct.ClusterID != nil {
		clusterID = *acct.ClusterID
	}

	toMigrate, err := clusterplacement.ListDeploymentsNeedingMigration(s.deployStore, req.AccountID, clusterID, s.clusters())
	if err != nil {
		return nil, fmt.Errorf("list deployments for migration: %w", err)
	}

	deploymentIDs, err := enqueueAccountClusterMigrations(ctx, s.queue, clusterID, toMigrate)
	if err != nil {
		return nil, err
	}

	if len(deploymentIDs) > 0 {
		_ = deploycache.Invalidate(ctx, s.cache, req.AccountID)
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin(req.AccountID, "grpc")
		evt.Action = auditlog.AccountSetCluster
		evt.ResourceType = "account"
		evt.ResourceID = req.AccountID
		evt.Description = "Admin migrated account deployments to " + clusterID
		evt.Metadata = map[string]any{
			"cluster_id":          clusterID,
			"migrations_enqueued": len(deploymentIDs),
			"deployment_ids":      deploymentIDs,
		}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.MigrateAccountDeploymentsResponse{
		MigrationsEnqueued: int32(len(deploymentIDs)), //nolint:gosec // bounded by account deployment count
		DeploymentIds:      deploymentIDs,
	}, nil
}

func enqueueAccountClusterMigrations(
	ctx context.Context,
	q adminJobQueue,
	targetClusterID string,
	deps []*deploymentstore.Deployment,
) ([]string, error) {
	deploymentIDs := make([]string, 0, len(deps))
	for _, dep := range deps {
		sourceClusterID := dep.EffectiveClusterID()
		if err := q.InsertMigrateDeploymentClusterJob(ctx, dep.ID, targetClusterID, sourceClusterID); err != nil {
			return deploymentIDs, fmt.Errorf(
				"enqueued %d/%d migration jobs before failure on deployment %s: %w",
				len(deploymentIDs), len(deps), dep.ID, err,
			)
		}
		deploymentIDs = append(deploymentIDs, dep.ID)
	}
	return deploymentIDs, nil
}
