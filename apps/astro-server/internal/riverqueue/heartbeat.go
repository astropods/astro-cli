package riverqueue

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
)

// OpenmeterArgs are the job arguments for the OpenMeter heartbeat worker.
type OpenmeterArgs struct{}

func (OpenmeterArgs) Kind() string { return "openmeter.heartbeat" }

// OpenmeterWorker emits periodic metering events via OpenMeter.
type OpenmeterWorker struct {
	river.WorkerDefaults[OpenmeterArgs]
	omClient *openmeter.Client
	db       *sql.DB
	log      *logger.Logger
}

func (w *OpenmeterWorker) Work(ctx context.Context, _ *river.Job[OpenmeterArgs]) error {
	hb := openmeter.NewHeartbeat(w.omClient, w.db, w.log)
	hb.Tick(ctx)
	return nil
}
