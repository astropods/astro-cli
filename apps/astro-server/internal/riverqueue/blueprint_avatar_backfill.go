package riverqueue

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/identitygen"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// BlueprintAvatarBackfillArgs are the job arguments for the blueprint avatar
// backfill worker.
type BlueprintAvatarBackfillArgs struct{}

func (BlueprintAvatarBackfillArgs) Kind() string { return "blueprint_avatar.backfill" }

// BlueprintAvatarBackfillWorker ensures every blueprint has a placeholder
// avatar in storage and every deployment inherits its blueprint's avatar.
//
// Runs on startup via periodic scheduling (24h). Mirrors the shape of
// AvatarBackfillWorker: cursor-paginate, skip what already exists, log and
// continue on per-record errors, return nil from Work so transient issues
// don't wedge the queue.
type BlueprintAvatarBackfillWorker struct {
	river.WorkerDefaults[BlueprintAvatarBackfillArgs]
	avatarStore *avatar.Store
	db          *sql.DB
	log         *logger.Logger
}

func (w *BlueprintAvatarBackfillWorker) Work(ctx context.Context, _ *river.Job[BlueprintAvatarBackfillArgs]) error {
	if w.avatarStore == nil {
		w.log.Debug("Blueprint avatar backfill skipped: no avatar store configured")
		return nil
	}

	bpProcessed, bpSkipped, bpFailed := w.backfillBlueprints(ctx)
	depProcessed, depSkipped, depFailed := w.backfillDeployments(ctx)

	if bpProcessed > 0 || bpFailed > 0 || depProcessed > 0 || depFailed > 0 {
		w.log.Info("Blueprint avatar backfill completed",
			"blueprints_processed", bpProcessed,
			"blueprints_skipped", bpSkipped,
			"blueprints_failed", bpFailed,
			"deployments_processed", depProcessed,
			"deployments_skipped", depSkipped,
			"deployments_failed", depFailed,
		)
	}
	return nil
}

// backfillBlueprints generates and uploads a placeholder avatar for every
// non-archived blueprint that doesn't already have one in storage.
func (w *BlueprintAvatarBackfillWorker) backfillBlueprints(ctx context.Context) (processed, skipped, failed int) {
	const batchSize = 100
	var (
		lastAccountID string
		lastName      string
	)
	for {
		rows, err := w.db.QueryContext(ctx, `
			SELECT a.account_id::text, a.name, acc.name
			FROM agents a
			JOIN accounts acc ON acc.id = a.account_id
			WHERE a.archived_at IS NULL
			  AND ($1 = '' OR (a.account_id, a.name) > ($1::uuid, $2))
			ORDER BY a.account_id, a.name
			LIMIT $3
		`, lastAccountID, lastName, batchSize)
		if err != nil {
			w.log.Error("Blueprint avatar backfill: query agents", "error", err)
			return processed, skipped, failed
		}

		var batchCount int
		for rows.Next() {
			var accountID, agentName, accountName string
			if err := rows.Scan(&accountID, &agentName, &accountName); err != nil {
				w.log.Error("Blueprint avatar backfill: scan agent row", "error", err)
				continue
			}
			lastAccountID = accountID
			lastName = agentName
			batchCount++

			exists, err := w.avatarStore.AgentAvatarExists(ctx, accountName, agentName)
			if err != nil {
				w.log.Error("Blueprint avatar backfill: exists check", "account", accountName, "name", agentName, "error", err)
				failed++
				continue
			}
			if exists {
				skipped++
				continue
			}

			jpegBytes, err := identitygen.GenerateIdentityJPEG(identitygen.IdentityOptions{
				Seed: accountName + "/" + agentName,
			})
			if err != nil {
				w.log.Error("Blueprint avatar backfill: generate", "account", accountName, "name", agentName, "error", err)
				failed++
				continue
			}
			if err := w.avatarStore.WriteAgentAvatarJPEG(ctx, accountName, agentName, jpegBytes); err != nil {
				w.log.Error("Blueprint avatar backfill: upload", "account", accountName, "name", agentName, "error", err)
				failed++
				continue
			}
			processed++
		}
		_ = rows.Close()

		if batchCount == 0 {
			break
		}
	}
	return processed, skipped, failed
}

// backfillDeployments copies the blueprint's avatar to each deployment that
// doesn't already have one. Assumes blueprints have been filled first so the
// source avatar always exists.
func (w *BlueprintAvatarBackfillWorker) backfillDeployments(ctx context.Context) (processed, skipped, failed int) {
	const batchSize = 100
	var lastID string
	for {
		rows, err := w.db.QueryContext(ctx, `
			SELECT d.id, d.agent_name, acc.name
			FROM deployments d
			JOIN accounts acc ON acc.id = d.account_id
			WHERE ($1 = '' OR d.id > $1)
			ORDER BY d.id
			LIMIT $2
		`, lastID, batchSize)
		if err != nil {
			w.log.Error("Blueprint avatar backfill: query deployments", "error", err)
			return processed, skipped, failed
		}

		var batchCount int
		for rows.Next() {
			var id, agentName, accountName string
			if err := rows.Scan(&id, &agentName, &accountName); err != nil {
				w.log.Error("Blueprint avatar backfill: scan deployment row", "error", err)
				continue
			}
			lastID = id
			batchCount++

			exists, err := w.avatarStore.DeploymentAvatarExists(ctx, id)
			if err != nil {
				w.log.Error("Blueprint avatar backfill: deployment exists check", "deployment", id, "error", err)
				failed++
				continue
			}
			if exists {
				skipped++
				continue
			}

			copied, err := w.avatarStore.CopyAgentToDeployment(ctx, accountName, agentName, id)
			if err != nil {
				w.log.Error("Blueprint avatar backfill: copy to deployment", "deployment", id, "account", accountName, "name", agentName, "error", err)
				failed++
				continue
			}
			if !copied {
				// Blueprint avatar missing — blueprint pass should've filled it.
				// Treat as failed so it's visible in logs and re-attempted next run.
				w.log.Warn("Blueprint avatar backfill: blueprint avatar missing for deployment", "deployment", id, "account", accountName, "name", agentName)
				failed++
				continue
			}
			processed++
		}
		_ = rows.Close()

		if batchCount == 0 {
			break
		}
	}
	return processed, skipped, failed
}
