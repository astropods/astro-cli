package riverqueue

import (
	"github.com/riverqueue/river"
)

// addWorkers registers all River workers into the registry.
func addWorkers(workers *river.Workers, cfg Config) {
	river.AddWorker(workers, &HeartbeatWorker{
		omClient: cfg.OMClient,
		db:       cfg.DB,
		log:      cfg.Logger,
	})
	river.AddWorker(workers, &ReconcilerWorker{
		omClient:     cfg.OMClient,
		accountStore: cfg.AccountStore,
		log:          cfg.Logger,
	})
	river.AddWorker(workers, &DriftCheckWorker{
		db:        cfg.DB,
		k8sClient: cfg.K8sClient,
		log:       cfg.Logger,
	})
	river.AddWorker(workers, &NsScanWorker{
		db:        cfg.DB,
		k8sClient: cfg.K8sClient,
		log:       cfg.Logger,
	})
}
