package riverqueue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// InsightsRollupProducer is the contract for rolling one account-day of
// upstream telemetry into insights_usage_daily. main wires it to the handlers
// implementation, which owns the Langfuse query helpers and the dev-tool
// adapter registry. An interface here keeps the handlers package out of
// riverqueue's import graph.
//
// nil → the roll-up workers become no-ops, so a deployment without the producer
// wired behaves exactly as it did before this existed.
type InsightsRollupProducer interface {
	RollUpDay(ctx context.Context, accountID string, day time.Time) error
}

// InsightsRollupArgs is the discovery half: a daily periodic job that enumerates
// accounts and enqueues one per-account roll-up. Per-account work runs in
// parallel and retries independently, so one unhealthy account can't stall the
// rest.
type InsightsRollupArgs struct {
	// Force skips the per-account dedup window. The scheduled tick leaves it
	// unset so a restart can't double the day's work, but a manual trigger from
	// the admin console must set it — otherwise the enqueue is collapsed into the
	// day's already-completed run and the operator watches a job succeed having
	// done nothing at all.
	Force bool `json:"force,omitempty"`
	// Reconcile re-rolls the whole retention window for every account instead of
	// the trailing days, ignoring watermarks. For use after an upstream fix, and
	// deliberately opt-in: it costs 90 days of queries per account.
	Reconcile bool `json:"reconcile,omitempty"`
}

func (InsightsRollupArgs) Kind() string { return "insights.rollup" }

func (InsightsRollupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueInsights}
}

// InsightsRollupAccountArgs is the per-account fan-out job.
type InsightsRollupAccountArgs struct {
	AccountID string `json:"account_id"`
	// Reconcile ignores the watermark and re-reads the full retention window.
	// A normal run trusts the watermark and re-rolls only the trailing days,
	// which is precisely what would skip history an upstream fix has changed.
	Reconcile bool `json:"reconcile,omitempty"`
}

func (InsightsRollupAccountArgs) Kind() string { return "insights.rollup_account" }

func (InsightsRollupAccountArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueInsights}
}

func init() {
	registerJobKind[InsightsRollupArgs]()
	registerJobKind[InsightsRollupAccountArgs]()
}

// InsightsRollupWorker enumerates accounts with Langfuse provisioned and
// enqueues a per-account roll-up. Goes through langfuseStore.ListAccountIDs for
// the same reason the v1 refresh worker does: accounts that *previously* had
// deployments still hold history worth keeping current.
type InsightsRollupWorker struct {
	river.WorkerDefaults[InsightsRollupArgs]
	queue         *Queue
	langfuseStore *langfuse.Store
	log           *logger.Logger
}

func (w *InsightsRollupWorker) Work(ctx context.Context, job *river.Job[InsightsRollupArgs]) error {
	if w.queue == nil || w.langfuseStore == nil {
		w.log.Debug("Insights rollup skipped: queue or langfuse store not wired")
		return nil
	}

	accountIDs, err := w.langfuseStore.ListAccountIDs()
	if err != nil {
		// Cheap DB query — bubble up so River retries rather than waiting a full
		// day for the next tick after a transient blip.
		w.log.Error("Insights rollup: list accounts", "error", err)
		return fmt.Errorf("list account IDs: %w", err)
	}

	opts := &river.InsertOpts{}
	if !job.Args.Force {
		// Collapse duplicate enqueues within the day, so a restart or a second
		// discovery tick doesn't re-run every account.
		opts.UniqueOpts = river.UniqueOpts{
			ByArgs:   true,
			ByPeriod: insightsrollup.RollupInterval,
		}
	}

	var enqueued, enqueueErrs int
	for _, accountID := range accountIDs {
		if _, ierr := w.queue.Insert(ctx,
			InsightsRollupAccountArgs{AccountID: accountID, Reconcile: job.Args.Reconcile},
			opts,
		); ierr != nil {
			w.log.Warn("Insights rollup: enqueue per-account job",
				"account_id", accountID, "error", ierr)
			enqueueErrs++
			continue
		}
		enqueued++
	}

	w.log.Info("Insights rollup discovery completed",
		"enqueued", enqueued, "enqueue_errors", enqueueErrs,
		"forced", job.Args.Force, "reconcile", job.Args.Reconcile)
	return nil
}

// InsightsRollupAccountWorker rolls up one account: every complete day from the
// watermark (minus the trailing re-roll window) through yesterday.
//
// The watermark advances only after every day behind it has committed, so it can
// never claim coverage the facts don't support. A failure holds it in place and
// is recorded — a stalled watermark is a visible state that surfaces to the page
// as `as_of`, rather than a silently stale cache entry.
type InsightsRollupAccountWorker struct {
	river.WorkerDefaults[InsightsRollupAccountArgs]
	producer InsightsRollupProducer
	rollups  *insightsrollup.Store
	log      *logger.Logger
}

func (w *InsightsRollupAccountWorker) Work(ctx context.Context, job *river.Job[InsightsRollupAccountArgs]) error {
	if w.producer == nil || w.rollups == nil {
		w.log.Debug("Insights rollup skipped: producer or store not wired")
		return nil
	}
	accountID := job.Args.AccountID
	if accountID == "" {
		// Malformed args — retrying would fail identically.
		return nil
	}

	state, err := w.rollups.State(ctx, accountID, insightsrollup.SourceAgents)
	if err != nil {
		return fmt.Errorf("read rollup state for %s: %w", accountID, err)
	}

	if job.Args.Reconcile {
		// Drop the watermark for this run only — nothing is written back, so a
		// failure mid-reconcile leaves the stored watermark untouched rather than
		// rewound.
		state = insightsrollup.State{}
	}

	days := insightsrollup.DaysToRoll(state, time.Now())
	if len(days) == 0 {
		return nil
	}

	for _, day := range days {
		if err := w.producer.RollUpDay(ctx, accountID, day); err != nil {
			if errors.Is(err, insightsrollup.ErrAccountGone) {
				// Deleted after discovery enqueued this. Return before touching the
				// state table: there is no coverage to claim, and after a hard delete
				// the row the watermark and the failure would go into is gone.
				w.log.Info("Insights rollup: account deleted; skipping",
					"account_id", accountID)
				return nil
			}
			// Stop at the first failure rather than pressing on: advancing past a
			// gap would leave a hole no later tick revisits, because the
			// watermark would claim the day was done.
			w.log.Warn("Insights rollup: day failed, holding watermark",
				"account_id", accountID, "day", day.Format(time.DateOnly), "error", err)
			if rerr := w.rollups.RecordFailure(ctx, accountID, insightsrollup.SourceAgents, err.Error()); rerr != nil {
				w.log.Warn("Insights rollup: record failure", "account_id", accountID, "error", rerr)
			}
			// Rejected credentials fail identically on every attempt, so spending
			// River's retry budget on them just repeats the same call with
			// backoff. Succeed without retrying and let the daily tick re-check —
			// the same reasoning the v1 refresh worker applies to a sustained
			// outage. The watermark still holds, so no coverage is claimed, and
			// consecutive_errors keeps climbing until someone fixes the account.
			if isUpstreamAuthFailure(err) {
				w.log.Error("Insights rollup: upstream rejected the account's credentials; no data will be rolled up until they are fixed",
					"account_id", accountID, "error", err)
				return nil
			}
			return fmt.Errorf("roll up %s for %s: %w", day.Format(time.DateOnly), accountID, err)
		}
	}

	lastComplete := days[len(days)-1]
	if err := w.rollups.Advance(ctx, accountID, insightsrollup.SourceAgents, lastComplete); err != nil {
		return fmt.Errorf("advance watermark for %s: %w", accountID, err)
	}

	w.log.Info("Insights rollup completed",
		"account_id", accountID, "days", len(days),
		"through", lastComplete.Format(time.DateOnly))
	return nil
}

func isUpstreamAuthFailure(err error) bool {
	return langfuse.IsAuthFailure(err)
}
