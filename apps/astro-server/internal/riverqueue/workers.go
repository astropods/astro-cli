package riverqueue

import (
	"github.com/riverqueue/river"
	"k8s.io/client-go/dynamic"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

// addWorkers registers all River workers into the registry.
// Returns the ReconcileWorker so the caller can set its queue reference after client creation.
func addWorkers(workers *river.Workers, cfg Config) *ReconcileWorker {
	river.AddWorker(workers, &HeartbeatWorker{
		omClient: cfg.OMClient,
		db:       cfg.DB,
		log:      cfg.Logger,
	})
	river.AddWorker(workers, &WorkOSEventsWorker{
		workOSAPIKey: cfg.WorkOSAPIKey,
		orgClient:    cfg.OrgClient,
		accountStore: cfg.AccountStore,
		db:           cfg.DB,
		log:          cfg.Logger,
	})

	store := deploymentstore.NewStore(cfg.DB)

	var dep *deployer.Deployer
	if cfg.K8sClient != nil && cfg.ServerConfig != nil {
		dep = &deployer.Deployer{
			K8sClient:    cfg.K8sClient,
			AccountStore: cfg.AccountStore,
			Cfg:          cfg.ServerConfig,
			Store:        store,
			Log:          cfg.Logger,
		}
	}

	river.AddWorker(workers, &DeployWorker{deployer: dep, store: store, log: cfg.Logger})
	river.AddWorker(workers, &UndeployWorker{deployer: dep, store: store, log: cfg.Logger})
	river.AddWorker(workers, &WakeUpWorker{deployer: dep, store: store, log: cfg.Logger})

	var dynClient dynamic.Interface
	if cfg.K8sClient != nil {
		dynClient, _ = dynamic.NewForConfig(cfg.K8sClient.Config())
	}

	rw := &ReconcileWorker{
		deployer:  dep,
		store:     store,
		k8s:       cfg.K8sClient,
		dynClient: dynClient,
		db:        cfg.DB,
		log:       cfg.Logger,
		// queue is set after client creation in New()
	}
	river.AddWorker(workers, rw)

	return rw
}
