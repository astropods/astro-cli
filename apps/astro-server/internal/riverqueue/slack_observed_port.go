package riverqueue

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
)

// SlackObservedPortArgs has no per-invocation arguments — the worker
// copies every active observed row from slack_identity_mappings into
// slack_observed_users in a single SQL statement.
type SlackObservedPortArgs struct{}

// Kind groups the job alongside other slack-prefixed workers in logs
// and the River UI.
func (SlackObservedPortArgs) Kind() string { return "slack.observed_port" }

// SlackObservedPortWorker is the ONE-SHOT migration worker that copies
// existing observed rows out of slack_identity_mappings (source='observed',
// revoked_at IS NULL) and into the new slack_observed_users table. After
// a successful run it writes a singleton marker row
// (slack_observed_port_marker); subsequent runs see the marker on entry
// and exit immediately.
//
// "Never runs again" guarantee, same layered pattern as the directory
// backfill:
//
//  1. main.go checks the marker before enqueueing — if set, no job is
//     enqueued at all.
//  2. The worker checks the marker on entry — defense against a race
//     where the marker was written between the main.go check and the
//     job actually executing (e.g. multi-replica startup).
//  3. UniqueOpts on the enqueue collapses concurrent enqueues into a
//     single queued job.
//
// To re-run (e.g. after a manual cleanup), DELETE FROM
// slack_observed_port_marker and the next pod restart picks it back up.
// The copy is idempotent (ON CONFLICT DO NOTHING) so re-runs produce
// zero net change.
//
// Lives in the riverqueue package alongside SlackDirectoryBackfillWorker
// because both are one-shot Slack-directory plumbing with identical
// lifecycle. Once PR 3 retires the legacy observed rows from
// slack_identity_mappings, this worker becomes dead code and can be
// removed along with the marker table.
type SlackObservedPortWorker struct {
	river.WorkerDefaults[SlackObservedPortArgs]

	slackStore *slackidentity.Store
	log        *logger.Logger
}

// Work runs the one-shot port. Returns nil on errors so the queue
// doesn't wedge — the marker only writes on full success, so a
// transient failure retries on the next pod restart.
func (w *SlackObservedPortWorker) Work(ctx context.Context, _ *river.Job[SlackObservedPortArgs]) error {
	if w.slackStore == nil {
		w.log.Debug("Slack observed port skipped: missing dependencies")
		return nil
	}

	done, err := w.slackStore.IsObservedPortComplete(ctx)
	if err != nil {
		w.log.Warn("Slack observed port: marker check failed; skipping", "error", err)
		return nil
	}
	if done {
		w.log.Debug("Slack observed port: already complete; skipping")
		return nil
	}

	n, err := w.slackStore.PortObservedRowsToNewTable(ctx)
	if err != nil {
		w.log.Error("Slack observed port: copy failed", "error", err)
		return nil
	}

	if err := w.slackStore.MarkObservedPortComplete(ctx); err != nil {
		// Marker write failure means the next restart re-runs the port.
		// The INSERT ... ON CONFLICT DO NOTHING makes that a zero-net-
		// change retry, so this is safe.
		w.log.Warn("Slack observed port: failed to mark complete; will re-run next deploy", "error", err)
	}
	w.log.Info("Slack observed port completed", "rows_inserted", n)
	return nil
}
