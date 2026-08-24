package riverqueue

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/accountlifecycle"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// AccountPurgeArgs are the job arguments for the account purge periodic worker.
type AccountPurgeArgs struct{}

func (AccountPurgeArgs) Kind() string { return "account.purge" }

func (AccountPurgeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMaintenance}
}

func init() {
	registerJobKind[AccountPurgeArgs]()
}

// AccountPurgeWorker hard-deletes soft-deleted accounts once their retention
// window has passed. The sequence itself lives in accountlifecycle.Purger, which
// the admin console also drives to purge one account on demand. The billing
// customer is archived at delete time (see accountlifecycle.Deleter), not here.
type AccountPurgeWorker struct {
	river.WorkerDefaults[AccountPurgeArgs]
	purger *accountlifecycle.Purger
	log    *logger.Logger
}

func (w *AccountPurgeWorker) Work(ctx context.Context, job *river.Job[AccountPurgeArgs]) error {
	accountIDs, err := w.purger.Overdue(ctx)
	if err != nil {
		return err
	}
	if len(accountIDs) == 0 {
		return nil
	}

	// One account that cannot be purged does not fail the job: the others are
	// independent, and the sweep runs again next tick. A permanent blocker
	// surfaces through the account.purge_overdue audit check, not from here.
	var purged, skipped int
	for _, accountID := range accountIDs {
		if err := w.purger.Purge(ctx, accountID); err != nil {
			w.log.Error("purge accounts: purge account, will retry next tick failed", "error", err, "account_id", accountID)
			skipped++
			continue
		}
		purged++
	}

	w.log.Info("purge accounts: account purge complete", "purged", purged, "skipped", skipped)
	return nil
}
