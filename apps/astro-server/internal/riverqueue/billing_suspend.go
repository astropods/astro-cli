package riverqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"k8s.io/client-go/kubernetes"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
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

	// Scaling deployments to zero stops only the traffic that starts in the
	// cluster. Dev keys and the judge key answer from anywhere until revoked.
	aigwProvisioner *aigateway.Provisioner
	aigwDevStore    *aigateway.DevStore
	aigwJudgeStore  *aigateway.JudgeStore

	// Nil selects the real scale-to-zero; only a test without a cluster sets it.
	stopWorkloads func(context.Context, kubernetes.Interface, string) error
}

func (w *BillingSuspendWorker) stopNamespace(ctx context.Context, cs kubernetes.Interface, ns string) error {
	if w.stopWorkloads != nil {
		return w.stopWorkloads(ctx, cs, ns)
	}
	return k8s.StopNamespaceWorkloads(ctx, cs, ns)
}

// reasonUnknown is recorded when the gating reason cannot be read. Readers pick
// their copy from this code, so an unobserved reason must not be guessed at.
const reasonUnknown = "unknown"

// A cluster that rejects one namespace usually rejects the next, so this waits.
const suspendRetryDelay = time.Minute

// retry keeps the job alive. River discards a plain error after MaxAttempts, and
// a discarded suspension leaves the account spending; a snooze raises the
// ceiling with it.
func (w *BillingSuspendWorker) retry(accountID, stage string, err error) error {
	w.log.Error("billing suspend: retrying", "account_id", accountID, "stage", stage, "error", err)
	return river.JobSnooze(suspendRetryDelay)
}

// The status decides whether to act; the reason is the snapshot every
// deployment in the job records.
func (w *BillingSuspendWorker) suspendState(ctx context.Context, accountID string) (billing.Status, string, error) {
	if w.status == nil {
		return billing.StatusSuspended, reasonUnknown, nil
	}
	status, reason, err := w.status.Get(ctx, accountID)
	if err != nil {
		return "", "", fmt.Errorf("read billing status: %w", err)
	}
	if reason == "" {
		reason = reasonUnknown
	}
	return status, reason, nil
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
	// A retry can land after the account paid. Recovery already fired its resume,
	// so suspending now would stop deployments that nothing brings back.
	status, reason, err := w.suspendState(ctx, job.Args.AccountID)
	if err != nil {
		return w.retry(job.Args.AccountID, "read_status", err)
	}
	if status != billing.StatusSuspended {
		w.log.Info("billing suspend: skipped, account not suspended", "account_id", job.Args.AccountID, "status", string(status))
		return nil
	}

	deps, err := w.store.GetActiveDeploymentsByAccount(job.Args.AccountID)
	if err != nil {
		return w.retry(job.Args.AccountID, "list_deployments", err)
	}
	event := suspendEvent(reason)
	var suspended int
	var failed []error
	for _, dep := range deps {
		client, err := suspendClusterClient(ctx, w.reg, dep)
		if err != nil {
			w.log.Error("billing suspend: cluster client", "deployment_id", dep.ID, "error", err)
			failed = append(failed, fmt.Errorf("deployment %s: cluster client: %w", dep.ID, err))
			continue
		}
		if err := w.stopNamespace(ctx, client.Clientset(), dep.Namespace); err != nil {
			w.log.Error("billing suspend: stop workloads", "deployment_id", dep.ID, "error", err)
			failed = append(failed, fmt.Errorf("deployment %s: stop workloads: %w", dep.ID, err))
			continue
		}
		k8scache.InvalidateNamespace(ctx, w.cache, dep.Namespace)
		if err := w.store.UpdateStatus(dep.ID, event); err != nil {
			w.log.Error("billing suspend: update status", "deployment_id", dep.ID, "error", err)
			failed = append(failed, fmt.Errorf("deployment %s: update status: %w", dep.ID, err))
			continue
		}
		suspended++
	}
	w.revokeGatewayKeys(ctx, job.Args.AccountID)
	w.log.Info("billing suspend: completed", "account_id", job.Args.AccountID, "suspended", suspended, "total", len(deps))
	if len(failed) == 0 {
		return nil
	}
	return w.retry(job.Args.AccountID, "stop_deployments", errors.Join(failed...))
}

// revokeGatewayKeys cuts the spend paths that survive scaling deployments to
// zero. Both kinds are re-minted on demand, so resume needs no counterpart.
// Per-deployment keys stay: their workloads are already stopped, and the value
// lives in a tenant Secret that resume re-applies rather than re-mints.
// Best-effort, so a gateway outage cannot undo a suspension already applied.
func (w *BillingSuspendWorker) revokeGatewayKeys(ctx context.Context, accountID string) {
	if w.aigwProvisioner == nil {
		return
	}
	if w.aigwDevStore != nil {
		if err := w.aigwProvisioner.RevokeAccountDevKeys(ctx, w.aigwDevStore, accountID); err != nil {
			w.log.Error("billing suspend: revoke dev keys", "account_id", accountID, "error", err)
		}
	}
	if w.aigwJudgeStore != nil {
		if err := w.aigwProvisioner.RevokeAccountJudgeKeys(ctx, w.aigwJudgeStore, accountID); err != nil {
			w.log.Error("billing suspend: revoke judge key", "account_id", accountID, "error", err)
		}
	}
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

// Narrowed from *Queue so a test can observe the enqueue without a River client.
type resumeQueue interface {
	InsertWakeUpJob(ctx context.Context, deploymentID, clusterID string) error
}

// BillingResumeWorker re-provisions deployments that billing suspended, via the
// existing wakeup path (status→pending, then wakeup re-applies manifests).
type BillingResumeWorker struct {
	river.WorkerDefaults[BillingResumeArgs]
	store *deploymentstore.Store
	queue resumeQueue
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
	w.log.Info("billing suspend: billing resume", "account_id", job.Args.AccountID, "resumed", resumed, "total", len(deps))
	return nil
}
