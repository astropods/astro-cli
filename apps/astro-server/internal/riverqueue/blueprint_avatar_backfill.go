package riverqueue

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/colorextract"
	"github.com/astropods/astro/apps/astro-server/internal/identitygen"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// BlueprintAvatarBackfillArgs are the job arguments for the blueprint avatar
// backfill worker.
type BlueprintAvatarBackfillArgs struct{}

func (BlueprintAvatarBackfillArgs) Kind() string { return "avatar.blueprint_backfill" }

func (BlueprintAvatarBackfillArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMaintenance}
}

func init() {
	registerJobKind[BlueprintAvatarBackfillArgs]()
}

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
		w.log.Debug("blueprint avatar backfill: skipped, no avatar store configured")
		return nil
	}

	bpProcessed, bpSkipped, bpFailed := w.backfillBlueprints(ctx)
	depProcessed, depSkipped, depFailed := w.backfillDeployments(ctx)

	w.log.Info("blueprint avatar backfill: completed",
		"blueprints_processed", bpProcessed,
		"blueprints_skipped", bpSkipped,
		"blueprints_failed", bpFailed,
		"deployments_processed", depProcessed,
		"deployments_skipped", depSkipped,
		"deployments_failed", depFailed,
	)
	return nil
}

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
			w.log.Error("blueprint avatar backfill: query agents", "error", err)
			return processed, skipped, failed
		}

		var batchCount int
		for rows.Next() {
			var accountID, agentName, accountName string
			if err := rows.Scan(&accountID, &agentName, &accountName); err != nil {
				w.log.Error("blueprint avatar backfill: scan agent row", "error", err)
				continue
			}
			lastAccountID = accountID
			lastName = agentName
			batchCount++

			exists, err := w.avatarStore.AgentAvatarExists(ctx, accountName, agentName)
			if err != nil {
				w.log.Error("blueprint avatar backfill: exists check", "account", accountName, "name", agentName, "error", err)
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
				w.log.Error("blueprint avatar backfill: generate", "account", accountName, "name", agentName, "error", err)
				failed++
				continue
			}
			if err := w.avatarStore.WriteAgentAvatarJPEG(ctx, accountName, agentName, jpegBytes); err != nil {
				w.log.Error("blueprint avatar backfill: upload", "account", accountName, "name", agentName, "error", err)
				failed++
				continue
			}
			if _, err := w.db.ExecContext(ctx, `UPDATE agents SET avatar_updated_at = now() WHERE account_id = $1::uuid AND name = $2`, accountID, agentName); err != nil {
				w.log.Warn("blueprint avatar backfill: stamp avatar_updated_at", "account", accountName, "name", agentName, "error", err)
			}
			if colors, err := colorextract.ExtractFromJPEG(jpegBytes); err != nil {
				w.log.Warn("blueprint avatar backfill: extract colors", "account", accountName, "name", agentName, "error", err)
			} else if j, err := json.Marshal(colors); err != nil {
				w.log.Warn("blueprint avatar backfill: marshal colors", "account", accountName, "name", agentName, "error", err)
			} else if _, err := w.db.ExecContext(ctx, `UPDATE agents SET avatar_colors = $1 WHERE account_id = $2::uuid AND name = $3`, j, accountID, agentName); err != nil {
				w.log.Warn("blueprint avatar backfill: store colors", "account", accountName, "name", agentName, "error", err)
			}
			processed++
		}
		iterErr := rows.Err()
		_ = rows.Close()
		if iterErr != nil {
			w.log.Error("blueprint avatar backfill: iterate agent rows failed", "error", iterErr)
			return processed, skipped, failed
		}

		if batchCount == 0 {
			break
		}
	}
	return processed, skipped, failed
}

func (w *BlueprintAvatarBackfillWorker) backfillDeployments(ctx context.Context) (processed, skipped, failed int) {
	const batchSize = 100
	var lastID string
	for {
		rows, err := w.db.QueryContext(ctx, `
			SELECT d.id, d.agent_name, owner.name
			FROM deployments d
			JOIN accounts owner ON owner.id = COALESCE(d.source_account_id, d.account_id)
			WHERE ($1 = '' OR d.id > $1)
			ORDER BY d.id
			LIMIT $2
		`, lastID, batchSize)
		if err != nil {
			w.log.Error("blueprint avatar backfill: query deployments", "error", err)
			return processed, skipped, failed
		}

		var batchCount int
		for rows.Next() {
			var id, agentName, ownerName string
			if err := rows.Scan(&id, &agentName, &ownerName); err != nil {
				w.log.Error("blueprint avatar backfill: scan deployment row", "error", err)
				continue
			}
			lastID = id
			batchCount++

			exists, err := w.avatarStore.DeploymentAvatarExists(ctx, id)
			if err != nil {
				w.log.Error("blueprint avatar backfill: deployment exists check", "deployment", id, "error", err)
				failed++
				continue
			}
			if exists {
				skipped++
				continue
			}

			copied, err := w.avatarStore.CopyAgentToDeployment(ctx, ownerName, agentName, id)
			if err != nil {
				w.log.Error("blueprint avatar backfill: copy to deployment", "deployment", id, "account", ownerName, "name", agentName, "error", err)
				failed++
				continue
			}
			if !copied {
				jpegBytes, err := identitygen.GenerateIdentityJPEG(identitygen.IdentityOptions{
					Seed: ownerName + "/" + agentName,
				})
				if err != nil {
					w.log.Error("blueprint avatar backfill: generate deployment avatar", "deployment", id, "account", ownerName, "name", agentName, "error", err)
					failed++
					continue
				}
				if err := w.avatarStore.WriteDeploymentAvatarJPEG(ctx, id, jpegBytes); err != nil {
					w.log.Error("blueprint avatar backfill: upload deployment avatar", "deployment", id, "account", ownerName, "name", agentName, "error", err)
					failed++
					continue
				}
			}
			if _, err := w.db.ExecContext(ctx, `UPDATE deployments SET avatar_updated_at = now() WHERE id = $1`, id); err != nil {
				w.log.Warn("blueprint avatar backfill: stamp deployment avatar_updated_at", "deployment", id, "error", err)
			}
			if jpegBytes, err := w.avatarStore.ReadDeploymentAvatar(ctx, id); err != nil {
				w.log.Warn("blueprint avatar backfill: read deployment avatar for colors", "deployment", id, "error", err)
			} else if colors, err := colorextract.ExtractFromJPEG(jpegBytes); err != nil {
				w.log.Warn("blueprint avatar backfill: extract deployment colors", "deployment", id, "error", err)
			} else if j, err := json.Marshal(colors); err != nil {
				w.log.Warn("blueprint avatar backfill: marshal deployment colors", "deployment", id, "error", err)
			} else if _, err := w.db.ExecContext(ctx, `UPDATE deployments SET avatar_colors = $1 WHERE id = $2`, j, id); err != nil {
				w.log.Warn("blueprint avatar backfill: store deployment colors", "deployment", id, "error", err)
			}
			processed++
		}
		iterErr := rows.Err()
		_ = rows.Close()
		if iterErr != nil {
			w.log.Error("blueprint avatar backfill: iterate deployment rows failed", "error", iterErr)
			return processed, skipped, failed
		}

		if batchCount == 0 {
			break
		}
	}
	return processed, skipped, failed
}
