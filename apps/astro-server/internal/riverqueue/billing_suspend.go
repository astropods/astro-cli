package riverqueue

import (
	"context"
	"errors"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// BillingSuspendArgs scales an account's active deployments to zero when the
// account is billing-suspended.
type BillingSuspendArgs struct {
	AccountID string `json:"account_id"`
}

func (BillingSuspendArgs) Kind() string { return "billing.suspend" }

func (BillingSuspendArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueBilling}
}

// BillingResumeArgs restores an account's billing-suspended deployments on
// recovery.
type BillingResumeArgs struct {
	AccountID string `json:"account_id"`
}

func (BillingResumeArgs) Kind() string { return "billing.resume" }

func (BillingResumeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueBilling}
}

func init() {
	registerJobKind[BillingSuspendArgs]()
	registerJobKind[BillingResumeArgs]()
}

// BillingSuspendWorker scales every active deployment of an account to zero and
// marks it StatusSuspended (distinct from a user StatusStopped so resume only
// restores what billing stopped).
type BillingSuspendWorker struct {
	river.WorkerDefaults[BillingSuspendArgs]
	store *deploymentstore.Store
	reg   *k8s.Registry
	cache k8scache.Cache
	log   *logger.Logger
}

func (w *BillingSuspendWorker) Work(ctx context.Context, job *river.Job[BillingSuspendArgs]) error {
	deps, err := w.store.GetActiveDeploymentsByAccount(job.Args.AccountID)
	if err != nil {
		return err
	}
	var suspended int
	for _, dep := range deps {
		client, err := suspendClusterClient(ctx, w.reg, dep)
		if err != nil {
			w.log.Error("billing suspend: cluster client", "deployment_id", dep.ID, "error", err)
			continue
		}
		if err := k8s.StopNamespaceWorkloads(ctx, client.Clientset(), dep.Namespace); err != nil {
			w.log.Error("billing suspend: stop workloads", "deployment_id", dep.ID, "error", err)
			continue
		}
		k8scache.InvalidateNamespace(ctx, w.cache, dep.Namespace)
		if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusSuspended}); err != nil {
			w.log.Error("billing suspend: update status", "deployment_id", dep.ID, "error", err)
			continue
		}
		suspended++
	}
	w.log.Info("billing suspend", "account_id", job.Args.AccountID, "suspended", suspended, "total", len(deps))
	return nil
}

// suspendClusterClient resolves the client for dep. A row with no cluster_id
// lives on the primary cluster, the convention every other resolver follows
// (handlers.clusterClientForDeployment, deployer.clusterClientForKey,
// admingrpc.deploymentClusterClient). Registry.Get rejects an empty id, so
// the fallback is what makes suspension work at all: nearly every deployment
// row carries no cluster_id.
func suspendClusterClient(ctx context.Context, reg *k8s.Registry, dep *deploymentstore.Deployment) (k8s.ClusterClient, error) {
	if dep.EffectiveClusterID() == "" {
		if reg.Default() == nil {
			return nil, errors.New("kubernetes client not configured")
		}
		return reg.Default(), nil
	}
	return reg.Get(ctx, dep.EffectiveClusterID())
}

// BillingResumeWorker re-provisions deployments that billing suspended, via the
// existing wakeup path (status→pending, then wakeup re-applies manifests).
type BillingResumeWorker struct {
	river.WorkerDefaults[BillingResumeArgs]
	store *deploymentstore.Store
	queue *Queue
	log   *logger.Logger
}

func (w *BillingResumeWorker) Work(ctx context.Context, job *river.Job[BillingResumeArgs]) error {
	deps, err := w.store.GetDeploymentsByAccountInStatuses(job.Args.AccountID, deploymentstore.StatusSuspended)
	if err != nil {
		return err
	}
	var resumed int
	for _, dep := range deps {
		if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusPending}); err != nil {
			w.log.Error("billing resume: update status", "deployment_id", dep.ID, "error", err)
			continue
		}
		if err := w.queue.InsertWakeUpJob(ctx, dep.ID, dep.EffectiveClusterID()); err != nil {
			w.log.Error("billing resume: enqueue wakeup", "deployment_id", dep.ID, "error", err)
			continue
		}
		resumed++
	}
	w.log.Info("billing resume", "account_id", job.Args.AccountID, "resumed", resumed, "total", len(deps))
	return nil
}
