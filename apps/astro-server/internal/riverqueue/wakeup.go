package riverqueue

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// WakeUpArgs are the job arguments for the wakeup worker.
type WakeUpArgs struct {
	DeploymentID string `json:"deployment_id"`
	ClusterID    string `json:"cluster_id,omitempty"`
}

func (WakeUpArgs) Kind() string { return "deployment.wakeup" }

func init() {
	registerJobKind[WakeUpArgs]()
}

func (WakeUpArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       queueDeploy,
		MaxAttempts: 3,
	}
}

// WakeUpWorker re-provisions K8s resources for a paused (stopped) deployment.
type WakeUpWorker struct {
	river.WorkerDefaults[WakeUpArgs]
	deployer *deployer.Deployer
	store    *deploymentstore.Store
	log      *logger.Logger
	cache    k8scache.Cache
}

func (w *WakeUpWorker) Work(ctx context.Context, job *river.Job[WakeUpArgs]) error {
	if w.deployer == nil {
		return fmt.Errorf("wakeup worker: K8s client not configured")
	}
	dep, err := w.store.GetDeploymentByID(job.Args.DeploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil || (dep.Status != deploymentstore.StatusStopped && dep.Status != deploymentstore.StatusPending) {
		w.log.Info("wakeup: skipped, status is not stopped/pending",
			"deployment_id", job.Args.DeploymentID,
			"status", statusOrNil(dep),
		)
		return nil
	}

	if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusProvisioning}); err != nil {
		return fmt.Errorf("set provisioning: %w", err)
	}

	if _, err := w.deployer.Apply(ctx, dep); err != nil {
		if sErr := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusFailed, ErrorMsg: err.Error()}); sErr != nil {
			w.log.Warn("wakeup: update deployment status failed", "error", sErr, "deployment_id", dep.ID, "status", deploymentstore.StatusFailed)
		}
		return fmt.Errorf("wakeup apply failed: %w", err)
	}
	k8scache.InvalidateNamespace(ctx, w.cache, dep.Namespace)

	// Manifests re-applied. The deployment controller drives deploying →
	// active/failed from observed health and resumes compute billing on the
	// real active transition (as for a fresh deploy).
	if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusDeploying}); err != nil {
		return fmt.Errorf("set deploying: %w", err)
	}

	return nil
}
