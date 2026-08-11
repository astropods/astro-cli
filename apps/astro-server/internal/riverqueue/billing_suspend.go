package riverqueue

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
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
	store  *deploymentstore.Store
	status *billing.StatusStore // reads the gating reason for the event; optional
	reg    *k8s.Registry
	cache  k8scache.Cache
	log    *logger.Logger
}

// reasonUnknown is recorded when the gating reason cannot be read. Readers pick
// their copy from this code, so an unobserved reason must not be guessed at.
const reasonUnknown = "unknown"

// suspendReason resolves the account's gating reason once per job, so every
// deployment in the job records the same snapshot.
func (w *BillingSuspendWorker) suspendReason(ctx context.Context, accountID string) string {
	if w.status == nil {
		return reasonUnknown
	}
	_, reason, err := w.status.Get(ctx, accountID)
	if err != nil || reason == "" {
		return reasonUnknown
	}
	return reason
}

// suspendEvent describes the stop on the deployment's timeline. Readers branch
// on the reason code; the message is display copy.
func suspendEvent(reason string) deploymentstore.StatusUpdate {
	msg := "Stopped by billing"
	if reason == billing.ReasonCreditsExhausted {
		msg = "Stopped: free credits used up and no payment method on file"
	}
	return deploymentstore.StatusUpdate{
		Status:       deploymentstore.StatusSuspended,
		EventMsg:     msg,
		EventDetails: billingEventDetails(reason),
	}
}

// billingEventDetails marshals rather than concatenates so a reason can never
// break out of the JSON string.
func billingEventDetails(reason string) json.RawMessage {
	b, err := json.Marshal(map[string]string{"source": "billing", "reason": reason})
	if err != nil {
		return nil
	}
	return b
}

func (w *BillingSuspendWorker) Work(ctx context.Context, job *river.Job[BillingSuspendArgs]) error {
	deps, err := w.store.GetActiveDeploymentsByAccount(job.Args.AccountID)
	if err != nil {
		return err
	}
	event := suspendEvent(w.suspendReason(ctx, job.Args.AccountID))
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
		if err := w.store.UpdateStatus(dep.ID, event); err != nil {
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
		if err := w.store.UpdateStatus(dep.ID, deploymentstore.StatusUpdate{
			Status:       deploymentstore.StatusPending,
			EventMsg:     "Restarting after billing was resolved",
			EventDetails: billingEventDetails("resumed"),
		}); err != nil {
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
