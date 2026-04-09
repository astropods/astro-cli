package riverqueue

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// AvatarBackfillArgs are the job arguments for the avatar backfill worker.
type AvatarBackfillArgs struct{}

func (AvatarBackfillArgs) Kind() string { return "avatar.backfill" }

// AvatarBackfillWorker assigns preset avatars to accounts that don't have one yet.
// Runs once on startup via periodic job scheduling; skips accounts whose avatar
// already exists in the storage backend.
type AvatarBackfillWorker struct {
	river.WorkerDefaults[AvatarBackfillArgs]
	avatarStore *avatar.Store
	db          *sql.DB
	log         *logger.Logger
}

func (w *AvatarBackfillWorker) Work(ctx context.Context, _ *river.Job[AvatarBackfillArgs]) error {
	if w.avatarStore == nil {
		w.log.Debug("Avatar backfill skipped: no avatar store configured")
		return nil
	}

	const batchSize = 100
	var lastID string
	var totalProcessed, totalSkipped, totalFailed int

	for {
		rows, err := w.db.QueryContext(ctx, `
			SELECT id, name FROM accounts
			WHERE ($1 = '' OR id > $1::uuid)
			ORDER BY id
			LIMIT $2
		`, lastID, batchSize)
		if err != nil {
			w.log.Error("Avatar backfill: failed to query accounts", "error", err)
			return nil // Don't retry — transient DB issues shouldn't wedge the queue
		}

		var batchCount int
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				w.log.Error("Avatar backfill: failed to scan row", "error", err)
				continue
			}
			lastID = id
			batchCount++

			exists, isJPEG := w.avatarStore.AvatarIsValidJPEG(ctx, name)
			if exists && isJPEG {
				totalSkipped++
				continue
			}

			if err := w.avatarStore.AssignPreset(ctx, name); err != nil {
				w.log.Error("Avatar backfill: failed to assign preset", "account", name, "error", err)
				totalFailed++
				continue
			}
			totalProcessed++
		}
		_ = rows.Close()

		if batchCount == 0 {
			break
		}
	}

	if totalProcessed > 0 || totalFailed > 0 {
		w.log.Info("Avatar backfill completed",
			"processed", totalProcessed,
			"skipped", totalSkipped,
			"failed", totalFailed,
		)
	}

	return nil
}
