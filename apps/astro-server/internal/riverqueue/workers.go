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
}
