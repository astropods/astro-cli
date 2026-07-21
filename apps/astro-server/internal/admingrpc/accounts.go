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

// SetAccountCluster assigns or clears the additional cluster placement for an account.
// When the cluster changes, enqueues migration jobs for active/failed/pending
// deployments that still route to the previous cluster, then updates the account row.
func (s *Server) SetAccountCluster(ctx context.Context, req *adminv1.SetAccountClusterRequest) (*adminv1.SetAccountClusterResponse, error) {
	if req.AccountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}

	clusterID := req.ClusterID
	if clusterID != "" {
		if s.clusterStore == nil {
			return nil, fmt.Errorf("cluster store not configured")
		}
		row, err := s.clusterStore.Get(ctx, clusterID)
		if err != nil {
			return nil, clusterStoreErr(err)
		}
		if !row.Enabled {
			return nil, fmt.Errorf("cluster %q is disabled", clusterID)
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

	clusterChanging := clusterplacement.NormalizedClusterID(oldClusterID) != clusterplacement.NormalizedClusterID(clusterID)

	var toMigrate []*deploymentstore.Deployment
	if clusterChanging {
		if s.deployStore == nil {
			return nil, fmt.Errorf("deployment store not configured; cannot migrate deployments after cluster change")
		}
		toMigrate, err = clusterplacement.ListDeploymentsNeedingMigration(s.deployStore, req.AccountID, clusterID)
		if err != nil {
			return nil, fmt.Errorf("list deployments for migration: %w", err)
		}
	}

	// River enqueues and SetClusterID are not one transaction. Workers read target/source
	// cluster from job args (not accounts.cluster_id), so jobs stay correct if SetClusterID
	// fails after enqueue; ops should retry SetAccountCluster or wait for in-flight jobs.
	var deploymentIDs []string
	if len(toMigrate) > 0 {
		if s.queue == nil {
			return nil, fmt.Errorf("queue not configured; cannot migrate deployments after cluster change")
		}
		deploymentIDs, err = enqueueAccountClusterMigrations(ctx, s.queue, clusterID, toMigrate)
		if err != nil {
			return nil, err
		}
	}

	if err := acctStore.SetClusterID(req.AccountID, clusterID); err != nil {
		if len(deploymentIDs) > 0 {
			return nil, fmt.Errorf(
				"enqueued %d migration job(s) but failed to update account cluster (account row unchanged; retry SetAccountCluster, avoid ReapplyDeployment until complete): %w",
				len(deploymentIDs), err,
			)
		}
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
		if clusterID == "" {
			evt.Description = "Admin cleared account cluster placement (primary)"
		} else {
			evt.Description = "Admin set account cluster placement to " + clusterID
		}
		evt.Metadata = map[string]any{
			"cluster_id":          clusterID,
			"old_cluster_id":      oldClusterID,
			"migrations_enqueued": len(deploymentIDs),
			"deployment_ids":      deploymentIDs,
		}
		s.auditStore.LogAsync(s.log, evt)
	}

	return &adminv1.SetAccountClusterResponse{
		Status:             "updated",
		ClusterID:          clusterID,
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
				"enqueued %d/%d migration jobs before failure on deployment %s (account cluster unchanged; retry SetAccountCluster, avoid ReapplyDeployment until complete): %w",
				len(deploymentIDs), len(deps), dep.ID, err,
			)
		}
		deploymentIDs = append(deploymentIDs, dep.ID)
	}
	return deploymentIDs, nil
}
