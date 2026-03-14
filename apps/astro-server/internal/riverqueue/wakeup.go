package riverqueue

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// WakeUpArgs are the job arguments for the wakeup worker.
type WakeUpArgs struct {
	DeploymentID string `json:"deployment_id"`
}

func (WakeUpArgs) Kind() string { return "wakeup" }

func (WakeUpArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       queueDeploy,
		MaxAttempts: 3,
	}
}

// WakeUpWorker re-provisions K8s resources for a KEDA-scaled-down deployment.
type WakeUpWorker struct {
	river.WorkerDefaults[WakeUpArgs]
	deployer *deployer.Deployer
	store    *deploymentstore.Store
	log      *logger.Logger
}

func (w *WakeUpWorker) Work(ctx context.Context, job *river.Job[WakeUpArgs]) error {
	if w.deployer == nil {
		return fmt.Errorf("wakeup worker: K8s client not configured")
	}
	dep, err := w.store.GetDeploymentByID(job.Args.DeploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil || dep.Status != deploymentstore.StatusScaledDown {
		w.log.Info("Wakeup skipped: not in scaled_down status",
			"deployment_id", job.Args.DeploymentID,
			"status", statusOrNil(dep),
		)
		return nil
	}

	if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusProvisioning, "", nil); err != nil {
		return fmt.Errorf("set provisioning: %w", err)
	}

	if _, err := w.deployer.Apply(ctx, dep); err != nil {
		if sErr := w.store.UpdateStatus(dep.ID, deploymentstore.StatusFailed, err.Error(), nil); sErr != nil {
			w.log.Warn("Failed to mark deployment as failed", "error", sErr, "deployment_id", dep.ID)
		}
		return fmt.Errorf("wakeup apply failed: %w", err)
	}

	if err := w.store.ClearScaledDown(dep.Namespace); err != nil {
		w.log.Warn("Failed to clear scaled-down state", "error", err, "namespace", dep.Namespace)
	}

	if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusActive, "", nil); err != nil {
		return fmt.Errorf("set active: %w", err)
	}

	return nil
}
