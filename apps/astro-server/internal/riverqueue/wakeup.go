package riverqueue

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
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
	cache    k8scache.Cache
	billing  *openmeter.BillingStateManager
}

func (w *WakeUpWorker) Work(ctx context.Context, job *river.Job[WakeUpArgs]) error {
	if w.deployer == nil {
		return fmt.Errorf("wakeup worker: K8s client not configured")
	}
	dep, err := w.store.GetDeploymentByID(job.Args.DeploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil || (dep.Status != deploymentstore.StatusScaledDown && dep.Status != deploymentstore.StatusStopped && dep.Status != deploymentstore.StatusPending) {
		w.log.Info("Wakeup skipped: status is not scaled_down/stopped/pending",
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
	k8scache.InvalidateNamespace(ctx, w.cache, dep.Namespace)

	if err := w.store.ClearScaledDown(dep.Namespace); err != nil {
		w.log.Warn("Failed to clear scaled-down state", "error", err, "namespace", dep.Namespace)
	}

	if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusActive, "", nil); err != nil {
		return fmt.Errorf("set active: %w", err)
	}

	// Resume event-driven compute billing after wakeup.
	if w.billing != nil {
		workloads, err := workloadInfoFromStore(w.store, dep.ID)
		if err != nil {
			w.log.Error("Failed to load workloads for billing, heartbeat will recover", "error", err, "deployment_id", dep.ID)
		} else {
			go w.billing.StartBilling(context.Background(), dep.ID, workloads) //nolint:gosec // intentional: context.Background() avoids cancellation on job completion
		}
	}

	return nil
}
