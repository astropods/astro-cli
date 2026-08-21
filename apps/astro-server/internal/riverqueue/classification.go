package riverqueue

import (
	"context"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// Labels feed daily aggregates; a backfill converges over successive ticks.
const ClassificationInterval = time.Hour

// Wired by main to the handlers implementation, keeping handlers out of
// riverqueue's import graph. nil → the workers no-op.
type ClassificationProducer interface {
	ClassifyAccount(ctx context.Context, accountID string) error
}

// Discovery half: fans out one job per account so a slow one cannot stall the rest.
type ClassificationArgs struct{}

func (ClassificationArgs) Kind() string { return "classification.discover" }

func (ClassificationArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueClassification}
}

// ClassificationAccountArgs is the per-account job.
type ClassificationAccountArgs struct {
	AccountID string `json:"account_id"`
}

func (ClassificationAccountArgs) Kind() string { return "classification.account" }

// Dedupes per account so a tick firing mid-run cannot queue a second backfill.
func (ClassificationAccountArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: queueClassification,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
	}
}

func init() {
	registerJobKind[ClassificationArgs]()
	registerJobKind[ClassificationAccountArgs]()
}

// Enqueues per account with a Langfuse project; accounts without dev-tool
// telemetry no-op on the producer side, keeping discovery a single query.
type ClassificationDiscoveryWorker struct {
	river.WorkerDefaults[ClassificationArgs]
	queue         *Queue
	langfuseStore *langfuse.Store
	log           *logger.Logger
}

func (w *ClassificationDiscoveryWorker) Work(ctx context.Context, job *river.Job[ClassificationArgs]) error {
	if w.queue == nil || w.langfuseStore == nil {
		w.log.Debug("classification: discovery skipped, queue or langfuse store not wired")
		return nil
	}

	accountIDs, err := w.langfuseStore.ListAccountIDs()
	if err != nil {
		return err
	}
	for _, id := range accountIDs {
		if _, err := w.queue.Insert(ctx, ClassificationAccountArgs{AccountID: id}, nil); err != nil {
			// One failure must not abort the sweep.
			w.log.Warn("classification: enqueue failed", "account_id", id, "error", err)
		}
	}
	w.log.Debug("classification: discovery enqueued", "accounts", len(accountIDs))
	return nil
}

// ClassificationAccountWorker runs one account's bounded chunk of work.
type ClassificationAccountWorker struct {
	river.WorkerDefaults[ClassificationAccountArgs]
	producer ClassificationProducer
	log      *logger.Logger
}

// Ceiling so a hung upstream cannot hold a worker slot indefinitely.
func (w *ClassificationAccountWorker) Timeout(*river.Job[ClassificationAccountArgs]) time.Duration {
	return 30 * time.Minute
}

func (w *ClassificationAccountWorker) Work(ctx context.Context, job *river.Job[ClassificationAccountArgs]) error {
	if w.producer == nil {
		return nil
	}
	return w.producer.ClassifyAccount(ctx, job.Args.AccountID)
}
