package riverqueue

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/riverqueue/river"
	"golang.org/x/sync/errgroup"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
)

// ObsSummaryRefreshArgs are the job arguments for the obs summary refresh
// worker. The job is parameterless — every tick processes every active
// deployment across every account.
type ObsSummaryRefreshArgs struct{}

func (ObsSummaryRefreshArgs) Kind() string { return "obs.summary_refresh" }

func init() {
	registerJobKind[ObsSummaryRefreshArgs]()
}

// langfuseSummaryClient is the subset of *langfuse.Client this worker needs.
// Defined as an interface so tests can substitute a fake.
type langfuseSummaryClient interface {
	GetTraces(ctx context.Context, deploymentID, startTime, endTime string, limit, offset int) (*langfuse.TracesResponse, error)
	GetDailyMetrics(ctx context.Context, deploymentID, startTime, endTime string) ([]langfuse.DailyMetric, error)
}

// langfuseClientFactory builds a per-account Langfuse client from creds.
// Production wires defaultLangfuseClientFactory; tests inject a fake.
type langfuseClientFactory func(baseURL, publicKey, secretKey string) langfuseSummaryClient

func defaultLangfuseClientFactory(baseURL, publicKey, secretKey string) langfuseSummaryClient {
	return langfuse.NewClient(baseURL, publicKey, secretKey)
}

// ObsSummaryRefreshWorker periodically refreshes the observability summary
// cache for every active deployment. The agents page reads the resulting
// Redis entries directly — it never calls Langfuse on the request path —
// so the cache is the SLO for how fast cards render.
type ObsSummaryRefreshWorker struct {
	river.WorkerDefaults[ObsSummaryRefreshArgs]
	cfg             *config.Config
	db              *sql.DB
	cache           k8scache.Cache
	deploymentStore *deploymentstore.Store
	langfuseStore   *langfuse.Store
	log             *logger.Logger
	// clientFactory is overridable in tests. Nil → defaultLangfuseClientFactory.
	clientFactory langfuseClientFactory
}

func (w *ObsSummaryRefreshWorker) Work(ctx context.Context, _ *river.Job[ObsSummaryRefreshArgs]) error {
	if w.cache == nil {
		w.log.Debug("Obs summary refresh skipped: no Redis cache configured")
		return nil
	}

	deployments, err := w.deploymentStore.ListAllActive()
	if err != nil {
		w.log.Error("Obs summary refresh: list active deployments", "error", err)
		return nil // transient; don't wedge the periodic job
	}
	if len(deployments) == 0 {
		return nil
	}

	// Bucket by account so we can fetch creds once per account and then fan
	// out per deployment. Accounts without Langfuse provisioned are skipped
	// entirely (no creds → no metrics).
	byAccount := make(map[string][]*deploymentstore.DeploymentWithAccount, 16)
	for _, d := range deployments {
		byAccount[d.AccountID] = append(byAccount[d.AccountID], d)
	}

	startTime, endTime, dates := refreshWindow()
	factory := w.clientFactory
	if factory == nil {
		factory = defaultLangfuseClientFactory
	}

	var attempted, skippedAccts int
	var failed atomic.Int32
	for accountID, deps := range byAccount {
		creds, credsErr := w.langfuseStore.Get(accountID)
		if credsErr != nil {
			w.log.Warn("Obs summary refresh: load Langfuse creds", "account_id", accountID, "error", credsErr)
			skippedAccts++
			continue
		}
		if creds == nil {
			// Account hasn't been provisioned for Langfuse — no metrics to
			// fetch, nothing to refresh.
			skippedAccts++
			continue
		}
		client := factory(w.cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

		var g errgroup.Group
		g.SetLimit(10)
		for _, d := range deps {
			id := d.ID
			g.Go(func() error {
				if err := w.refreshOne(ctx, client, id, startTime, endTime, dates); err != nil {
					w.log.Warn("Obs summary refresh: refresh deployment", "deployment_id", id, "error", err)
					failed.Add(1)
					return nil // individual failures are non-fatal
				}
				return nil
			})
		}
		_ = g.Wait()
		attempted += len(deps)
	}

	failedN := int(failed.Load())
	w.log.Info("Obs summary refresh completed",
		"refreshed", attempted-failedN,
		"failed", failedN,
		"skipped_accounts", skippedAccts,
	)
	return nil
}

// refreshOne fetches the two Langfuse summaries for a single deployment and
// writes the resulting entry to Redis. Errors are returned (not logged) so
// the caller can decide how to surface them — typically as a Warn so one
// flaky deployment doesn't tank the run for the others.
func (w *ObsSummaryRefreshWorker) refreshOne(
	ctx context.Context,
	client langfuseSummaryClient,
	deploymentID, startTime, endTime string,
	dates []string,
) error {
	traces, err := client.GetTraces(ctx, deploymentID, "", "", 1, 0)
	if err != nil {
		return fmt.Errorf("get traces: %w", err)
	}
	var lastTraceAt string
	if len(traces.Data) > 0 {
		lastTraceAt = traces.Data[0].CreatedAt
	}

	requestSeries := make([]int, obssummary.DaysOfHistory)
	tokenSeries := make([]int, obssummary.DaysOfHistory)
	dailyMetrics, dmErr := client.GetDailyMetrics(ctx, deploymentID, startTime, endTime)
	if dmErr != nil {
		// Daily metrics failure shouldn't drop the whole entry — total_traces
		// + last_trace_at are still useful. Leave the series zero-padded.
		w.log.Warn("Obs summary refresh: daily metrics", "deployment_id", deploymentID, "error", dmErr)
	} else {
		byDate := make(map[string]langfuse.DailyMetric, len(dailyMetrics))
		for _, m := range dailyMetrics {
			byDate[m.Date] = m
		}
		for j, d := range dates {
			if m, ok := byDate[d]; ok {
				requestSeries[j] = m.CountTraces
				tokenSeries[j] = m.InputTokens() + m.OutputTokens()
			}
		}
	}

	entry := &obssummary.Entry{
		TotalTraces:   traces.Meta.TotalItems,
		LastTraceAt:   lastTraceAt,
		RequestSeries: requestSeries,
		TokenSeries:   tokenSeries,
		RefreshedAt:   time.Now().UTC(),
	}
	return obssummary.Put(ctx, w.cache, deploymentID, entry)
}

// refreshWindow returns the start/end RFC3339 timestamps and the per-day
// YYYY-MM-DD labels (oldest → newest, length DaysOfHistory) used by the
// worker to align Langfuse's response into a fixed-length series.
func refreshWindow() (string, string, []string) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -(obssummary.DaysOfHistory - 1)).Format(time.RFC3339)
	end := now.Format(time.RFC3339)
	dates := make([]string, obssummary.DaysOfHistory)
	for i := 0; i < obssummary.DaysOfHistory; i++ {
		dates[i] = now.AddDate(0, 0, -(obssummary.DaysOfHistory - 1 - i)).Format("2006-01-02")
	}
	return start, end, dates
}
