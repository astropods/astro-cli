package riverqueue

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// Per-status deadlines for the stuck-deployment watchdog. Each bounds how long a
// deployment may sit in a non-terminal in-progress status before the watchdog
// flips it to failed.
//
// deploying is the loosest: K8s progressDeadlineSeconds (180s) already fails
// Deployment-kind rollouts fast, so this only backstops the kinds K8s does not
// bound (StatefulSet/Job pods that never schedule, PVCs that never bind).
// provisioning and pending are held only by the deploy worker; exceeding them
// means the worker died mid-apply (a River retry no-ops once status left
// pending) or never ran (job lost / worker down), so a tighter bound is safe.
const (
	deployingDeadline    = 30 * time.Minute
	provisioningDeadline = 15 * time.Minute
	pendingDeadline      = 15 * time.Minute
)

// staleStatusRule pairs an in-progress status with its deadline and the message
// recorded on the failed deployment + its event.
type staleStatusRule struct {
	status   string
	deadline time.Duration
	errMsg   string
}

var staleStatusRules = []staleStatusRule{
	{deploymentstore.StatusDeploying, deployingDeadline, "deployment exceeded 30m in deploying without becoming healthy"},
	{deploymentstore.StatusProvisioning, provisioningDeadline, "deployment exceeded 15m in provisioning (deploy worker likely died mid-apply)"},
	{deploymentstore.StatusPending, pendingDeadline, "deployment exceeded 15m in pending (deploy job never started)"},
}

// DeploymentWatchdogArgs are the job arguments for the stuck-deployment sweep.
type DeploymentWatchdogArgs struct{}

func (DeploymentWatchdogArgs) Kind() string { return "deployment.staleness_sweep" }

func (DeploymentWatchdogArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMaintenance}
}

func init() {
	registerJobKind[DeploymentWatchdogArgs]()
}

// DeploymentWatchdogWorker fails deployments wedged in a non-terminal in-progress
// status past its deadline. failed is drivable (not terminal): the deploy
// controller drives failed→active once workloads later observe healthy, so this
// is a soft flip that unsticks the UI without foreclosing recovery, and it never
// tears down resources (running pods are left alone, matching the apply-failure
// path).
type DeploymentWatchdogWorker struct {
	river.WorkerDefaults[DeploymentWatchdogArgs]
	store *deploymentstore.Store
	cache k8scache.Cache
	log   *logger.Logger
}

func (w *DeploymentWatchdogWorker) Work(ctx context.Context, _ *river.Job[DeploymentWatchdogArgs]) error {
	for _, rule := range staleStatusRules {
		ids, err := w.store.FailStaleDeployments(rule.status, rule.deadline, rule.errMsg)
		if err != nil {
			// Log and continue — one status's failure shouldn't block the others.
			w.log.Error("deployment watchdog: sweep failed", "status", rule.status, "error", err)
			continue
		}
		for _, id := range ids {
			w.log.Warn("deployment watchdog: failed stuck deployment",
				"deployment_id", id, "prior_status", rule.status, "deadline", rule.deadline.String())
			// Status change → the account's deploy cache is stale. Best-effort:
			// the deployment row is already updated, so a cache miss self-heals.
			if dep, derr := w.store.GetDeploymentByID(id); derr == nil && dep != nil {
				_ = deploycache.Invalidate(ctx, w.cache, dep.AccountID)
			}
		}
	}
	return nil
}
