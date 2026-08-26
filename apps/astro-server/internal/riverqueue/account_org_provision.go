package riverqueue

import (
	"context"
	"errors"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/org"
)

const orgProvisionSweepLimit = 200

type AccountOrgProvisionArgs struct {
	AccountID string `json:"account_id" river:"unique"`
}

func (AccountOrgProvisionArgs) Kind() string { return "account.org_provision" }

func (AccountOrgProvisionArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: queueMaintenance,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
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

type AccountOrgProvisionSweepArgs struct{}

func (AccountOrgProvisionSweepArgs) Kind() string { return "account.org_provision_sweep" }

func (AccountOrgProvisionSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMaintenance}
}

func init() {
	registerJobKind[AccountOrgProvisionArgs]()
	registerJobKind[AccountOrgProvisionSweepArgs]()
}

type AccountOrgProvisionWorker struct {
	river.WorkerDefaults[AccountOrgProvisionArgs]
	provisioner *org.Provisioner
	log         *logger.Logger
}

func (w *AccountOrgProvisionWorker) Work(ctx context.Context, job *river.Job[AccountOrgProvisionArgs]) error {
	if w.provisioner == nil {
		return nil
	}
	orgID, err := w.provisioner.EnsureOrganization(ctx, job.Args.AccountID)
	if err != nil {
		if errors.Is(err, account.ErrAccountNotFound) {
			w.log.Warn("org provision: account not found", "account_id", job.Args.AccountID)
			return nil
		}
		return err
	}
	w.log.Info("org provision: completed", "account_id", job.Args.AccountID, "workos_org_id", orgID)
	return nil
}

type orgProvisionEnqueuer interface {
	InsertAccountOrgProvision(ctx context.Context, accountID string) error
}

type AccountOrgProvisionSweepWorker struct {
	river.WorkerDefaults[AccountOrgProvisionSweepArgs]
	accounts *account.AccountStore
	queue    orgProvisionEnqueuer
	log      *logger.Logger
}

func (w *AccountOrgProvisionSweepWorker) Work(ctx context.Context, _ *river.Job[AccountOrgProvisionSweepArgs]) error {
	if w.accounts == nil || w.queue == nil {
		return nil
	}
	pending, err := w.accounts.GetAccountsPendingOrgProvision(orgProvisionSweepLimit)
	if err != nil {
		return err
	}
	for _, a := range pending {
		if err := w.queue.InsertAccountOrgProvision(ctx, a.ID); err != nil {
			w.log.Error("org provision sweep: enqueue failed", "account_id", a.ID, "error", err)
		}
	}
	if len(pending) > 0 {
		w.log.Info("org provision: sweep", "enqueued", len(pending), "limit", orgProvisionSweepLimit)
	}
	return nil
}
