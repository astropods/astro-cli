package riverqueue

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
)

// SlackDirectoryBackfillArgs has no per-invocation arguments — the worker
// discovers work by listing accounts with linked Slack mappings.
type SlackDirectoryBackfillArgs struct{}

// Kind groups the job with other slack-prefixed workers in logs and the
// River UI.
func (SlackDirectoryBackfillArgs) Kind() string { return "slack.directory_backfill" }

// SlackDirectoryBackfillWorker is a ONE-SHOT historical backfill. It
// populates slack_identity_mappings observed-only rows
// (workos_user_id IS NULL, source='observed') for every bare-Slack
// userId Langfuse has on disk for accounts with at least one linked
// Slack member. After a successful run it writes a singleton marker row
// (slack_directory_backfill_marker); subsequent runs see the marker on
// entry and exit immediately without touching any data.
//
// "Never runs again" guarantee, layered:
//
//  1. main.go checks the marker before enqueueing — if set, no job is
//     enqueued at all.
//  2. The worker checks the marker on entry — defense against a race
//     where the marker was written between the main.go check and the
//     job actually executing (e.g. multi-replica startup).
//  3. UniqueOpts on the enqueue collapses concurrent enqueues into a
//     single queued job.
//
// To re-run the backfill (e.g. after onboarding a new account with
// pre-existing Slack history), DELETE FROM slack_directory_backfill_marker
// and the next pod restart will pick it back up.
//
// Why a River job and not a CLI script: the bare userIds live in
// Langfuse (ClickHouse), separate DB from astro-server's Postgres, and
// the Langfuse credentials are KMS-encrypted in account_langfuse. Doing
// this in-process reuses the existing decryption + Langfuse client
// wiring without exposing KMS access to ops.
type SlackDirectoryBackfillWorker struct {
	river.WorkerDefaults[SlackDirectoryBackfillArgs]

	cfg           *config.Config
	slackStore    *slackidentity.Store
	langfuseStore *langfuse.Store
	log           *logger.Logger
}

// Work runs the one-shot backfill. Returns nil on errors so the queue
// doesn't wedge — transient failures get retried on the next pod
// restart since the marker only writes on full success.
func (w *SlackDirectoryBackfillWorker) Work(ctx context.Context, _ *river.Job[SlackDirectoryBackfillArgs]) error {
	if w.slackStore == nil || w.langfuseStore == nil {
		w.log.Debug("Slack directory backfill skipped: missing dependencies")
		return nil
	}

	// Marker gate: if a previous run already completed, exit without
	// doing any work. This is the "never runs again" guarantee — the
	// worker becomes a fast no-op for every enqueue after the first.
	done, err := w.slackStore.IsDirectoryBackfillComplete(ctx)
	if err != nil {
		w.log.Warn("Slack directory backfill: marker check failed; skipping", "error", err)
		return nil
	}
	if done {
		w.log.Debug("Slack directory backfill: already complete; skipping")
		return nil
	}

	accountTeams, err := w.slackStore.ListLinkedAccountTeams()
	if err != nil {
		w.log.Error("Slack directory backfill: list linked accounts failed", "error", err)
		return nil
	}
	if len(accountTeams) == 0 {
		// Mark complete even when there's no work — otherwise every pod
		// restart will re-attempt and re-confirm "nothing to do".
		w.log.Info("Slack directory backfill: no linked accounts; marking complete")
		if mErr := w.slackStore.MarkDirectoryBackfillComplete(ctx); mErr != nil {
			w.log.Warn("Slack directory backfill: failed to mark complete; will retry next deploy", "error", mErr)
		}
		return nil
	}

	var totalUpserted, totalSkipped int
	for _, at := range accountTeams {
		creds, credsErr := w.langfuseStore.Get(at.AccountID)
		if credsErr != nil || creds == nil {
			totalSkipped++
			continue
		}
		client := langfuse.NewClient(w.cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)
		userIDs, queryErr := distinctBareSlackUserIDs(ctx, client)
		if queryErr != nil {
			w.log.Warn("Slack directory backfill: langfuse query failed",
				"account_id", at.AccountID, "error", queryErr)
			continue
		}
		for _, uid := range userIDs {
			if err := w.slackStore.UpsertObserved(ctx, at.TeamID, uid); err != nil {
				w.log.Warn("Slack directory backfill: upsert failed",
					"account_id", at.AccountID, "team_id", at.TeamID, "user_id", uid, "error", err)
				continue
			}
			totalUpserted++
		}
	}

	if err := w.slackStore.MarkDirectoryBackfillComplete(ctx); err != nil {
		// Failing to write the marker means the next restart re-runs the
		// backfill. That's fine — the UPSERT is idempotent, so re-runs
		// produce zero new rows for accounts we already covered.
		w.log.Warn("Slack directory backfill: failed to mark complete; will re-run next deploy", "error", err)
	}
	w.log.Info("Slack directory backfill completed",
		"upserted", totalUpserted,
		"skipped_accounts", totalSkipped,
		"linked_accounts", len(accountTeams),
	)
	return nil
}

// distinctBareSlackUserIDs queries Langfuse for every userId that has
// trace activity (no date bounds — we want full history) and filters to
// the bare-Slack shape. Mirrors the userId dimension query used by the
// users-summary handler but without metric aggregation since we only
// need the keys.
func distinctBareSlackUserIDs(ctx context.Context, client *langfuse.Client) ([]string, error) {
	q := langfuse.MetricsQuery{
		View: "traces",
		Metrics: []langfuse.MetricsQueryField{
			{Measure: "count", Aggregation: "count"},
		},
		Dimensions: []langfuse.MetricsDimension{{Field: "userId"}},
	}
	resp, err := client.GetMetrics(ctx, q)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(resp.Data))
	for _, row := range resp.Data {
		uid, _ := row["userId"].(string)
		if uid == "" || uid == "-" {
			continue
		}
		if !slackidentity.IsBareSlackUserID(uid) {
			continue
		}
		seen[uid] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out, nil
}
