package riverqueue

import (
	"context"
	"time"

	"github.com/riverqueue/river"

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
	EmitBillingNotify(ctx context.Context, ev notify.Event) error
}

// DunningSweepWorker ages past_due accounts to suspended once their dunning
// grace window elapses. Pure timer — it makes no Metronome calls; it only
// re-runs the state machine against the stored dunning_since.
type DunningSweepWorker struct {
	river.WorkerDefaults[DunningSweepArgs]
	status *billing.StatusStore
	queue  dunningQueue // set post-construction in New(); enqueues workload suspend
	log    *logger.Logger
}

func (w *DunningSweepWorker) Work(ctx context.Context, _ *river.Job[DunningSweepArgs]) error {
	if w.status == nil {
		return nil
	}
	ids, err := w.status.ListInDunning(ctx, dunningSweepLimit)
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
				if err := w.queue.EmitBillingNotify(ctx, notify.BillingSuspended(id, "")); err != nil {
					w.log.Warn("dunning sweep: emit suspended notification failed", "account_id", id, "error", err)
				}
			}
		}
	}
	if len(ids) > 0 {
		w.log.Info("billing dunning sweep", "evaluated", len(ids), "suspended", suspended)
	}
	return nil
}
