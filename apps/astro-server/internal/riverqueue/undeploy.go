package riverqueue

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
)

// UndeployArgs are the job arguments for the undeploy worker.
type UndeployArgs struct {
	DeploymentID string `json:"deployment_id"`
}

func (UndeployArgs) Kind() string { return "undeploy" }

func (UndeployArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       queueDeploy,
		MaxAttempts: 3,
	}
}

// UndeployWorker tears down K8s resources for an undeploying deployment.
type UndeployWorker struct {
	river.WorkerDefaults[UndeployArgs]
	deployer *deployer.Deployer
	store    *deploymentstore.Store
	ksStore  *knowledgestore.Store
	log      *logger.Logger
	cache    k8scache.Cache
	billing  *openmeter.BillingStateManager
}

func (w *UndeployWorker) Work(ctx context.Context, job *river.Job[UndeployArgs]) error {
	if w.deployer == nil {
		return fmt.Errorf("undeploy worker: K8s client not configured")
	}
	dep, err := w.store.GetDeploymentByID(job.Args.DeploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil || dep.Status != deploymentstore.StatusUndeploying {
		w.log.Info("Undeploy skipped: not in undeploying status",
			"deployment_id", job.Args.DeploymentID,
			"status", statusOrNil(dep),
		)
		return nil
	}

	// Clean up knowledge store bindings before teardown.
	if w.ksStore != nil {
		if err := w.ksStore.DeleteBindingsForDeployment(ctx, dep.ID); err != nil {
			w.log.Warn("Failed to delete knowledge store bindings", "error", err, "deployment_id", dep.ID)
		}
	}

	if err := w.deployer.Teardown(ctx, dep); err != nil {
		if sErr := w.store.UpdateStatus(dep.ID, deploymentstore.StatusFailed, "undeploy failed: "+err.Error(), nil); sErr != nil {
			w.log.Warn("Failed to mark deployment as failed", "error", sErr, "deployment_id", dep.ID)
		}
		return fmt.Errorf("teardown failed: %w", err)
	}
	k8scache.InvalidateNamespace(ctx, w.cache, dep.Namespace)

	if err := w.store.ClearScaledDown(dep.Namespace); err != nil {
		w.log.Warn("Failed to clear scaled-down state", "error", err, "namespace", dep.Namespace)
	}

	if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUndeployed, "", nil); err != nil {
		return fmt.Errorf("set undeployed: %w", err)
	}

	if err := w.store.MarkUndeployedByID(dep.ID); err != nil {
		w.log.Warn("Failed to set undeployed_at", "error", err, "deployment_id", dep.ID)
	}

	// Record billing stop — heartbeat emits the final CU-hours on its next tick.
	if w.billing != nil {
		go w.billing.StopBilling(context.Background(), dep.ID, time.Now()) //nolint:gosec // intentional: context.Background() avoids cancellation on job completion
	}

	return nil
}
