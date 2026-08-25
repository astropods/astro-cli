package riverqueue

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// provisionSweepLimit caps accounts enqueued per tick.
const provisionSweepLimit = 200

// defaultSpendLimitCents is $20 in the cents the provider stores thresholds in.
const defaultSpendLimitCents = 2000

// BillingProvisionArgs puts one account on the rate card and grants its signup
// credit.
type BillingProvisionArgs struct {
	AccountID string `json:"account_id" river:"unique"`
}

func (BillingProvisionArgs) Kind() string { return "billing.provision" }

// InsertOpts dedupes by account so the sweep and the signup path can both
// enqueue without racing to create two provider customers (CreateCustomer has
// no uniqueness key of its own, unlike the contract and grant).
func (BillingProvisionArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: queueBilling,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			// River's default set includes completed, which would make an
			// already-run job block the hourly re-enqueue until the cleaner
			// removes it. An account left unprovisioned because the provider
			// wasn't configured has to be retryable on the next sweep.
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
	}
}

// BillingProvisionSweepArgs enqueues provisioning for accounts still pending.
type BillingProvisionSweepArgs struct{}

func (BillingProvisionSweepArgs) Kind() string { return "billing.provision_sweep" }

func (BillingProvisionSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueBilling}
}

func init() {
	registerJobKind[BillingProvisionArgs]()
	registerJobKind[BillingProvisionSweepArgs]()
}

// BillingProvisionWorker ensures an account has a billing customer, then puts it
// on the rate card and grants signup credit.
type BillingProvisionWorker struct {
	river.WorkerDefaults[BillingProvisionArgs]
	accounts *account.AccountStore
	provider billing.BillingProvider
	backend  string
	status   *billing.StatusStore
	queue    *Queue // set post-construction in New(); enqueues resume
	log      *logger.Logger
}

func (w *BillingProvisionWorker) Work(ctx context.Context, job *river.Job[BillingProvisionArgs]) error {
	if w.accounts == nil || w.provider == nil {
		return nil
	}
	acct, err := w.accounts.GetByID(job.Args.AccountID)
	if err != nil {
		// A deleted account is permanent; acking is correct.
		w.log.Warn("billing provision: account not found", "account_id", job.Args.AccountID, "error", err)
		return nil
	}

	customerID, err := w.accounts.GetBillingCustomerID(acct.ID, w.backend)
	if err != nil {
		return err
	}
	if customerID == "" {
		bifrostID, _ := w.accounts.GetBifrostCustomerID(acct.ID)
		// A missing owner email reads as "", but a failed lookup must not
		// silently create the customer without one — there is no key to
		// correct it by afterwards.
		ownerEmail, err := w.accounts.GetOwnerEmail(acct.ID)
		if err != nil {
			return err
		}
		customerID, err = w.provider.CreateCustomer(ctx, billing.Account{
			ID:                acct.ID,
			Name:              acct.Name,
			Type:              acct.Type,
			OwnerEmail:        ownerEmail,
			BifrostCustomerID: bifrostID,
		})
		if err != nil {
			return err
		}
		if err := w.accounts.SetBillingCustomerID(acct.ID, w.backend, customerID); err != nil {
			return err
		}
	}

	// Provisioning is optional on the seam; backends without it are done here.
	// The plan escapes the branch because it decides the credit latch below, and
	// it stays empty for a backend that chooses no plan.
	var plan billing.Plan
	if p, ok := w.provider.(billing.Provisioner); ok {
		var err error
		plan, err = w.plan(acct.ID)
		if err != nil {
			return err
		}
		provisioned, err := p.ProvisionCustomer(ctx, customerID, acct.ID, plan)
		if err != nil {
			if serr := w.syncCoverage(ctx, acct.ID, customerID); serr != nil {
				w.log.Error("billing provision: coverage sync failed", "account_id", acct.ID, "error", serr)
			}
			return err
		}
		// Leaving it unmarked keeps it in the sweep, so it provisions once the
		// config lands.
		if !provisioned {
			w.log.Info("billing provision skipped: provider not configured", "account_id", acct.ID)
			return nil
		}
	}
	// Read before the stamp, so a re-provision does not reimpose a cleared cap.
	seeded, err := w.accounts.IsBillingProvisioned(acct.ID)
	if err != nil {
		return err
	}
	if !seeded {
		if err := w.seedSpendLimit(ctx, acct.ID, customerID, plan); err != nil {
			return err
		}
	}
	if err := w.accounts.MarkBillingProvisioned(acct.ID); err != nil {
		return err
	}
	if err := w.syncCoverage(ctx, acct.ID, customerID); err != nil {
		return err
	}

	// Set the credit latch from the plan the account was just put on. A plan
	// that grants no credit has no balance for the provider's low-balance alert
	// to fire on, so nothing else would ever raise it, and the account would run
	// with neither credit nor a card. Lifting it here instead is what an
	// operator credit grant re-runs this job for, and the backstop when the
	// alert stays IN_ALARM because the balance never crossed back.
	if w.status != nil {
		newStatus, changed, err := billing.ApplySignal(ctx, w.status, acct.ID, creditSignal(plan), time.Now())
		if err != nil {
			return err
		}
		if changed {
			w.log.Info("billing provision: billing status changed", "source", "provision", "account_id", acct.ID, "status", string(newStatus))
		}
		if err := reconcileWorkloads(ctx, w.queue, acct.ID, newStatus); err != nil {
			return err
		}
	}

	w.log.Info("billing provision: completed", "account_id", acct.ID, "customer_id", customerID)
	return nil
}

func (w *BillingProvisionWorker) seedSpendLimit(ctx context.Context, accountID, customerID string, plan billing.Plan) error {
	writer, ok := w.provider.(billing.SpendThresholdWriter)
	if !ok || customerID == "" {
		return nil
	}
	if err := writer.SetCustomerSpendThreshold(ctx, customerID, billing.SpendThresholdLimit, defaultSpendLimitCents); err != nil {
		return fmt.Errorf("seed default spend limit: %w", err)
	}
	w.log.Info("billing default spend limit set", "account_id", accountID, "cents", defaultSpendLimitCents)
	return nil
}

// creditSignal is the latch a freshly provisioned account takes. Only the
// no-credit plan starts without a balance, and only it has no low-balance alert
// coming to raise the latch later.
func creditSignal(plan billing.Plan) billing.Signal {
	if plan == billing.PlanNoCredit {
		return billing.SignalCreditsExhausted
	}
	return billing.SignalCreditsGranted
}

// syncCoverage gates the account on whether a contract covers it. Provisioning
// reporting success is a different fact: a contract can be ended afterwards. It
// asks the same method the billing read asks, so the two cannot disagree on what
// covered means.
func (w *BillingProvisionWorker) syncCoverage(ctx context.Context, accountID, customerID string) error {
	planner, ok := w.provider.(billing.PlanReporter)
	if !ok || w.status == nil || customerID == "" {
		return nil
	}
	_, covered, err := planner.CustomerPlan(ctx, customerID)
	if err != nil {
		return fmt.Errorf("read billing coverage: %w", err)
	}
	sig := billing.SignalProvisioned
	if !covered {
		sig = billing.SignalNotProvisioned
	}
	newStatus, changed, err := billing.ApplySignal(ctx, w.status, accountID, sig, time.Now())
	if err != nil {
		return err
	}
	if changed {
		w.log.Info("billing provision: billing status changed", "source", "coverage", "account_id", accountID, "status", string(newStatus))
	}
	return reconcileWorkloads(ctx, w.queue, accountID, newStatus)
}

func (w *BillingProvisionWorker) plan(accountID string) (billing.Plan, error) {
	withCredit, err := w.claimSignupCredit(accountID)
	if err != nil {
		return "", err
	}
	if withCredit {
		return billing.PlanCredit, nil
	}
	return billing.PlanNoCredit, nil
}

// claimSignupCredit reports whether this account should be provisioned with the
// signup credit.
//
// The credit belongs to a person. Nothing caps how many accounts one user
// creates, and every account would otherwise carry its own grant, so a user
// could mint organizations for free credit indefinitely. The claim is taken
// against the creator and outlives the account, which also closes the same farm
// run by deleting and recreating.
//
// An account whose creator cannot be resolved is provisioned without credit. A
// grant that cannot be attributed to anyone is the case this guards against, so
// the safe answer is to withhold it and let an operator grant it by hand.
func (w *BillingProvisionWorker) claimSignupCredit(accountID string) (bool, error) {
	creator, err := w.accounts.GetCreatorUserID(accountID)
	if err != nil {
		w.log.Warn("billing provision: no creator resolved, provisioning without signup credit",
			"account_id", accountID, "error", err)
		return false, nil
	}
	claimed, err := w.accounts.ClaimSignupCredit(creator, accountID)
	if err != nil {
		return false, err
	}
	if !claimed {
		w.log.Info("billing provision: signup credit already taken by this user",
			"account_id", accountID, "user_id", creator)
	}
	return claimed, nil
}

// BillingProvisionSweepWorker backfills accounts that never got provisioned —
// created before provisioning existed, or whose signup enqueue was dropped.
type BillingProvisionSweepWorker struct {
	river.WorkerDefaults[BillingProvisionSweepArgs]
	accounts *account.AccountStore
	queue    *Queue // set post-construction in New()
	log      *logger.Logger
}

func (w *BillingProvisionSweepWorker) Work(ctx context.Context, _ *river.Job[BillingProvisionSweepArgs]) error {
	if w.accounts == nil || w.queue == nil {
		return nil
	}
	pending, err := w.accounts.GetAccountsPendingBillingProvision(provisionSweepLimit)
	if err != nil {
		return err
	}
	for _, a := range pending {
		if err := w.queue.InsertBillingProvision(ctx, a.ID); err != nil {
			w.log.Error("billing provision sweep: enqueue failed", "account_id", a.ID, "error", err)
		}
	}
	if len(pending) > 0 {
		w.log.Info("billing provision: sweep", "enqueued", len(pending))
	}
	return nil
}
