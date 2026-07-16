package riverqueue

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// AccountPurgeArgs are the job arguments for the account purge periodic worker.
type AccountPurgeArgs struct{}

func (AccountPurgeArgs) Kind() string { return "account_purge" }

func init() {
	registerJobKind[AccountPurgeArgs]()
}

// AccountPurgeWorker finds soft-deleted accounts past the retention period and
// hard-deletes them after cleaning up external resources (OpenMeter, Langfuse)
// and retrying any failed deployment teardowns.
type AccountPurgeWorker struct {
	river.WorkerDefaults[AccountPurgeArgs]
	db              *sql.DB
	deployStore     *deploymentstore.Store
	billingProvider billing.BillingProvider
	lfProvisioner   *langfuse.Provisioner
	lfStore         *langfuse.Store
	aigwProvisioner *aigateway.Provisioner
	aigwStore       *aigateway.Store
	aigwDevStore    *aigateway.DevStore
	retentionDays   int
	log             *logger.Logger
	enqueueUndeploy func(ctx context.Context, deploymentID string) error
}

func (w *AccountPurgeWorker) Work(ctx context.Context, job *river.Job[AccountPurgeArgs]) error {
	retention := w.retentionDays
	if retention <= 0 {
		retention = 7
	}

	cutoff := time.Now().AddDate(0, 0, -retention)

	rows, err := w.db.QueryContext(ctx,
		`SELECT id FROM accounts WHERE deleted_at IS NOT NULL AND deleted_at < $1`,
		cutoff,
	)
	if err != nil {
		return fmt.Errorf("query deleted accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var accountIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan account id: %w", err)
		}
		accountIDs = append(accountIDs, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate accounts: %w", err)
	}

	if len(accountIDs) == 0 {
		return nil
	}

	var purged, skipped int
	for _, accountID := range accountIDs {
		if err := w.purgeAccount(ctx, accountID); err != nil {
			w.log.Error("Failed to purge account, will retry next tick", "error", err, "account_id", accountID)
			skipped++
			continue
		}
		purged++
	}

	w.log.Info("Account purge complete", "purged", purged, "skipped", skipped)
	return nil
}

func (w *AccountPurgeWorker) purgeAccount(ctx context.Context, accountID string) error {
	// 1. Check for deployments that haven't finished tearing down
	// GetVisibleDeploymentsByAccount returns all deployments where status != 'undeployed'
	pending, err := w.deployStore.GetVisibleDeploymentsByAccount(accountID)
	if err != nil {
		return fmt.Errorf("get pending deployments: %w", err)
	}

	if len(pending) > 0 {
		for _, dep := range pending {
			// Re-enqueue undeploy for anything not already in undeploying state
			if dep.Status != deploymentstore.StatusUndeploying {
				if err := w.enqueueUndeploy(ctx, dep.ID); err != nil {
					w.log.Error("Failed to re-enqueue undeploy", "error", err, "deployment_id", dep.ID, "account_id", accountID)
				}
			}
		}
		return fmt.Errorf("account %s has %d deployments still pending teardown", accountID, len(pending))
	}

	// 2. Clean up external resources (must succeed before hard-delete)

	// Billing customer
	if w.billingProvider != nil {
		var customerID sql.NullString
		if err := w.db.QueryRowContext(ctx,
			`SELECT openmeter_customer_id FROM accounts WHERE id = $1`, accountID,
		).Scan(&customerID); err != nil {
			return fmt.Errorf("query billing customer id: %w", err)
		}
		if customerID.Valid && customerID.String != "" {
			if err := w.billingProvider.DeleteCustomer(ctx, customerID.String); err != nil {
				return fmt.Errorf("delete billing customer: %w", err)
			}
		}
	}

	// Langfuse
	if w.lfProvisioner != nil && w.lfStore != nil {
		al, err := w.lfStore.Get(accountID)
		if err != nil {
			return fmt.Errorf("get langfuse credentials: %w", err)
		}
		if al != nil {
			if err := w.lfProvisioner.DeleteProject(ctx, al.LangfuseProjectID); err != nil {
				return fmt.Errorf("delete langfuse project: %w", err)
			}
		}
	}

	// AI Gateway: revoke every per-deployment virtual key under the account
	// upstream so they stop accruing usage, then drop the local rows. The
	// account hard-delete cascades to deployment_ai_gateway via FK; we still
	// need the upstream revokes since LiteLLM has no FK back to us.
	if w.aigwProvisioner != nil && w.aigwStore != nil {
		if err := w.aigwProvisioner.RevokeAccount(ctx, w.aigwStore, accountID); err != nil {
			w.log.Warn("Failed to revoke AI Gateway keys, continuing purge", "error", err, "account_id", accountID)
		}
	}

	// Dev keys: same shape as above — the FK cascade clears the rows but
	// the upstream LiteLLM keys would linger until their 8h TTL without an
	// explicit /key/delete.
	if w.aigwProvisioner != nil && w.aigwDevStore != nil {
		if err := w.aigwProvisioner.RevokeAccountDevKeys(ctx, w.aigwDevStore, accountID); err != nil {
			w.log.Warn("Failed to revoke AI Gateway dev keys, continuing purge", "error", err, "account_id", accountID)
		}
	}

	// 3. Hard-delete the account — cascade handles all child tables
	result, err := w.db.ExecContext(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil // already deleted
	}

	w.log.Info("Account purged", "account_id", accountID)
	return nil
}
