package openmeter

import (
	"context"

	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/logger"
)

const reconcileBatchSize = 50

// Reconciler backfills OpenMeter customers for existing accounts that don't have one.
// Runs once at startup, processing all accounts in batches.
type Reconciler struct {
	client       *Client
	accountStore *account.AccountStore
	log          *logger.Logger
}

// NewReconciler creates a new OpenMeter reconciler.
func NewReconciler(client *Client, accountStore *account.AccountStore, log *logger.Logger) *Reconciler {
	return &Reconciler{
		client:       client,
		accountStore: accountStore,
		log:          log,
	}
}

// Run backfills all accounts missing an OpenMeter customer. Processes in batches
// until no more accounts remain. Safe to call concurrently with account creation.
func (r *Reconciler) Run(ctx context.Context) {
	r.log.Info("OpenMeter backfill started")

	total := 0
	for {
		n := r.reconcileBatch(ctx)
		if n == 0 {
			break
		}
		total += n
	}

	r.log.Info("OpenMeter backfill complete", "synced", total)
}

// reconcileBatch processes one batch of accounts. Returns the number successfully synced.
func (r *Reconciler) reconcileBatch(ctx context.Context) int {
	accounts, err := r.accountStore.GetAccountsMissingOpenMeterCustomer(reconcileBatchSize)
	if err != nil {
		r.log.Error("OpenMeter backfill: failed to query accounts", "error", err)
		return 0
	}

	if len(accounts) == 0 {
		return 0
	}

	synced := 0
	for _, acct := range accounts {
		customerID, err := r.client.CreateCustomer(ctx, acct.ID, acct.Name, acct.Type, "")
		if err != nil {
			r.log.Error("OpenMeter backfill: failed to create customer",
				"error", err,
				"account_id", acct.ID,
				"account_name", acct.Name,
			)
			continue
		}

		if err := r.accountStore.SetOpenMeterCustomerID(acct.ID, customerID); err != nil {
			r.log.Error("OpenMeter backfill: failed to store customer ID",
				"error", err,
				"account_id", acct.ID,
			)
			continue
		}

		synced++
	}

	return synced
}
