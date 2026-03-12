package riverqueue

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/nsscan"
)

// NsScanArgs are the job arguments for the namespace scanner worker.
type NsScanArgs struct{}

func (NsScanArgs) Kind() string { return "nsscan" }

// NsScanWorker reconciles DB deployments against K8s namespaces.
type NsScanWorker struct {
	river.WorkerDefaults[NsScanArgs]
	db        *sql.DB
	k8sClient k8s.ClusterClient
	log       *logger.Logger
}

func (w *NsScanWorker) Work(ctx context.Context, _ *river.Job[NsScanArgs]) error {
	scanner := nsscan.New(w.db, w.k8sClient, w.log)
	result, err := scanner.Scan(ctx)
	if err != nil {
		return err
	}
	scanner.LogResult(result)
	return nil
}
