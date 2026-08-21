package riverqueue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldataset"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// DeployArgs are the job arguments for the deploy worker.
type DeployArgs struct {
	DeploymentID string `json:"deployment_id"`
	ClusterID    string `json:"cluster_id,omitempty"`
}

func (DeployArgs) Kind() string { return "deployment.deploy" }

func init() {
	registerJobKind[DeployArgs]()
}

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
	deployer        *deployer.Deployer
	store           *deploymentstore.Store
	datasetStore    *evaldatasetstore.Store
	langfuseStore   *langfuse.Store
	langfuseBaseURL string
	log             *logger.Logger
	cache           k8scache.Cache
	reconcile       func(namespace string)
}

func (w *DeployWorker) Work(ctx context.Context, job *river.Job[DeployArgs]) error {
	if w.deployer == nil {
		return fmt.Errorf("deploy worker: K8s client not configured")
	}
	if job.Args.ClusterID == "" {
		w.log.Info("deploy worker: primary cluster routing", "deployment_id", job.Args.DeploymentID)
	}
	dep, err := w.store.GetDeploymentByID(job.Args.DeploymentID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	if dep == nil || dep.Status != deploymentstore.StatusPending {
		w.log.Info("deploy: skipped, not in pending status",
			"deployment_id", job.Args.DeploymentID,
			"status", statusOrNil(dep),
		)
		return nil
	}

	if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusProvisioning}); err != nil {
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
		if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusFailed, ErrorMsg: applyErr.Error(), ErrorDetails: errDetails}); err != nil {
			w.log.Warn("deploy: update deployment status failed", "error", err, "deployment_id", dep.ID, "status", deploymentstore.StatusFailed)
		}
		// Status change → deploy cache for the account is stale.
		_ = deploycache.Invalidate(ctx, w.cache, dep.AccountID)
		return fmt.Errorf("deploy failed: %w", applyErr)
	}
	k8scache.InvalidateNamespace(ctx, w.cache, dep.Namespace)

	if len(result.Errors) > 0 {
		// Partial failure — some K8s resources failed. Mark failed with details.
		errJSON, jsonErr := json.Marshal(result.Errors)
		if jsonErr != nil {
			w.log.Warn("deploy: marshal error details failed", "error", jsonErr, "deployment_id", dep.ID)
		}
		if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusFailed, ErrorMsg: "partial failure", ErrorDetails: errJSON}); err != nil {
			w.log.Warn("deploy: update deployment status failed", "error", err, "deployment_id", dep.ID, "status", deploymentstore.StatusFailed, "reason", "partial failure")
		}
		_ = deploycache.Invalidate(ctx, w.cache, dep.AccountID)
		return nil // no retry — user needs to fix the spec
	}

	// Re-read status to guard against a concurrent undeploy requested during Apply.
	refreshed, err := w.store.GetDeploymentByID(dep.ID)
	if err != nil {
		return fmt.Errorf("re-read after apply: %w", err)
	}
	if refreshed == nil || refreshed.Status != deploymentstore.StatusProvisioning {
		w.log.Info("deploy: status changed during apply, skipping active transition",
			"deployment_id", dep.ID,
			"status", statusOrNil(refreshed),
		)
		return nil
	}
	if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusDeploying}); err != nil {
		return fmt.Errorf("set deploying: %w", err)
	}
	// Manifests applied. The deployment controller now drives deploying →
	// active/failed from observed workload health and starts compute billing on
	// the real active transition. Kick an immediate reconcile so a no-change
	// redeploy (no informer event) doesn't wait for the resync.
	if w.reconcile != nil {
		w.reconcile(dep.Namespace)
	}
	// Invalidate so the agents page reflects the deploying state immediately.
	_ = deploycache.Invalidate(ctx, w.cache, dep.AccountID)

	if w.langfuseStore != nil && w.datasetStore != nil {
		go w.provisionDataset(dep) //nolint:gosec
	}

	return nil
}

func (w *DeployWorker) provisionDataset(dep *deploymentstore.Deployment) {
	creds, err := w.langfuseStore.Get(dep.AccountID)
	if err != nil {
		w.log.Warn("deploy: load Langfuse credentials for dataset provisioning failed", "deployment_id", dep.ID, "account_id", dep.AccountID, "error", err)
		return
	}
	if creds == nil {
		return
	}
	client := langfuse.NewClient(w.langfuseBaseURL, creds.PublicKey, creds.SecretKey)
	if _, err := evaldataset.Ensure(context.Background(), w.datasetStore, client, evaldataset.EnsureOptions{
		DeploymentID: dep.ID,
		AccountID:    dep.AccountID,
		Description:  dep.AgentName,
	}); err != nil {
		w.log.Warn("deploy: provision dataset failed", "deployment_id", dep.ID, "error", err)
	}
}

func statusOrNil(dep *deploymentstore.Deployment) string {
	if dep == nil {
		return "<nil>"
	}
	return dep.Status
}
