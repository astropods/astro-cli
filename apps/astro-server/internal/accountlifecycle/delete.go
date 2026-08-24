// Package accountlifecycle owns the account soft-delete sequence. The public
// API's owner-initiated delete and the admin console's delete both run it, so
// neither can archive billing, tear down deployments, or drop the WorkOS
// organization in a way the other misses.
package accountlifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/org"
)

// Deleter runs the soft-delete sequence for one account. Log, Accounts, and
// Deployments are required. Every other collaborator is optional: an
// unconfigured backend drops its cleanup step instead of failing the delete.
type Deleter struct {
	Log         *logger.Logger
	Accounts    *account.AccountStore
	Deployments *deploymentstore.Store

	// Undeploy moves one deployment to undeploying and enqueues its teardown
	// job. Injected so this package and the public undeploy route share a
	// single implementation (handlers.EnqueueUndeploy).
	Undeploy func(ctx context.Context, dep *deploymentstore.Deployment) error

	Org            *org.Client
	Billing        billing.BillingProvider
	BillingBackend string
	AIGateway      *aigateway.Provisioner
	JudgeKeys      *aigateway.JudgeStore
}

// Result reports what the delete set in motion.
type Result struct {
	// DeploymentsUndeploying counts the deployments queued for teardown.
	DeploymentsUndeploying int
}

// Delete archives the account's billing customer, soft-deletes the account, and
// starts best-effort teardown of what it owns. The row itself survives until the
// purge worker hard-deletes it after the retention window.
//
// Billing is archived first because it is the only step that can still cost
// money after the fact: an account marked deleted while its customer keeps
// accruing charges bills someone nobody is watching. A failure there aborts
// before anything is mutated, so the caller can retry. Past the soft-delete
// every step is best-effort, and the purge worker retries each one before it
// removes the row.
func (d *Deleter) Delete(ctx context.Context, acct *account.Account) (Result, error) {
	if acct == nil {
		return Result{}, errors.New("nil account")
	}

	if d.Billing != nil {
		customerID, err := d.Accounts.GetBillingCustomerID(acct.ID, d.BillingBackend)
		if err != nil {
			return Result{}, fmt.Errorf("load billing customer id: %w", err)
		}
		if customerID != "" {
			if err := d.Billing.DeleteCustomer(ctx, customerID); err != nil {
				return Result{}, fmt.Errorf("archive billing customer %s: %w", customerID, err)
			}
		}
	}

	if err := d.Accounts.MarkDeleted(acct.ID); err != nil {
		return Result{}, err
	}

	// Revoke the account-scoped judge key now, matching how undeploy revokes
	// deployment keys. It is long-lived, so leaving it until the purge would
	// keep a usable credential alive for the whole retention window.
	if d.AIGateway != nil && d.JudgeKeys != nil {
		if err := d.AIGateway.RevokeAccountJudgeKeys(ctx, d.JudgeKeys, acct.ID); err != nil {
			d.Log.Warn("account delete: revoke AI Gateway judge key failed", "error", err, "account_id", acct.ID)
		}
	}

	var result Result
	deps, err := d.Deployments.GetVisibleDeploymentsByAccount(acct.ID)
	if err != nil {
		d.Log.Error("account delete: list deployments failed", "error", err, "account_id", acct.ID)
	}
	for _, dep := range deps {
		if err := d.Undeploy(ctx, dep); err != nil {
			d.Log.Error("account delete: enqueue undeploy failed", "error", err, "deployment_id", dep.ID, "account_id", acct.ID)
			continue
		}
		result.DeploymentsUndeploying++
	}

	if d.Org != nil && acct.WorkOSOrganizationID != "" {
		if err := d.Org.DeleteOrganization(ctx, acct.WorkOSOrganizationID); err != nil {
			d.Log.Error("account delete: delete WorkOS organization failed", "error", err, "workos_org_id", acct.WorkOSOrganizationID, "account_id", acct.ID)
		}
	}

	d.Log.Info("account delete: account deleted", "account_id", acct.ID, "account_name", acct.Name, "deployments_undeploying", result.DeploymentsUndeploying)
	return result, nil
}
