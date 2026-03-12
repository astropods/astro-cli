package riverqueue

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/org"
)

const queueWorkOS = "workos"

// WorkOSEventsArgs are the job arguments for the WorkOS events consumer worker.
type WorkOSEventsArgs struct{}

func (WorkOSEventsArgs) Kind() string { return "workos.events" }
func (WorkOSEventsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueWorkOS}
}

// WorkOSEventsWorker polls the WorkOS Events API and processes membership changes.
type WorkOSEventsWorker struct {
	river.WorkerDefaults[WorkOSEventsArgs]
	workOSAPIKey string
	orgClient    *org.Client
	accountStore *account.AccountStore
	db           *sql.DB
	log          *logger.Logger
}

func (w *WorkOSEventsWorker) Work(ctx context.Context, _ *river.Job[WorkOSEventsArgs]) error {
	consumer := org.NewEventsConsumer(w.workOSAPIKey, w.orgClient, w.accountStore, w.db, w.log, 0)
	consumer.Poll(ctx)
	return nil
}
