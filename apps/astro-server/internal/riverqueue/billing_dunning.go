package riverqueue

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
)

// dunningSweepLimit caps accounts processed per tick.
const dunningSweepLimit = 500

// DunningSweepArgs are the args for the billing dunning-grace sweep.
type DunningSweepArgs struct{}

func (DunningSweepArgs) Kind() string { return "billing.dunning_sweep" }

func (DunningSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueBilling}
}

func init() {
	registerJobKind[DunningSweepArgs]()
}

// dunningQueue is the sweep's output: stopping the workloads and telling the
// owner. Narrowed from *Queue so a test can observe the enqueue, which is the
// part of this worker worth proving; the recompute only decides that it happens.
type dunningQueue interface {
	InsertBillingSuspend(ctx context.Context, accountID string) error
	InsertBillingResume(ctx context.Context, accountID string) error
	EmitBillingNotify(ctx context.Context, ev notify.Event) error
}

// DunningSweepWorker ages past_due accounts to suspended once their dunning
// grace window elapses. Pure timer — it makes no Metronome calls; it only
// re-runs the state machine against the stored dunning_since.
type DunningSweepWorker struct {
	river.WorkerDefaults[DunningSweepArgs]
	status   *billing.StatusStore
	queue    dunningQueue // set post-construction in New(); enqueues workload suspend
	accounts accountNamer
	log      *logger.Logger
}

// accountNamer resolves an account id to its name. *account.AccountStore
// satisfies it. Leave the field unset rather than assigning a nil *AccountStore,
// which would store a non-nil interface holding a nil pointer and panic on the
// first call.
type accountNamer interface {
	GetByID(id string) (*account.Account, error)
}

// notifyAccountName resolves the name a suspension notification shows, returning
// "" when it cannot. A suspension is worth reporting unnamed, so a miss logs and
// carries on rather than holding the notification back.
func notifyAccountName(namer accountNamer, log *logger.Logger, accountID string) string {
	if namer == nil {
		return ""
	}
	acct, err := namer.GetByID(accountID)
	if err != nil || acct == nil {
		if err != nil {
			log.Warn("billing sweep: account name lookup failed", "account_id", accountID, "error", err)
		}
		return ""
	}
	return acct.Name
}

func (w *DunningSweepWorker) Work(ctx context.Context, _ *river.Job[DunningSweepArgs]) error {
	if w.status == nil {
		return nil
	}
	ids, err := w.status.ListForRecompute(ctx, dunningSweepLimit)
	if err != nil {
		return err
	}
	now := time.Now()
	var suspended int
	for _, id := range ids {
		st, changed, rerr := w.status.Recompute(ctx, id, now)
		if rerr != nil {
			w.log.Error("dunning sweep: recompute failed", "account_id", id, "error", rerr)
			continue
		}
		if changed && st == billing.StatusSuspended {
			suspended++
			if w.queue != nil {
				if err := w.queue.InsertBillingSuspend(ctx, id); err != nil {
					w.log.Error("dunning sweep: enqueue suspend failed", "account_id", id, "error", err)
				}
				// Notify the owner their account was suspended (best-effort).
				if err := w.queue.EmitBillingNotify(ctx, notify.BillingSuspended(id, notifyAccountName(w.accounts, w.log, id))); err != nil {
					w.log.Warn("dunning sweep: emit suspended notification failed", "account_id", id, "error", err)
				}
			}
		}
	}
	if len(ids) > 0 {
		w.log.Info("billing dunning: sweep", "evaluated", len(ids), "suspended", suspended)
	}
	w.reconcileExempt(ctx, now)
	return nil
}

// reconcileExempt returns exempt accounts to active and resumes whatever an
// earlier suspension stopped.
//
// The sweep above only scans past_due rows, so an account already suspended when
// it became exempt is never re-evaluated. Its reads report active while its
// deployments stay stopped, which is the one failure the exemption exists to
// prevent. Running every sweep rather than once at startup makes it self-healing
// whatever order the exemption and the suspension arrived in.
func (w *DunningSweepWorker) reconcileExempt(ctx context.Context, now time.Time) {
	for _, id := range w.status.ExemptAccountIDs() {
		st, _, err := w.status.Recompute(ctx, id, now)
		if err != nil {
			w.log.Error("billing exempt reconcile: recompute failed", "account_id", id, "error", err)
			continue
		}
		if st != billing.StatusActive || w.queue == nil {
			continue
		}
		// Unconditional, not only on a transition: a resume touches suspended
		// deployments and nothing else, so it repairs a row that already read
		// active while its workloads were still stopped.
		if err := w.queue.InsertBillingResume(ctx, id); err != nil {
			w.log.Error("billing exempt reconcile: enqueue resume failed", "account_id", id, "error", err)
		}
	}
}
