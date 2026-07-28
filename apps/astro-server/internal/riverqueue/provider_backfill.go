package riverqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro-spec"
)

// ProviderBackfillArgs are the job arguments for the workload-provider backfill.
type ProviderBackfillArgs struct{}

func (ProviderBackfillArgs) Kind() string { return "workload.provider_backfill" }

func (ProviderBackfillArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMaintenance}
}

func init() {
	registerJobKind[ProviderBackfillArgs]()
}

// ProviderBackfillWorker fills deployment_workloads.provider for rows written
// before the column existed, deriving each workload's provider from the
// deployment's stored spec. Idempotent: only rows still NULL are touched, so
// re-runs and freshly-deployed rows are left alone.
type ProviderBackfillWorker struct {
	river.WorkerDefaults[ProviderBackfillArgs]
	db  *sql.DB
	log *logger.Logger
}

func (w *ProviderBackfillWorker) Work(ctx context.Context, _ *river.Job[ProviderBackfillArgs]) error {
	const batchSize = 200
	var lastID string
	var scanned, updated int

	for {
		rows, err := w.db.QueryContext(ctx, `
			SELECT d.id, d.deployment_spec_json
			FROM deployments d
			WHERE COALESCE(d.deployment_spec_json, '') <> ''
			  AND EXISTS (
			    SELECT 1 FROM deployment_workloads wl
			    WHERE wl.deployment_id = d.id
			      AND wl.provider IS NULL
			      AND wl.component_kind IN ('knowledge', 'model')
			  )
			  AND ($1 = '' OR d.id > $1)
			ORDER BY d.id
			LIMIT $2
		`, lastID, batchSize)
		if err != nil {
			// Return the error so River retries with backoff — a run that fails
			// because the column isn't in place yet must recover, not be marked
			// done. Per-deployment failures below stay log-and-continue.
			return fmt.Errorf("query deployments: %w", err)
		}

		type pending struct{ id, specJSON string }
		var batch []pending
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.id, &p.specJSON); err != nil {
				w.log.Error("Provider backfill: scan row", "error", err)
				continue
			}
			lastID = p.id
			batch = append(batch, p)
		}
		_ = rows.Close()
		if len(batch) == 0 {
			break
		}

		for _, p := range batch {
			scanned++
			var ds spec.AstroDeploymentSpec
			if err := json.Unmarshal([]byte(p.specJSON), &ds); err != nil {
				w.log.Warn("Provider backfill: parse spec", "deployment_id", p.id, "error", err)
				continue
			}
			byKind := map[string]map[string]string{"model": {}, "knowledge": {}}
			for name, m := range ds.Models {
				if m.Provider != "" {
					byKind["model"][name] = m.Provider
				}
			}
			for name, k := range ds.Knowledge {
				if k.Provider != "" {
					byKind["knowledge"][name] = k.Provider
				}
			}
			for kind, byKey := range byKind {
				for key, provider := range byKey {
					res, err := w.db.ExecContext(ctx, `
						UPDATE deployment_workloads
						SET provider = $1
						WHERE deployment_id = $2 AND component_kind = $3 AND component_key = $4 AND provider IS NULL
					`, provider, p.id, kind, key)
					if err != nil {
						w.log.Warn("Provider backfill: update", "deployment_id", p.id, "kind", kind, "key", key, "error", err)
						continue
					}
					if n, _ := res.RowsAffected(); n > 0 {
						updated += int(n)
					}
				}
			}

			// Settle provider-less knowledge/model rows (custom containers) to ''
			// so the deployment stops matching the scan on subsequent runs.
			if _, err := w.db.ExecContext(ctx, `
				UPDATE deployment_workloads
				SET provider = ''
				WHERE deployment_id = $1 AND component_kind IN ('knowledge', 'model') AND provider IS NULL
			`, p.id); err != nil {
				w.log.Warn("Provider backfill: settle", "deployment_id", p.id, "error", err)
			}
		}
	}

	w.log.Info("Provider backfill completed", "deployments_scanned", scanned, "rows_updated", updated)
	return nil
}
