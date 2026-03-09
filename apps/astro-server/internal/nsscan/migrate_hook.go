package nsscan

// TEMPORARY — delete this file and remove the AddHook call in main.go
// once all legacy deployments have been migrated to UUID-based namespaces.

import (
	"context"
	"database/sql"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// MigrationHook returns a ScanHook that adopts orphaned K8s namespaces by
// matching them to stale DB deployments via account_id + agent_name. This
// handles the case where the multi-deployment schema change altered namespace
// derivation (SHA256-based → deployment-ID-based) but K8s still has the old
// namespaces running.
func MigrationHook(db *sql.DB, log *logger.Logger) ScanHook {
	return func(ctx context.Context, result *ScanResult) error {
		if len(result.Orphaned) == 0 || len(result.StaleDeployments) == 0 {
			return nil
		}

		// Build a lookup from (account_id, agent_name) → orphaned K8s namespace.
		// If multiple orphaned namespaces share the same key, the last one wins;
		// this is acceptable because the hook runs every scan cycle and will
		// adopt the remaining ones on subsequent passes.
		type adoptCandidate struct {
			k8sNamespace string
		}
		orphanIndex := make(map[string]adoptCandidate) // key: "account_id:agent_name"
		for _, o := range result.Orphaned {
			if o.AccountID == "" || o.AgentName == "" {
				continue // skip namespaces without required labels
			}
			key := o.AccountID + ":" + o.AgentName
			orphanIndex[key] = adoptCandidate{k8sNamespace: o.Name}
		}

		adopted := 0
		for _, sd := range result.StaleDeployments {
			key := sd.AccountID + ":" + sd.AgentName
			candidate, ok := orphanIndex[key]
			if !ok {
				continue
			}

			// Update the deployment's namespace to the actual K8s namespace.
			_, err := db.ExecContext(ctx, `
				UPDATE deployments SET namespace = $1 WHERE id = $2 AND status = 'active'
			`, candidate.k8sNamespace, sd.ID)
			if err != nil {
				log.Warn("Failed to adopt orphaned namespace",
					"deployment_id", sd.ID,
					"old_namespace", sd.Namespace,
					"k8s_namespace", candidate.k8sNamespace,
					"error", err,
				)
				continue
			}

			// Delete the stale namespace_ownership row (keyed by old namespace).
			// The next scan cycle will re-create it with the correct K8s namespace.
			_, err = db.ExecContext(ctx, `
				DELETE FROM namespace_ownership WHERE namespace = $1
			`, sd.Namespace)
			if err != nil {
				log.Warn("Failed to clean up old namespace_ownership entry",
					"old_namespace", sd.Namespace,
					"error", err,
				)
			}

			log.Info("Adopted orphaned namespace",
				"deployment_id", sd.ID,
				"agent", sd.AgentName,
				"old_namespace", sd.Namespace,
				"k8s_namespace", candidate.k8sNamespace,
			)
			adopted++

			// Remove from index so we don't double-adopt.
			delete(orphanIndex, key)
		}

		if adopted > 0 {
			log.Info("Migration hook complete", "adopted", adopted)
		}

		// Also count remaining legacy deployments for visibility.
		var legacyCount int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM deployments
			WHERE status = 'active'
			AND namespace NOT LIKE 'astro-' || REPLACE(id, '-', '') || '%'
		`).Scan(&legacyCount)
		if err != nil {
			return err
		}
		if legacyCount > 0 {
			log.Info("Legacy namespace deployments remaining",
				"count", legacyCount,
			)
		}

		return nil
	}
}
