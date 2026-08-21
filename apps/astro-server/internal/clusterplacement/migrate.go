package clusterplacement

import (
	"context"
	"errors"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
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
	Clusters clusterid.Resolver
}

// MigrateInput identifies one deployment cluster move.
type MigrateInput struct {
	DeploymentID    string
	TargetClusterID string
	SourceClusterID string // expected current routing; empty skips the guard
	EventMessage    string // recorded before status → pending; defaults to account migration text
}

type MigrateOutcome string

const (
	MigrateApplied         MigrateOutcome = "applied"
	MigrateAlreadyOnTarget MigrateOutcome = "already_on_target"
	MigrateNotMigratable   MigrateOutcome = "not_migratable"
	MigrateSourceMoved     MigrateOutcome = "source_moved"
)

type MigrateResult struct {
	Outcome  MigrateOutcome
	Enqueued bool
}

func isMigratableStatus(status string) bool {
	for _, s := range MigratableStatuses {
		if status == s {
			return true
		}
	}
	return false
}

func (m *Migrator) enqueueDeploy(ctx context.Context, dep *deploymentstore.Deployment, targetClusterID string) error {
	if err := m.Queue.InsertDeployJob(ctx, dep.ID, targetClusterID); err != nil {
		return fmt.Errorf("enqueue deploy job: %w", err)
	}
	_ = deploycache.Invalidate(ctx, m.Cache, dep.AccountID)
	return nil
}

// MigrateDeployment tears down on the source cluster, updates routing, and enqueues deploy.
func (m *Migrator) MigrateDeployment(ctx context.Context, in MigrateInput) (MigrateResult, error) {
	if m == nil || m.Store == nil {
		return MigrateResult{}, fmt.Errorf("clusterplacement: migrator not configured")
	}
	if in.DeploymentID == "" {
		return MigrateResult{}, fmt.Errorf("clusterplacement: deployment_id is required")
	}
	if m.Queue == nil {
		return MigrateResult{}, fmt.Errorf("clusterplacement: queue not configured")
	}

	dep, err := m.Store.GetDeploymentByID(in.DeploymentID)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil || !isMigratableStatus(dep.Status) {
		return MigrateResult{Outcome: MigrateNotMigratable}, nil
	}

	sourceClusterID := dep.EffectiveClusterID()
	if m.Clusters.Same(in.TargetClusterID, sourceClusterID) {
		res := MigrateResult{Outcome: MigrateAlreadyOnTarget}
		if dep.Status != deploymentstore.StatusPending {
			return res, nil
		}
		if err := m.enqueueDeploy(ctx, dep, in.TargetClusterID); err != nil {
			return res, err
		}
		res.Enqueued = true
		return res, nil
	}
	if in.SourceClusterID != "" && !m.Clusters.Same(sourceClusterID, in.SourceClusterID) {
		res := MigrateResult{Outcome: MigrateSourceMoved}
		if dep.Status != deploymentstore.StatusPending {
			return res, nil
		}
		patchedSpec, err := PatchDeploymentSpecClusterID(dep.DeploymentSpecJSON, sourceClusterID, m.Clusters)
		if err != nil {
			return res, err
		}
		if err := m.Store.ApplyClusterMigration(deploymentstore.ClusterMigrationParams{
			DeploymentID:    dep.ID,
			TargetClusterID: sourceClusterID,
			PatchedSpecJSON: patchedSpec,
			PriorStatus:     dep.Status,
			EventMessage:    "Cluster migration superseded, deploying to " + m.Clusters.Label(sourceClusterID),
		}); err != nil {
			return res, fmt.Errorf("record superseded migration: %w", err)
		}
		if err := m.enqueueDeploy(ctx, dep, sourceClusterID); err != nil {
			return res, err
		}
		res.Enqueued = true
		return res, nil
	}

	if m.Deployer == nil {
		return MigrateResult{}, fmt.Errorf("clusterplacement: deployer not configured")
	}

	if err := m.Deployer.TeardownOnCluster(ctx, dep, sourceClusterID); err != nil {
		if !errors.Is(err, deployer.ErrClusterClientUnavailable) {
			return MigrateResult{}, fmt.Errorf("teardown source cluster: %w", err)
		}
	}

	patchedSpec, err := PatchDeploymentSpecClusterID(dep.DeploymentSpecJSON, in.TargetClusterID, m.Clusters)
	if err != nil {
		return MigrateResult{}, err
	}

	eventMsg := in.EventMessage
	if eventMsg == "" {
		eventMsg = AccountMigrationEventMessage(sourceClusterID, in.TargetClusterID, m.Clusters)
	}

	if err := m.Store.ApplyClusterMigration(deploymentstore.ClusterMigrationParams{
		DeploymentID:    dep.ID,
		TargetClusterID: in.TargetClusterID,
		PatchedSpecJSON: patchedSpec,
		PriorStatus:     dep.Status,
		EventMessage:    eventMsg,
	}); err != nil {
		return MigrateResult{}, fmt.Errorf("apply cluster migration: %w", err)
	}

	if err := m.enqueueDeploy(ctx, dep, in.TargetClusterID); err != nil {
		return MigrateResult{Outcome: MigrateApplied}, err
	}
	return MigrateResult{Outcome: MigrateApplied, Enqueued: true}, nil
}
