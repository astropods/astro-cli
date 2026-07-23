package riverqueue

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/billing/metering"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// MeteringArgs are the job arguments for the metering heartbeat worker.
type MeteringArgs struct{}

func (MeteringArgs) Kind() string { return "metering.heartbeat" }

func (MeteringArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMetering}
}

func init() {
	registerJobKind[MeteringArgs]()
}

// MeteringWorker emits periodic metering events via the active billing provider.
type MeteringWorker struct {
	river.WorkerDefaults[MeteringArgs]
	provider billing.BillingProvider
	db       *sql.DB
	log      *logger.Logger
	billing  *metering.BillingStateManager
}

func (w *MeteringWorker) Work(ctx context.Context, _ *river.Job[MeteringArgs]) error {
	hb := metering.NewHeartbeat(w.provider, w.db, w.log, w.billing)
	hb.Tick(ctx)
	return nil
}
