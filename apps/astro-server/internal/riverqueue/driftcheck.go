package riverqueue

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/driftcheck"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// DriftCheckArgs are the job arguments for the drift checker worker.
type DriftCheckArgs struct{}

func (DriftCheckArgs) Kind() string { return "driftcheck" }

// DriftCheckWorker compares desired deployment state against K8s and logs drift.
type DriftCheckWorker struct {
	river.WorkerDefaults[DriftCheckArgs]
	db        *sql.DB
	k8sClient k8s.ClusterClient
	log       *logger.Logger
}

func (w *DriftCheckWorker) Work(ctx context.Context, _ *river.Job[DriftCheckArgs]) error {
	if w.k8sClient == nil {
		w.log.Warn("Drift check skipped: K8s client unavailable")
		return nil
	}
	deployStore := deploymentstore.NewStore(w.db)
	dc := driftcheck.New(deployStore, w.k8sClient, w.log)
	report := dc.Check(ctx)
	dc.LogReport(report)
	return nil
}
