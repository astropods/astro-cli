package accountlifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// RetentionDays is how long a soft-deleted account is kept before the purge may
// hard-delete it. The system audit check for overdue purges measures against the
// same window, so both move together. An operator who needs an account gone
// sooner purges it directly rather than shortening the window for everyone.
const RetentionDays = 7

// ErrTeardownPending reports that an account still owns work the purge refuses
// to delete around: deployments that have not finished undeploying, or WorkOS
// authorization rows that have not converged. Hard-deleting through either
// would orphan cluster resources or WorkOS tuples that nothing else cleans up.
var ErrTeardownPending = errors.New("account teardown still pending")

// Purger hard-deletes a soft-deleted account and the external resources it
// owns. The periodic sweep runs it for every account past the retention window;
// the admin console runs it for one account on demand.
type Purger struct {
	Log         *logger.Logger
	DB          *sql.DB
	Deployments *deploymentstore.Store

	// Undeploy re-enqueues teardown for a deployment the soft-delete failed to
	// queue. Set after the job queue exists, so a Purger built during worker
	// registration is only complete once the queue is wired.
	Undeploy func(ctx context.Context, deploymentID string) error

	Langfuse      *langfuse.Provisioner
	LangfuseStore *langfuse.Store
	AIGateway     *aigateway.Provisioner
	Keys          *aigateway.Store
	DevKeys       *aigateway.DevStore
	JudgeKeys     *aigateway.JudgeStore
	FGASync       *authz.DeploymentFGASyncStore
}

// Overdue returns the soft-deleted accounts whose retention window has passed.
func (p *Purger) Overdue(ctx context.Context) ([]string, error) {
	cutoff := time.Now().AddDate(0, 0, -RetentionDays)

	rows, err := p.DB.QueryContext(ctx,
		`SELECT id FROM accounts WHERE deleted_at IS NOT NULL AND deleted_at < $1`,
		cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("query deleted accounts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan account id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	return ids, nil
}

// Purge removes an account's external resources and then the row itself. It
// refuses while teardown is outstanding, re-enqueueing the undeploys it is
// waiting on so the next attempt has a chance of succeeding. Callers retry:
// nothing here is destructive until the deletes at the end.
func (p *Purger) Purge(ctx context.Context, accountID string) error {
	pending, err := p.Deployments.GetVisibleDeploymentsByAccount(accountID)
	if err != nil {
		return fmt.Errorf("get pending deployments: %w", err)
	}
	if len(pending) > 0 {
		for _, dep := range pending {
			if dep.Status == deploymentstore.StatusUndeploying {
				continue
			}
			if err := p.Undeploy(ctx, dep.ID); err != nil {
				p.Log.Error("purge accounts: re-enqueue undeploy failed", "error", err, "deployment_id", dep.ID, "account_id", accountID)
			}
		}
		return fmt.Errorf("%w: %d deployment(s) not yet undeployed", ErrTeardownPending, len(pending))
	}
	if p.FGASync != nil {
		pendingFGA, err := p.FGASync.HasPendingForAccount(ctx, accountID)
		if err != nil {
			return err
		}
		if pendingFGA {
			return fmt.Errorf("%w: deployment authorization cleanup has not converged", ErrTeardownPending)
		}
	}

	// Langfuse must succeed before the row goes: the project holds trace data
	// under our org, and once the account row is gone nothing records which
	// project to delete.
	if p.Langfuse != nil && p.LangfuseStore != nil {
		al, err := p.LangfuseStore.Get(accountID)
		if err != nil {
			return fmt.Errorf("get langfuse credentials: %w", err)
		}
		if al != nil {
			if err := p.Langfuse.DeleteProject(ctx, al.LangfuseProjectID); err != nil {
				return fmt.Errorf("delete langfuse project: %w", err)
			}
		}
	}

	// The account delete cascades the gateway-key rows, but LiteLLM holds no FK
	// back to us: a key not revoked upstream keeps working with no local record
	// that it exists. Warn and continue, because a gateway outage must not
	// block the purge indefinitely.
	if p.AIGateway != nil && p.Keys != nil {
		if err := p.AIGateway.RevokeAccount(ctx, p.Keys, accountID); err != nil {
			p.Log.Warn("purge accounts: revoke AI Gateway keys, continuing purge failed", "error", err, "account_id", accountID)
		}
	}
	if p.AIGateway != nil && p.DevKeys != nil {
		if err := p.AIGateway.RevokeAccountDevKeys(ctx, p.DevKeys, accountID); err != nil {
			p.Log.Warn("purge accounts: revoke AI Gateway dev keys, continuing purge failed", "error", err, "account_id", accountID)
		}
	}
	if p.AIGateway != nil && p.JudgeKeys != nil {
		if err := p.AIGateway.RevokeAccountJudgeKeys(ctx, p.JudgeKeys, accountID); err != nil {
			p.Log.Warn("purge accounts: revoke AI Gateway judge key, continuing purge failed", "error", err, "account_id", accountID)
		}
	}

	result, err := p.DB.ExecContext(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil // already purged
	}

	p.Log.Info("purge accounts: account purged", "account_id", accountID)
	return nil
}
