package riverqueue

import (
	"context"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// provisionSweepLimit caps accounts enqueued per tick.
const provisionSweepLimit = 200

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
	accounts         *account.AccountStore
	provider         billing.BillingProvider
	backend          string
	status           *billing.StatusStore
	unlimitedDomains []string
	queue            *Queue // set post-construction in New(); enqueues resume
	log              *logger.Logger
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
	if p, ok := w.provider.(billing.Provisioner); ok {
		plan, err := w.plan(acct.ID)
		if err != nil {
			return err
		}
		provisioned, err := p.ProvisionCustomer(ctx, customerID, acct.ID, plan)
		if err != nil {
			return err
		}
		// Leaving it unmarked keeps it in the sweep, so it provisions once the
		// config lands.
		if !provisioned {
			w.log.Info("billing provision skipped: provider not configured", "account_id", acct.ID)
			return nil
		}
	}
	if err := w.accounts.MarkBillingProvisioned(acct.ID); err != nil {
		return err
	}

	// The account now holds credit, so lift any exhaustion latch and reconcile.
	// Metronome's resolved alert clears it too; this tail is what an operator
	// credit grant re-runs the job for, and the backstop when the alert stays
	// IN_ALARM because the balance never crossed back.
	if w.status != nil {
		newStatus, changed, err := billing.ApplySignal(ctx, w.status, acct.ID, billing.SignalCreditsGranted, time.Now())
		if err != nil {
			return err
		}
		if changed {
			w.log.Info("billing status changed", "source", "provision", "account_id", acct.ID, "status", string(newStatus))
		}
		if err := reconcileWorkloads(ctx, w.queue, acct.ID, newStatus); err != nil {
			return err
		}
	}

	w.log.Info("billing provisioned", "account_id", acct.ID, "customer_id", customerID)
	return nil
}

// plan resolves the rate treatment. An internal creator is answered before the
// ledger, so the plan does not spend the person's one claim on an account that
// has no use for it.
func (w *BillingProvisionWorker) plan(accountID string) (billing.Plan, error) {
	creatorEmail, err := w.accounts.GetCreatorVerifiedEmail(accountID)
	if err != nil {
		return "", err
	}
	if hasEmailDomain(creatorEmail, w.unlimitedDomains) {
		return billing.PlanUnlimited, nil
	}
	withCredit, err := w.claimSignupCredit(accountID)
	if err != nil {
		return "", err
	}
	if withCredit {
		return billing.PlanCredit, nil
	}
	return billing.PlanNoCredit, nil
}

// hasEmailDomain compares the part after the last "@" for equality, so neither a
// subdomain nor a lookalike like evil-postman.com matches.
func hasEmailDomain(email string, domains []string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	got := strings.ToLower(email[at+1:])
	if got == "" {
		return false
	}
	for _, want := range domains {
		if got == strings.ToLower(strings.TrimSpace(want)) {
			return true
		}
	}
	return false
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
		w.log.Info("billing provision sweep", "enqueued", len(pending))
	}
	return nil
}
