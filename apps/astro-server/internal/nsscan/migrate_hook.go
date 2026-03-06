package nsscan

// TEMPORARY — delete this file and remove the AddHook call in main.go
// once all legacy deployments have been migrated to UUID-based namespaces.

import (
	"context"
	"database/sql"

	"github.com/postman/astro/apps/astro-server/internal/logger"
)

// MigrationHook returns a ScanHook that detects deployments still using the
// legacy SHA256-based namespace (pre-multi-deployment). It logs them for
// visibility; actual migration logic will be added when the migration strategy
// is finalized.
func MigrationHook(db *sql.DB, log *logger.Logger) ScanHook {
	return func(ctx context.Context, result *ScanResult) error {
		// Count deployments whose namespace doesn't match the UUID-based
		// pattern (astro- followed by 20 hex chars from a UUID).
		// Legacy namespaces use SHA256 hashes which are also hex, so we
		// identify them by checking if the deployment ID is embedded in
		// the namespace. For now, just log the stale/orphaned counts as
		// migration candidates.
		var legacyCount int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM deployments
			WHERE status = 'active'
			AND namespace NOT LIKE 'astro-' || REPLACE(id::text, '-', '') || '%'
		`).Scan(&legacyCount)
		if err != nil {
			return err
		}

		if legacyCount > 0 {
			log.Info("Legacy namespace deployments detected",
				"count", legacyCount,
			)
		}

		return nil
	}
}
