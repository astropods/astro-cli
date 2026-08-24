package riverqueue

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/systemaudit"
)

const auditFindingRetention = 30 * 24 * time.Hour

type SystemAuditArgs struct{}

func (SystemAuditArgs) Kind() string { return "system.audit" }

func (SystemAuditArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMaintenance}
}

func init() {
	registerJobKind[SystemAuditArgs]()
}

type SystemAuditWorker struct {
	river.WorkerDefaults[SystemAuditArgs]
	audit *systemaudit.Store
	log   *logger.Logger
}

func (w *SystemAuditWorker) Work(ctx context.Context, _ *river.Job[SystemAuditArgs]) error {
	var open, resolved int
	var failed error
	for _, r := range w.audit.Run(ctx) {
		if r.Err != nil {
			w.log.Warn("system audit: check failed", "check", r.CheckName, "error", r.Err)
			failed = r.Err
			continue
		}
		open += r.Open
		resolved += r.Resolved
	}

	purged, err := w.audit.Purge(ctx, auditFindingRetention)
	if err != nil {
		w.log.Warn("system audit: purge resolved findings failed", "error", err)
	}

	w.log.Info("system audit: completed", "open", open, "resolved", resolved, "purged", purged)
	return failed
}
