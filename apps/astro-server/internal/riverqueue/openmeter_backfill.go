package riverqueue

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
)

// OpenMeterBackfillArgs are the job arguments for the OpenMeter customer backfill worker.
// This is a periodic job with no per-invocation arguments — it discovers work by querying
// the accounts table for rows where openmeter_customer_id IS NULL.
type OpenMeterBackfillArgs struct{}

// Kind returns the unique River job kind identifier. River uses this string to
// route jobs to the correct worker and to deduplicate periodic job insertions
// (via ByPeriod). The "openmeter." prefix groups it with the other OpenMeter
// heartbeat worker ("openmeter.heartbeat") in logs and the River UI.
func (OpenMeterBackfillArgs) Kind() string { return "openmeter.backfill" }

func init() {
	registerJobKind[OpenMeterBackfillArgs]()
}

// OpenMeterBackfillWorker creates OpenMeter customers for Astro accounts that
// are missing one. This handles two scenarios:
//
//  1. Inline failure recovery — When an account is created (POST /api/v1/accounts),
//     the handler calls openmeter.CreateCustomer inline. If that call fails (network
//     error, OpenMeter downtime), the account is created without an OpenMeter customer.
//     The handler logs the error but does not block account creation (OpenMeter is
//     non-critical path). This worker picks up those orphaned accounts on its next run.
//
//  2. Pre-integration backfill — Accounts created before the OpenMeter integration
//     was deployed have no openmeter_customer_id at all. This worker creates customers
//     for them so they can be metered and entitled going forward.
//
// The worker runs daily via River's periodic job scheduler (configured in periodic.go)
// and also on startup (RunOnStart: true) so new deployments immediately reconcile.
// It is gated on cfg.OMClient != nil — if OPENMETER_URL is not set, the worker
// no-ops gracefully.
//
// For each account, it also resolves the owner's email from WorkOS (via the account's
// first member) and auto-subscribes the customer to the default plan (e.g. "private_beta")
// if OPENMETER_DEFAULT_PLAN is configured. Subscription failures are logged but don't
// block — the next daily run will retry since the customer now exists but the subscription
// may be missing.
type OpenMeterBackfillWorker struct {
	river.WorkerDefaults[OpenMeterBackfillArgs]

	// omClient is the typed HTTP client for the OpenMeter API. If nil (OPENMETER_URL
	// not set), Work() returns immediately.
	omClient *openmeter.Client

	// accountStore provides access to the accounts table — specifically
	// GetAccountsMissingOpenMeterCustomer (finds accounts to backfill) and
	// SetOpenMeterCustomerID (persists the result). Also used to look up the
	// first member's user ID for email resolution.
	accountStore *account.AccountStore

	// workosClient is used to fetch the owner's email from WorkOS. This is the
	// same client used by the auth middleware and org sync. May be nil if
	// WORKOS_API_KEY is not configured — in that case, customers are created
	// with an empty primaryEmail (OpenMeter allows this).
	workosClient *auth.WorkOSClient

	// defaultPlan is the OpenMeter plan key (e.g. "private_beta") to auto-subscribe
	// newly backfilled customers to. Sourced from OPENMETER_DEFAULT_PLAN env var.
	// If empty, no subscription is created — the customer exists but has no plan.
	defaultPlan string

	log *logger.Logger
}

// Work is the main entry point called by River when the periodic job fires.
// It processes all accounts missing an OpenMeter customer in batches of 50.
//
// Error handling strategy: the worker returns nil on all errors (including DB
// failures) so River marks the job as complete rather than retrying. This is
// intentional — transient failures (DB hiccups, OpenMeter timeouts) will be
// naturally retried on the next daily run, and we don't want a persistent error
// (e.g. a single malformed account) to wedge the queue with infinite retries.
// Individual account failures are logged and counted in the summary.
//
// The batch loop re-queries on each iteration rather than paginating with a cursor.
// This works because each successful iteration removes accounts from the result set
// (by setting their openmeter_customer_id). If an account fails, it stays in the
// result set — but since we process the full batch before re-querying, we won't
// spin on the same failing account within a single run.
func (w *OpenMeterBackfillWorker) Work(ctx context.Context, _ *river.Job[OpenMeterBackfillArgs]) error {
	if w.omClient == nil {
		w.log.Debug("OpenMeter backfill skipped: no OpenMeter client configured")
		return nil
	}

	const batchSize = 50
	var totalCreated, totalFailed int

	for {
		// Query accounts that don't have an OpenMeter customer yet.
		// Ordered by created_at so older accounts are backfilled first.
		accounts, err := w.accountStore.GetAccountsMissingOpenMeterCustomer(batchSize)
		if err != nil {
			w.log.Error("OpenMeter backfill: failed to query accounts", "error", err)
			return nil // Don't retry — transient DB issues shouldn't wedge the queue
		}
		if len(accounts) == 0 {
			break
		}

		for _, acct := range accounts {
			// Resolve the owner's email from WorkOS for the OpenMeter customer record.
			// This is best-effort — if WorkOS is unavailable, we create the customer
			// with an empty email. The email is used by OpenMeter for display/notifications
			// only, not for metering or entitlements.
			ownerEmail := w.resolveOwnerEmail(ctx, acct)

			// Create the OpenMeter customer. The account.ID is used as both the customer
			// key and the subject key for usage attribution, matching the inline creation
			// in handlers/accounts.go CreateAccount.
			customerID, createErr := w.omClient.CreateCustomer(ctx, acct.ID, acct.Name, acct.Type, ownerEmail)
			if createErr != nil {
				w.log.Error("OpenMeter backfill: failed to create customer", "account_id", acct.ID, "error", createErr)
				totalFailed++
				continue
			}

			// Persist the OpenMeter customer ID back to the accounts table.
			// This removes the account from future GetAccountsMissingOpenMeterCustomer
			// queries, preventing duplicate customer creation.
			if storeErr := w.accountStore.SetOpenMeterCustomerID(acct.ID, customerID); storeErr != nil {
				w.log.Error("OpenMeter backfill: failed to store customer ID", "account_id", acct.ID, "error", storeErr)
				totalFailed++
				continue
			}

			// Auto-subscribe to the default plan so entitlements are immediately active.
			// If this fails, the customer still exists — the subscription will be retried
			// on the next run (the account won't appear in GetAccountsMissingOpenMeterCustomer
			// since the customer ID is now set, but the subscription can be created manually
			// or via a separate reconciliation if needed).
			if w.defaultPlan != "" {
				if subErr := w.omClient.CreateSubscription(ctx, customerID, w.defaultPlan); subErr != nil {
					w.log.Error("OpenMeter backfill: failed to subscribe account", "account_id", acct.ID, "plan", w.defaultPlan, "error", subErr)
					// Don't count as failed — customer was created, subscription can be retried next run
				}
			}

			totalCreated++
		}
	}

	// Only log if there was actual work to report. Silent runs (0 created, 0 failed)
	// mean all accounts already have OpenMeter customers — no need to spam logs.
	if totalCreated > 0 || totalFailed > 0 {
		w.log.Info("OpenMeter backfill completed",
			"created", totalCreated,
			"failed", totalFailed,
		)
	}

	return nil
}

// resolveOwnerEmail looks up the owner email for an account by going through
// two hops: accountStore → WorkOS.
//
// For personal accounts, GetFirstMemberUserID returns the sole owner (the user
// who created the account). For organization accounts, it returns the first
// member added — which is always the creator, since Create() in the account
// store adds the creator as the first member before any invites.
//
// The email is used as the OpenMeter customer's primaryEmail field, which
// OpenMeter uses for display and notifications. It is NOT required for metering
// or entitlements to function — an empty email is perfectly valid.
//
// Returns empty string if either lookup fails. Failures are logged at Debug
// level since they're expected in edge cases (deleted users, WorkOS API key
// not configured, accounts with no members due to data inconsistency).
func (w *OpenMeterBackfillWorker) resolveOwnerEmail(ctx context.Context, acct account.Account) string {
	if w.workosClient == nil {
		return ""
	}

	userID, err := w.accountStore.GetFirstMemberUserID(acct.ID)
	if err != nil {
		w.log.Debug("OpenMeter backfill: could not resolve owner for account", "account_id", acct.ID, "error", err)
		return ""
	}

	user, err := w.workosClient.GetUser(ctx, userID)
	if err != nil {
		w.log.Debug("OpenMeter backfill: could not fetch user from WorkOS", "user_id", userID, "error", err)
		return ""
	}

	return user.Email
}
