package riverqueue

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
	"github.com/astropods/astro/apps/astro-server/internal/clusterplacement"
	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// MigrateDeploymentClusterArgs are the job arguments for cross-cluster migration.
type MigrateDeploymentClusterArgs struct {
	DeploymentID    string `json:"deployment_id"`
	TargetClusterID string `json:"target_cluster_id"`
	SourceClusterID string `json:"source_cluster_id,omitempty"`
}

func (MigrateDeploymentClusterArgs) Kind() string { return "deployment.migrate_cluster" }

func clusterResolver(cfg Config) clusterid.Resolver {
	if cfg.ServerConfig == nil {
		return clusterid.Resolver{}
	}
	return clusterid.New(cfg.ServerConfig.Deployment.DefaultClusterID)
}

func init() {
	registerJobKind[MigrateDeploymentClusterArgs]()
}

func (MigrateDeploymentClusterArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       queueDeploy,
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
	}
}

// MigrateDeploymentClusterWorker moves a deployment from source to target cluster.
type MigrateDeploymentClusterWorker struct {
	river.WorkerDefaults[MigrateDeploymentClusterArgs]
	deployer *deployer.Deployer
	store    *deploymentstore.Store
	queue    *Queue
	log      *logger.Logger
	cache    k8scache.Cache
	clusters clusterid.Resolver
}

func (w *MigrateDeploymentClusterWorker) Work(ctx context.Context, job *river.Job[MigrateDeploymentClusterArgs]) error {
	if w.store == nil {
		return fmt.Errorf("migrate cluster worker: store not configured")
	}
	m := &clusterplacement.Migrator{
		Deployer: w.deployer,
		Store:    w.store,
		Queue:    w.queue,
		Cache:    w.cache,
		Clusters: w.clusters,
	}
	res, err := m.MigrateDeployment(ctx, clusterplacement.MigrateInput{
		DeploymentID:    job.Args.DeploymentID,
		TargetClusterID: job.Args.TargetClusterID,
		SourceClusterID: job.Args.SourceClusterID,
	})
	if err != nil {
		return err
	}
	w.log.Info("Migrate cluster finished",
		"deployment_id", job.Args.DeploymentID,
		"target_cluster_id", job.Args.TargetClusterID,
		"outcome", res.Outcome,
		"deploy_enqueued", res.Enqueued,
	)
	return nil
}
