package riverqueue

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
)

// HeartbeatArgs are the job arguments for the OpenMeter heartbeat worker.
type HeartbeatArgs struct{}

func (HeartbeatArgs) Kind() string { return "openmeter.heartbeat" }

// HeartbeatWorker emits periodic metering events via OpenMeter.
type HeartbeatWorker struct {
	river.WorkerDefaults[HeartbeatArgs]
	omClient *openmeter.Client
	db       *sql.DB
	log      *logger.Logger
}

func (w *HeartbeatWorker) Work(ctx context.Context, _ *river.Job[HeartbeatArgs]) error {
	hb := openmeter.NewHeartbeat(w.omClient, w.db, w.log)
	hb.Tick(ctx)
	return nil
}
