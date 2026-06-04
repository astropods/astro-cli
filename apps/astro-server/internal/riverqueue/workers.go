package riverqueue

import (
	"context"

	"github.com/riverqueue/river"
	"k8s.io/client-go/dynamic"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/insightscache"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
)

// addWorkers registers all River workers into the registry.
// Returns the ReconcileWorker, AccountPurgeWorker and InsightsRefreshWorker
// so the caller can set their queue references after client creation.
func addWorkers(workers *river.Workers, cfg Config) (*ReconcileWorker, *AccountPurgeWorker, *InsightsRefreshWorker, *MigrateDeploymentClusterWorker) {
	log := cfg.Logger

	billing := openmeter.NewBillingStateManager(cfg.OMClient, cfg.DB, log)

	river.AddWorker(workers, &OpenmeterWorker{
		omClient: cfg.OMClient,
		db:       cfg.DB,
		log:      log,
		billing:  billing,
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
	if cfg.K8sRegistry != nil && cfg.ServerConfig != nil {
		dep = &deployer.Deployer{
			Registry:         cfg.K8sRegistry,
			AccountStore:     cfg.AccountStore,
			Cfg:              cfg.ServerConfig,
			Store:            store,
			Log:              cfg.Logger,
			KnowledgeStore:   knowledgestore.NewStore(cfg.DB),
			ImagePreflighter: cfg.ImagePreflighter,
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

		// Initialize AI Gateway per-account virtual key provisioning if configured.
		// Empty AI_GATEWAY_URL disables the feature entirely — the validator
		// then rejects provider:astro-gateway at admission (see deployment/validator.go).
		if cfg.ServerConfig.Deployment.AIGatewayURL != "" {
			dep.AIGatewayStore = aigateway.NewStore(cfg.DB)
			dep.AIGatewayProvisioner = aigateway.NewProvisioner(
				aigateway.NewClient(
					cfg.ServerConfig.Deployment.AIGatewayURL,
					cfg.ServerConfig.Deployment.AIGatewayMasterKey,
				),
			)
		}
	}

	river.AddWorker(workers, &DeployWorker{deployer: dep, store: store, log: log, cache: cfg.K8sCache, billing: billing})
	log.Info("river: registered worker", "worker", "DeployWorker")
	river.AddWorker(workers, &UndeployWorker{deployer: dep, store: store, ksStore: knowledgestore.NewStore(cfg.DB), log: log, cache: cfg.K8sCache, billing: billing})
	log.Info("river: registered worker", "worker", "UndeployWorker")
	river.AddWorker(workers, &WakeUpWorker{deployer: dep, store: store, log: log, cache: cfg.K8sCache, billing: billing})
	log.Info("river: registered worker", "worker", "WakeUpWorker")
	migrateWorker := &MigrateDeploymentClusterWorker{deployer: dep, store: store, log: log, cache: cfg.K8sCache}
	river.AddWorker(workers, migrateWorker)
	log.Info("river: registered worker", "worker", "MigrateDeploymentClusterWorker")

	var dynClient dynamic.Interface
	if cfg.K8sRegistry != nil {
		dynClient, _ = dynamic.NewForConfig(cfg.K8sRegistry.Default().Config())
	}

	river.AddWorker(workers, &MessageCountSyncWorker{
		promClient:   cfg.PromClient,
		accountStore: cfg.AccountStore,
		db:           cfg.DB,
		log:          log,
	})
	log.Info("river: registered worker", "worker", "MessageCountSyncWorker", "period", "5m")

	river.AddWorker(workers, &ObsSummaryRefreshWorker{
		cfg:             cfg.ServerConfig,
		db:              cfg.DB,
		cache:           cfg.K8sCache,
		deploymentStore: store,
		langfuseStore:   langfuse.NewStore(cfg.DB),
		log:             log,
	})
	log.Info("river: registered worker", "worker", "ObsSummaryRefreshWorker", "period", obssummary.RefreshInterval.String())

	// Discovery worker — enumerates accounts and enqueues per-account fan-out
	// jobs. Queue reference is wired post-construction in New() below, same
	// pattern as ReconcileWorker.
	insightsDiscovery := &InsightsRefreshWorker{
		langfuseStore: langfuse.NewStore(cfg.DB),
		log:           log,
	}
	river.AddWorker(workers, insightsDiscovery)
	log.Info("river: registered worker", "worker", "InsightsRefreshWorker", "period", insightscache.RefreshInterval.String())

	river.AddWorker(workers, &InsightsRefreshAccountWorker{
		cache:    cfg.K8sCache,
		computer: cfg.InsightsSummaryComputer,
		log:      log,
	})
	log.Info("river: registered worker", "worker", "InsightsRefreshAccountWorker")

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
		pw.aigwProvisioner = dep.AIGatewayProvisioner
		pw.aigwStore = dep.AIGatewayStore
	}
	if cfg.ServerConfig != nil && cfg.ServerConfig.Deployment.AIGatewayURL != "" {
		pw.aigwDevStore = aigateway.NewDevStore(cfg.DB)
	}
	// enqueueUndeploy is set after client creation in New() via SetPurgeQueue
	river.AddWorker(workers, pw)
	log.Info("river: registered worker", "worker", "AccountPurgeWorker", "period", "1h")

	rw := &ReconcileWorker{
		deployer:  dep,
		store:     store,
		registry:  cfg.K8sRegistry,
		dynClient: dynClient,
		log:       cfg.Logger,
		billing:   billing,
		cache:     cfg.K8sCache,
		// queue is set after client creation in New()
	}
	river.AddWorker(workers, rw)
	log.Info("river: registered worker", "worker", "ReconcileWorker", "period", "10m")

	ksStoreForWorkers := knowledgestore.NewStore(cfg.DB)
	river.AddWorker(workers, &KnowledgeReconcileWorker{
		ksStore:  ksStoreForWorkers,
		registry: cfg.K8sRegistry,
		log:      cfg.Logger,
		billing:  billing,
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
		ghBuildWorker := NewGitHubBuildWorker(cfg.PipesClient, cfg.GitHubStore, cfg.AgentIndex, cfg.K8sRegistry, cfg.ServerConfig, log, cfg.OMClient, cfg.DB, store, cfg.K8sCache)
		if err := ghBuildWorker.builder.EnsureInfrastructure(context.Background()); err != nil {
			log.Warn("github build: failed to ensure build infrastructure", "error", err)
		}
		river.AddWorker(workers, ghBuildWorker)
		log.Info("river: registered worker", "worker", "GitHubBuildWorker")
	}

	return rw, pw, insightsDiscovery, migrateWorker
}
