package riverqueue

import (
	"context"

	"github.com/riverqueue/river"
	"k8s.io/client-go/dynamic"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

// addWorkers registers all River workers into the registry.
// Returns the ReconcileWorker and AccountPurgeWorker so the caller can set
// their queue references after client creation.
func addWorkers(workers *river.Workers, cfg Config) (*ReconcileWorker, *AccountPurgeWorker) {
	log := cfg.Logger

	river.AddWorker(workers, &OpenmeterWorker{
		omClient: cfg.OMClient,
		db:       cfg.DB,
		log:      log,
	})
	log.Info("river: registered worker", "worker", "OpenmeterWorker", "period", "5m")

	river.AddWorker(workers, &WorkOSEventsWorker{
		workOSAPIKey: cfg.WorkOSAPIKey,
		orgClient:    cfg.OrgClient,
		accountStore: cfg.AccountStore,
		agentIdx:     cfg.AgentIndex,
		avatarStore:  cfg.AvatarStore,
		db:           cfg.DB,
		log:          log,
	})
	log.Info("river: registered worker", "worker", "WorkOSEventsWorker", "period", "15s")

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

		// Initialize Langfuse per-account provisioning if configured
		if cfg.ServerConfig.Deployment.LangfuseDBURL != "" {
			dep.LangfuseStore = langfuse.NewStore(cfg.DB)
			prov, provErr := langfuse.NewProvisioner(
				cfg.ServerConfig.Deployment.LangfuseDBURL,
				cfg.ServerConfig.Deployment.LangfuseSalt,
				cfg.ServerConfig.Deployment.LangfuseOrgID,
			)
			if provErr != nil {
				cfg.Logger.Warn("Failed to initialize Langfuse provisioner", "error", provErr)
			} else {
				dep.LangfuseProvisioner = prov
			}
		}
	}

	river.AddWorker(workers, &DeployWorker{deployer: dep, store: store, log: log, cache: cfg.K8sCache})
	log.Info("river: registered worker", "worker", "DeployWorker")
	river.AddWorker(workers, &UndeployWorker{deployer: dep, store: store, log: log, cache: cfg.K8sCache})
	log.Info("river: registered worker", "worker", "UndeployWorker")
	river.AddWorker(workers, &WakeUpWorker{deployer: dep, store: store, log: log, cache: cfg.K8sCache})
	log.Info("river: registered worker", "worker", "WakeUpWorker")

	var dynClient dynamic.Interface
	if cfg.K8sClient != nil {
		dynClient, _ = dynamic.NewForConfig(cfg.K8sClient.Config())
	}

	river.AddWorker(workers, &MessageCountSyncWorker{
		promClient:   cfg.PromClient,
		accountStore: cfg.AccountStore,
		db:           cfg.DB,
		log:          log,
	})
	log.Info("river: registered worker", "worker", "MessageCountSyncWorker", "period", "5m")

	river.AddWorker(workers, &AvatarBackfillWorker{
		avatarStore: cfg.AvatarStore,
		db:          cfg.DB,
		log:         log,
	})
	log.Info("river: registered worker", "worker", "AvatarBackfillWorker", "period", "24h")

	river.AddWorker(workers, &BlueprintAvatarBackfillWorker{
		avatarStore: cfg.AvatarStore,
		db:          cfg.DB,
		log:         log,
	})
	log.Info("river: registered worker", "worker", "BlueprintAvatarBackfillWorker", "period", "24h")

	var omDefaultPlan string
	if cfg.ServerConfig != nil {
		omDefaultPlan = cfg.ServerConfig.OpenMeterDefaultPlan
	}
	river.AddWorker(workers, &OpenMeterBackfillWorker{
		omClient:     cfg.OMClient,
		accountStore: cfg.AccountStore,
		workosClient: cfg.WorkOSClient,
		defaultPlan:  omDefaultPlan,
		log:          log,
	})
	log.Info("river: registered worker", "worker", "OpenMeterBackfillWorker", "period", "24h")

	// Account purge worker — needs langfuse provisioner/store from deployer (if available)
	pw := &AccountPurgeWorker{
		db:            cfg.DB,
		deployStore:   store,
		omClient:      cfg.OMClient,
		retentionDays: cfg.AccountRetentionDays,
		log:           log,
	}
	if dep != nil {
		pw.lfProvisioner = dep.LangfuseProvisioner
		pw.lfStore = dep.LangfuseStore
	}
	// enqueueUndeploy is set after client creation in New() via SetPurgeQueue
	river.AddWorker(workers, pw)
	log.Info("river: registered worker", "worker", "AccountPurgeWorker", "period", "1h")

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
	log.Info("river: registered worker", "worker", "ReconcileWorker", "period", "10m")

	ksStoreForWorkers := knowledgestore.NewStore(cfg.DB)
	river.AddWorker(workers, &KnowledgeReconcileWorker{
		ksStore: ksStoreForWorkers,
		k8s:     cfg.K8sClient,
		log:     cfg.Logger,
	})
	log.Info("river: registered worker", "worker", "KnowledgeReconcileWorker", "period", "30s")

	river.AddWorker(workers, &PrivateLinkProvisionWorker{
		ksStore: ksStoreForWorkers,
		cfg:     cfg.ServerConfig,
		log:     cfg.Logger,
	})
	log.Info("river: registered worker", "worker", "PrivateLinkProvisionWorker")

	river.AddWorker(workers, &PrivateLinkDeleteWorker{
		ksStore: ksStoreForWorkers,
		log:     cfg.Logger,
	})
	log.Info("river: registered worker", "worker", "PrivateLinkDeleteWorker")

	if cfg.PipesClient != nil && cfg.GitHubStore != nil && cfg.AgentIndex != nil {
		ghBuildWorker := NewGitHubBuildWorker(cfg.PipesClient, cfg.GitHubStore, cfg.AgentIndex, cfg.K8sClient, cfg.ServerConfig, log)
		if err := ghBuildWorker.builder.EnsureInfrastructure(context.Background()); err != nil {
			log.Warn("github build: failed to ensure build infrastructure", "error", err)
		}
		river.AddWorker(workers, ghBuildWorker)
		log.Info("river: registered worker", "worker", "GitHubBuildWorker")
	}

	return rw, pw
}
