package riverqueue

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// ReextractColorsArgs are the job arguments for the one-time color
// re-extraction migration triggered by the algorithm change on 2026-04-23.
type ReextractColorsArgs struct{}

func (ReextractColorsArgs) Kind() string { return "avatar_colors.reextract_2026_04_23" }

// ReextractColorsWorker re-extracts avatar_colors for every agent and
// deployment using the updated target-based palette selection algorithm.
// Unlike the regular backfill which only processes rows with NULL colors,
// this worker processes ALL rows to regenerate stale palettes.
//
// It is registered as a one-time periodic job that runs on startup, then
// marks itself complete by checking a sentinel value in the database.
type ReextractColorsWorker struct {
	river.WorkerDefaults[ReextractColorsArgs]
	avatarStore *avatar.Store
	db          *sql.DB
	log         *logger.Logger
}

func (w *ReextractColorsWorker) Work(ctx context.Context, job *river.Job[ReextractColorsArgs]) error {
	if w.avatarStore == nil {
		w.log.Debug("Color re-extraction skipped: no avatar store configured")
		return nil
	}

	// Check if a previous run already completed. Look for any completed job
	// with this kind that isn't the current job.
	var count int
	err := w.db.QueryRowContext(ctx, `
		SELECT count(*) FROM river_job
		WHERE kind = $1 AND state = 'completed' AND id != $2
	`, (ReextractColorsArgs{}).Kind(), job.ID).Scan(&count)
	if err == nil && count > 0 {
		w.log.Info("Color re-extraction already completed in a previous run, skipping")
		return nil
	}

	agentProcessed, agentSkipped, agentFailed := w.reextractAgentColors(ctx)
	depProcessed, depSkipped, depFailed := w.reextractDeploymentColors(ctx)

	w.log.Info("Color re-extraction migration completed",
		"agents_processed", agentProcessed,
		"agents_skipped", agentSkipped,
		"agents_failed", agentFailed,
		"deployments_processed", depProcessed,
		"deployments_skipped", depSkipped,
		"deployments_failed", depFailed,
	)
	return nil
}

// reextractAgentColors re-extracts avatar colors for ALL agents (not just NULL).
func (w *ReextractColorsWorker) reextractAgentColors(ctx context.Context) (processed, skipped, failed int) {
	var lastAccountID, lastName string
	return backfillColors(ctx, w.log, "Agent color re-extraction", func(ctx context.Context) ([]colorBackfillItem, error) {
		rows, err := w.db.QueryContext(ctx, `
			SELECT a.account_id::text, a.name, acc.name
			FROM agents a
			JOIN accounts acc ON acc.id = a.account_id
			WHERE a.archived_at IS NULL
			  AND ($1 = '' OR (a.account_id, a.name) > ($1::uuid, $2))
			ORDER BY a.account_id, a.name
			LIMIT $3
		`, lastAccountID, lastName, 100)
		if err != nil {
			return nil, err
		}
		defer rows.Close() //nolint:errcheck

		var items []colorBackfillItem
		for rows.Next() {
			var accountID, agentName, accountName string
			if err := rows.Scan(&accountID, &agentName, &accountName); err != nil {
				w.log.Error("Agent color re-extraction: scan row", "error", err)
				continue
			}
			lastAccountID = accountID
			lastName = agentName
			items = append(items, colorBackfillItem{
				readAvatar: func(ctx context.Context) ([]byte, error) {
					return w.avatarStore.ReadAgentAvatar(ctx, accountName, agentName)
				},
				storeColors: func(ctx context.Context, j []byte) error {
					_, err := w.db.ExecContext(ctx, `UPDATE agents SET avatar_colors = $1 WHERE account_id = $2::uuid AND name = $3`, j, accountID, agentName)
					return err
				},
				logAttrs: []any{"account", accountName, "name", agentName},
			})
		}
		return items, rows.Err()
	})
}

// reextractDeploymentColors re-extracts avatar colors for ALL deployments.
func (w *ReextractColorsWorker) reextractDeploymentColors(ctx context.Context) (processed, skipped, failed int) {
	var lastID string
	return backfillColors(ctx, w.log, "Deployment color re-extraction", func(ctx context.Context) ([]colorBackfillItem, error) {
		rows, err := w.db.QueryContext(ctx, `
			SELECT d.id
			FROM deployments d
			WHERE ($1 = '' OR d.id > $1)
			ORDER BY d.id
			LIMIT 100
		`, lastID)
		if err != nil {
			return nil, err
		}
		defer rows.Close() //nolint:errcheck

		var items []colorBackfillItem
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				w.log.Error("Deployment color re-extraction: scan row", "error", err)
				continue
			}
			lastID = id
			items = append(items, colorBackfillItem{
				readAvatar: func(ctx context.Context) ([]byte, error) { return w.avatarStore.ReadDeploymentAvatar(ctx, id) },
				storeColors: func(ctx context.Context, j []byte) error {
					_, err := w.db.ExecContext(ctx, `UPDATE deployments SET avatar_colors = $1 WHERE id = $2`, j, id)
					return err
				},
				logAttrs:        []any{"deployment", id},
				skipOnReadError: true,
			})
		}
		return items, rows.Err()
	})
}
