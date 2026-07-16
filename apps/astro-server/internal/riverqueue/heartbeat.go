package riverqueue

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/billing/openmeter"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// OpenmeterArgs are the job arguments for the OpenMeter heartbeat worker.
type OpenmeterArgs struct{}

func (OpenmeterArgs) Kind() string { return "openmeter.heartbeat" }

func init() {
	registerJobKind[OpenmeterArgs]()
}

// OpenmeterWorker emits periodic metering events via OpenMeter.
type OpenmeterWorker struct {
	river.WorkerDefaults[OpenmeterArgs]
	provider billing.BillingProvider
	db       *sql.DB
	log      *logger.Logger
	billing  *openmeter.BillingStateManager
}

func (w *OpenmeterWorker) Work(ctx context.Context, _ *river.Job[OpenmeterArgs]) error {
	hb := openmeter.NewHeartbeat(w.provider, w.db, w.log, w.billing)
	hb.Tick(ctx)
	return nil
}
