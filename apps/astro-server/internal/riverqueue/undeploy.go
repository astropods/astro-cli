package riverqueue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/billing/metering"
	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// UndeployArgs are the job arguments for the undeploy worker.
type UndeployArgs struct {
	DeploymentID string `json:"deployment_id"`
	ClusterID    string `json:"cluster_id,omitempty"`
}

func (UndeployArgs) Kind() string { return "deployment.undeploy" }

func init() {
	registerJobKind[UndeployArgs]()
}

func (UndeployArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       queueDeploy,
		MaxAttempts: 3,
	}
}

// UndeployWorker tears down K8s resources for an undeploying deployment.
type UndeployWorker struct {
	river.WorkerDefaults[UndeployArgs]
	deployer  *deployer.Deployer
	store     *deploymentstore.Store
	accounts  *account.AccountStore
	resources authz.ResourceLifecycle
	ksStore   *knowledgestore.Store
	log       *logger.Logger
	cache     k8scache.Cache
	billing   *metering.BillingStateManager
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
		w.log.Info("undeploy: skipped, not in undeploying status",
			"deployment_id", job.Args.DeploymentID,
			"status", statusOrNil(dep),
		)
		return nil
	}

	// Clean up knowledge store bindings before teardown.
	if w.ksStore != nil {
		if err := w.ksStore.DeleteBindingsForDeployment(ctx, dep.ID); err != nil {
			w.log.Warn("undeploy: delete knowledge store bindings failed", "error", err, "deployment_id", dep.ID)
		}
	}

	if err := w.deployer.Teardown(ctx, dep); err != nil {
		if errors.Is(err, deployer.ErrClusterClientUnavailable) {
			w.log.Warn("undeploy: cluster client unavailable, skipping K8s teardown",
				"deployment_id", dep.ID,
				"cluster_id", dep.EffectiveClusterID(),
				"error", err,
			)
		} else {
			if sErr := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusFailed, ErrorMsg: "undeploy failed: " + err.Error()}); sErr != nil {
				w.log.Warn("undeploy: update deployment status failed", "error", sErr, "deployment_id", dep.ID, "status", deploymentstore.StatusFailed)
			}
			_ = deploycache.Invalidate(ctx, w.cache, dep.AccountID)
			return fmt.Errorf("teardown failed: %w", err)
		}
	}
	k8scache.InvalidateNamespace(ctx, w.cache, dep.Namespace)

	if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusUndeployed}); err != nil {
		return fmt.Errorf("set undeployed: %w", err)
	}
	if w.resources != nil && w.accounts != nil {
		acct, accountErr := w.accounts.GetByID(dep.AccountID)
		if accountErr != nil {
			w.log.Warn("undeploy: load account for authorization cleanup failed", "error", accountErr, "account_id", dep.AccountID, "deployment_id", dep.ID)
		} else if acct.Type == "organization" && acct.WorkOSOrganizationID != "" {
			resourceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			deleteErr := w.resources.DeleteResource(resourceCtx, acct.WorkOSOrganizationID, authz.DeploymentResource(dep.ID))
			cancel()
			if deleteErr != nil && !errors.Is(deleteErr, authz.ErrResourceNotFound) {
				w.log.Warn("undeploy: delete authorization resource failed", "error", deleteErr, "account_id", dep.AccountID, "deployment_id", dep.ID)
			}
		}
	}
	// Undeploy → the deployment drops out of the visible list for this account.
	_ = deploycache.Invalidate(ctx, w.cache, dep.AccountID)

	// Record billing stop — heartbeat emits the final CU-hours on its next tick.
	if w.billing != nil {
		go w.billing.StopBilling(context.Background(), dep.ID, time.Now()) //nolint:gosec // intentional: context.Background() avoids cancellation on job completion
	}

	return nil
}
