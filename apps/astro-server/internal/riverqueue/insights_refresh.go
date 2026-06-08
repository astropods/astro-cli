package riverqueue

import (
	"context"
	"errors"
	"fmt"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/insightscache"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// InsightsRefreshArgs is the discovery half of the refresh pipeline: a
// periodic job that enumerates accounts and enqueues one
// InsightsRefreshAccountArgs per account into River. Per-account work runs
// in parallel and retries independently — one slow or unhealthy account
// can't stall the rest.
type InsightsRefreshArgs struct{}

func (InsightsRefreshArgs) Kind() string { return "insights.refresh" }

func init() {
	registerJobKind[InsightsRefreshArgs]()
	registerJobKind[InsightsRefreshAccountArgs]()
}

// InsightsRefreshAccountArgs is the per-account fan-out job. River
// schedules these in parallel and applies its own retry/backoff on
// transient errors. ErrAllLangfuseCallsFailed is treated as "preserve
// cache, don't retry" — the next periodic discovery tick will pick the
// account up again, and during a sustained Langfuse outage repeated
// retries every few seconds would just waste capacity.
type InsightsRefreshAccountArgs struct {
	AccountID string `json:"account_id"`
}

func (InsightsRefreshAccountArgs) Kind() string { return "insights.refresh_account" }

// InsightsRefreshWorker is the discovery worker. It enumerates accounts
// that have Langfuse provisioned and enqueues one per-account refresh job.
// Goes through langfuseStore.ListAccountIDs rather than the deployments
// table so accounts that *previously* had deployments (and may still have
// a cached snapshot) keep getting their cache surfaced or cleared.
type InsightsRefreshWorker struct {
	river.WorkerDefaults[InsightsRefreshArgs]
	queue         *Queue
	langfuseStore *langfuse.Store
	log           *logger.Logger
}

func (w *InsightsRefreshWorker) Work(ctx context.Context, _ *river.Job[InsightsRefreshArgs]) error {
	if w.queue == nil {
		w.log.Debug("Insights refresh skipped: no queue wired")
		return nil
	}
	if w.langfuseStore == nil {
		w.log.Debug("Insights refresh skipped: no langfuse store wired")
		return nil
	}

	accountIDs, err := w.langfuseStore.ListAccountIDs()
	if err != nil {
		// Bubble up so River retries with backoff — this is a cheap DB query,
		// and waiting 6h for the next periodic tick after a transient DB
		// blip would leave the cache stale for everyone.
		w.log.Error("Insights refresh: list accounts", "error", err)
		return fmt.Errorf("list account IDs: %w", err)
	}
	if len(accountIDs) == 0 {
		return nil
	}

	var enqueued, enqueueErrs int
	for _, accountID := range accountIDs {
		// Deduplicate within the discovery window: if a per-account job for
		// this account is already pending/running, River dedupes it via the
		// UniqueOpts. Without this, two near-simultaneous discovery ticks
		// (e.g. after a restart) would double up.
		_, ierr := w.queue.Insert(ctx,
			InsightsRefreshAccountArgs{AccountID: accountID},
			&river.InsertOpts{
				UniqueOpts: river.UniqueOpts{
					ByArgs:   true,
					ByPeriod: insightscache.RefreshInterval,
				},
			},
		)
		if ierr != nil {
			w.log.Warn("Insights refresh: enqueue per-account job",
				"account_id", accountID, "error", ierr)
			enqueueErrs++
			continue
		}
		enqueued++
	}

	w.log.Info("Insights refresh discovery completed",
		"enqueued", enqueued, "enqueue_errors", enqueueErrs,
	)
	return nil
}

// InsightsRefreshAccountWorker handles a single account's refresh. River
// runs these in parallel up to the queue's concurrency cap. Retries on
// transient errors; deliberately *succeeds without retry* on the
// ErrAllLangfuseCallsFailed signal — the cron's next discovery cycle is
// the right retry granularity for a sustained upstream outage.
type InsightsRefreshAccountWorker struct {
	river.WorkerDefaults[InsightsRefreshAccountArgs]
	cache    k8scache.Cache
	computer InsightsSummaryComputer
	log      *logger.Logger
}

// fetchForVariant maps each canonical Variant to the computer call that
// produces its bytes. Kept local to this package because the computer
// type is local; the (endpoint, params) tuples themselves come from
// insightscache.WarmedVariants so the writer and invalidator never drift.
//
// Adding a new variant: append to insightscache.WarmedVariants AND wire
// up a fetch case here. The init() below fails fast if the worker can't
// produce bytes for every declared variant.
var fetchForVariant = map[insightscache.Variant]func(ctx context.Context, c InsightsSummaryComputer, accountID string) ([]byte, error){
	{Endpoint: insightscache.EndpointSummary, Params: insightscache.Params{GroupBy: "user", IncludeArchived: false}}: func(ctx context.Context, c InsightsSummaryComputer, accountID string) ([]byte, error) {
		return c.ComputeSummary(ctx, accountID, "user", false)
	},
	{Endpoint: insightscache.EndpointDeploymentsSummary, Params: insightscache.Params{IncludeArchived: false}}: func(ctx context.Context, c InsightsSummaryComputer, accountID string) ([]byte, error) {
		return c.ComputeDeploymentsSummary(ctx, accountID, false)
	},
	{Endpoint: insightscache.EndpointUsersSummary, Params: insightscache.Params{}}: func(ctx context.Context, c InsightsSummaryComputer, accountID string) ([]byte, error) {
		return c.ComputeUsersSummary(ctx, accountID)
	},
}

func init() {
	// Refuse to start if a declared variant has no fetcher — that would
	// silently skip an endpoint at runtime.
	for _, v := range insightscache.WarmedVariants {
		if _, ok := fetchForVariant[v]; !ok {
			panic(fmt.Sprintf("insights_refresh: no fetch fn for variant %+v", v))
		}
	}
}

func (w *InsightsRefreshAccountWorker) Work(ctx context.Context, job *river.Job[InsightsRefreshAccountArgs]) error {
	if w.cache == nil {
		w.log.Debug("Insights account refresh skipped: no Redis cache configured")
		return nil
	}
	if w.computer == nil {
		w.log.Debug("Insights account refresh skipped: no summary computer injected")
		return nil
	}

	accountID := job.Args.AccountID
	if accountID == "" {
		// Malformed args — don't retry, would just fail the same way.
		return nil
	}

	// Track transient errors separately from "Langfuse fully down" so
	// we can ask River to retry the former but not the latter.
	var transient error
	for _, v := range insightscache.WarmedVariants {
		fetch := fetchForVariant[v] // init() guarantees non-nil
		data, err := fetch(ctx, w.computer, accountID)
		switch {
		case errors.Is(err, insightscache.ErrAllLangfuseCallsFailed):
			// Skip the cache write to preserve the previous value, and
			// don't escalate to River — sustained outages get refreshed
			// by the next discovery tick, not by retry storms.
			w.log.Warn("Insights account refresh: upstream unavailable, preserving cache",
				"account_id", accountID, "endpoint", string(v.Endpoint))
			continue
		case err != nil:
			w.log.Warn("Insights account refresh: compute failed",
				"account_id", accountID, "endpoint", string(v.Endpoint), "error", err)
			transient = fmt.Errorf("compute %s for %s: %w", v.Endpoint, accountID, err)
			continue
		}
		if perr := insightscache.Put(ctx, w.cache, accountID, v.Endpoint, v.Params, data); perr != nil {
			w.log.Warn("Insights account refresh: cache write failed",
				"account_id", accountID, "endpoint", string(v.Endpoint), "error", perr)
			transient = fmt.Errorf("cache write %s for %s: %w", v.Endpoint, accountID, perr)
			continue
		}
	}
	// Returning non-nil triggers River's per-job retry/backoff. Returning
	// nil after a Langfuse outage is intentional — we lean on the
	// periodic discovery cron, not River retries, to re-attempt.
	return transient
}
