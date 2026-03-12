package riverqueue

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
)

// ReconcilerArgs are the job arguments for the OpenMeter reconciler worker.
type ReconcilerArgs struct{}

func (ReconcilerArgs) Kind() string { return "openmeter.reconciler" }

// ReconcilerWorker backfills OpenMeter customers for accounts missing one.
type ReconcilerWorker struct {
	river.WorkerDefaults[ReconcilerArgs]
	omClient     *openmeter.Client
	accountStore *account.AccountStore
	log          *logger.Logger
}

func (w *ReconcilerWorker) Work(ctx context.Context, _ *river.Job[ReconcilerArgs]) error {
	reconciler := openmeter.NewReconciler(w.omClient, w.accountStore, w.log)
	reconciler.Run(ctx)
	return nil
}
