package clusterplacement

import (
	"context"
	"errors"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
)

// MigratableStatuses are deployment statuses eligible for cross-cluster migration.
var MigratableStatuses = []string{
	deploymentstore.StatusActive,
	deploymentstore.StatusFailed,
	deploymentstore.StatusPending,
}

// DeployJobEnqueuer enqueues a deploy worker job after routing is updated.
type DeployJobEnqueuer interface {
	InsertDeployJob(ctx context.Context, deploymentID, clusterID string) error
}

// Migrator orchestrates teardown on the source cluster, routing updates, and redeploy.
type Migrator struct {
	Deployer *deployer.Deployer
	Store    *deploymentstore.Store
	Queue    DeployJobEnqueuer
	Cache    k8scache.Cache
}

// MigrateInput identifies one deployment cluster move.
type MigrateInput struct {
	DeploymentID    string
	TargetClusterID string
	SourceClusterID string // expected current routing; empty skips the guard
	EventMessage    string // recorded before status → pending; defaults to account migration text
}

func isMigratableStatus(status string) bool {
	for _, s := range MigratableStatuses {
		if status == s {
			return true
		}
	}
	return false
}

// ListDeploymentsNeedingMigration returns migratable deployments whose routing
// cluster differs from the target account cluster.
func ListDeploymentsNeedingMigration(store *deploymentstore.Store, accountID, targetClusterID string) ([]*deploymentstore.Deployment, error) {
	deps, err := store.GetDeploymentsByAccountInStatuses(accountID, MigratableStatuses...)
	if err != nil {
		return nil, err
	}
	var out []*deploymentstore.Deployment
	for _, d := range deps {
		if PlacementMismatch(targetClusterID, d.EffectiveClusterID()) {
			out = append(out, d)
		}
	}
	return out, nil
}

func (m *Migrator) finishDeployEnqueue(ctx context.Context, dep *deploymentstore.Deployment, targetClusterID string) (bool, error) {
	if err := m.Queue.InsertDeployJob(ctx, dep.ID, targetClusterID); err != nil {
		return false, fmt.Errorf("enqueue deploy job: %w", err)
	}
	_ = deploycache.Invalidate(ctx, m.Cache, dep.AccountID)
	return false, nil
}

// MigrateDeployment tears down on the source cluster, updates routing, and enqueues deploy.
// Returns skipped=true when the deployment no longer needs migration.
func (m *Migrator) MigrateDeployment(ctx context.Context, in MigrateInput) (skipped bool, err error) {
	if m == nil || m.Store == nil {
		return false, fmt.Errorf("clusterplacement: migrator not configured")
	}
	if in.DeploymentID == "" {
		return false, fmt.Errorf("clusterplacement: deployment_id is required")
	}
	if m.Queue == nil {
		return false, fmt.Errorf("clusterplacement: queue not configured")
	}

	dep, err := m.Store.GetDeploymentByID(in.DeploymentID)
	if err != nil {
		return false, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil || !isMigratableStatus(dep.Status) {
		return true, nil
	}

	sourceClusterID := dep.EffectiveClusterID()
	if !PlacementMismatch(in.TargetClusterID, sourceClusterID) {
		if dep.Status == deploymentstore.StatusPending {
			return m.finishDeployEnqueue(ctx, dep, in.TargetClusterID)
		}
		return true, nil
	}
	if in.SourceClusterID != "" && NormalizedClusterID(sourceClusterID) != NormalizedClusterID(in.SourceClusterID) {
		return true, nil
	}

	if m.Deployer == nil {
		return false, fmt.Errorf("clusterplacement: deployer not configured")
	}

	if err := m.Deployer.TeardownOnCluster(ctx, dep, sourceClusterID); err != nil {
		if !errors.Is(err, deployer.ErrClusterClientUnavailable) {
			return false, fmt.Errorf("teardown source cluster: %w", err)
		}
	}

	patchedSpec, err := PatchDeploymentSpecClusterID(dep.DeploymentSpecJSON, in.TargetClusterID)
	if err != nil {
		return false, err
	}

	eventMsg := in.EventMessage
	if eventMsg == "" {
		eventMsg = AccountMigrationEventMessage(sourceClusterID, in.TargetClusterID)
	}

	if err := m.Store.ApplyClusterMigration(deploymentstore.ClusterMigrationParams{
		DeploymentID:    dep.ID,
		TargetClusterID: in.TargetClusterID,
		PatchedSpecJSON: patchedSpec,
		PriorStatus:     dep.Status,
		EventMessage:    eventMsg,
	}); err != nil {
		if errors.Is(err, deploymentstore.ErrClusterMigrationStatusChanged) {
			return true, nil
		}
		return false, fmt.Errorf("apply cluster migration: %w", err)
	}

	return m.finishDeployEnqueue(ctx, dep, in.TargetClusterID)
}
