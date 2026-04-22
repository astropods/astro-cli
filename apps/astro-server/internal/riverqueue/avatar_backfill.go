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

	// Backfill colors for accounts that have avatars but no colors yet.
	colorProcessed, colorSkipped, colorFailed := w.backfillAccountColors(ctx)

	w.log.Info("Avatar backfill completed",
		"processed", totalProcessed,
		"skipped", totalSkipped,
		"failed", totalFailed,
		"colors_processed", colorProcessed,
		"colors_skipped", colorSkipped,
		"colors_failed", colorFailed,
	)

	return nil
}

// backfillAccountColors extracts and stores avatar colors for accounts
// that have an avatar but no colors yet.
func (w *AvatarBackfillWorker) backfillAccountColors(ctx context.Context) (processed, skipped, failed int) {
	var lastID string
	return backfillColors(ctx, w.log, "Account color backfill", func(ctx context.Context) ([]colorBackfillItem, error) {
		rows, err := w.db.QueryContext(ctx, `
			SELECT id, name FROM accounts
			WHERE deleted_at IS NULL
			  AND avatar_colors IS NULL
			  AND ($1 = '' OR id > $1::uuid)
			ORDER BY id
			LIMIT 100
		`, lastID)
		if err != nil {
			return nil, err
		}
		defer rows.Close() //nolint:errcheck

		var items []colorBackfillItem
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				w.log.Error("Account color backfill: scan row", "error", err)
				continue
			}
			lastID = id
			items = append(items, colorBackfillItem{
				readAvatar: func(ctx context.Context) ([]byte, error) { return w.avatarStore.ReadAvatar(ctx, name) },
				storeColors: func(ctx context.Context, j []byte) error {
					_, err := w.db.ExecContext(ctx, `UPDATE accounts SET avatar_colors = $1 WHERE id = $2`, j, id)
					return err
				},
				logAttrs:        []any{"account", name},
				skipOnReadError: true,
			})
		}
		return items, rows.Err()
	})
}
