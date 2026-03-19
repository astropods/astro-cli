package riverqueue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// DeployArgs are the job arguments for the deploy worker.
type DeployArgs struct {
	DeploymentID string `json:"deployment_id"`
}

func (DeployArgs) Kind() string { return "deploy" }

func (DeployArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       queueDeploy,
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			// Exclude completed/discarded from uniqueness check so that
			// re-apply, rollback, and reconciler re-enqueue can create new
			// jobs after the original deploy job finishes.
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

// DeployWorker provisions K8s resources for a pending deployment.
type DeployWorker struct {
	river.WorkerDefaults[DeployArgs]
	deployer *deployer.Deployer
	store    *deploymentstore.Store
	log      *logger.Logger
}

func (w *DeployWorker) Work(ctx context.Context, job *river.Job[DeployArgs]) error {
	if w.deployer == nil {
		return fmt.Errorf("deploy worker: K8s client not configured")
	}
	dep, err := w.store.GetDeploymentByID(job.Args.DeploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil || dep.Status != deploymentstore.StatusPending {
		w.log.Info("Deploy skipped: not in pending status",
			"deployment_id", job.Args.DeploymentID,
			"status", statusOrNil(dep),
		)
		return nil
	}

	if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusProvisioning, "", nil); err != nil {
		return fmt.Errorf("set provisioning: %w", err)
	}

	result, applyErr := w.deployer.Apply(ctx, dep)
	if applyErr != nil {
		// Total failure. Do NOT teardown — old pods may still be running in the
		// same namespace (redeploy case). Mark failed and let River retry.
		errDetails, jsonErr := json.Marshal([]map[string]string{{"error": applyErr.Error()}})
		if jsonErr != nil {
			errDetails = nil
		}
		if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusFailed, applyErr.Error(), errDetails); err != nil {
			w.log.Warn("Failed to mark deployment as failed", "error", err, "deployment_id", dep.ID)
		}
		return fmt.Errorf("deploy failed: %w", applyErr)
	}

	if len(result.Errors) > 0 {
		// Partial failure — some K8s resources failed. Mark failed with details.
		errJSON, jsonErr := json.Marshal(result.Errors)
		if jsonErr != nil {
			w.log.Warn("Failed to marshal error details", "error", jsonErr, "deployment_id", dep.ID)
		}
		if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusFailed, "partial failure", errJSON); err != nil {
			w.log.Warn("Failed to mark deployment as partially failed", "error", err, "deployment_id", dep.ID)
		}
		return nil // no retry — user needs to fix the spec
	}

	if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusActive, "", nil); err != nil {
		return fmt.Errorf("set active: %w", err)
	}
	return nil
}

func statusOrNil(dep *deploymentstore.Deployment) string {
	if dep == nil {
		return "<nil>"
	}
	return dep.Status
}
