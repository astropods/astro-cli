package riverqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/riverqueue/river"
	"k8s.io/client-go/dynamic"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/insightscache"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
)

type kindEntry struct {
	argsSchema json.RawMessage
	trigger    func(ctx context.Context, q *Queue, argsJSON json.RawMessage) (int64, error)
}

var kindRegistry = map[string]kindEntry{}
var duplicateJobKinds = map[string]int{}

// JobKindInfo holds a registered kind and its zero-value args schema.
type JobKindInfo struct {
	Kind       string
	ArgsSchema json.RawMessage
}

// RegisteredJobKinds returns all registered kinds with their args schemas.
func RegisteredJobKinds() []JobKindInfo {
	out := make([]JobKindInfo, 0, len(kindRegistry))
	for kind, e := range kindRegistry {
		out = append(out, JobKindInfo{Kind: kind, ArgsSchema: e.argsSchema})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Kind < out[j].Kind
	})
	return out
}

// TriggerJob enqueues a job of the given kind with the provided args JSON.
func (q *Queue) TriggerJob(ctx context.Context, kind string, argsJSON json.RawMessage) (int64, error) {
	entry, ok := kindRegistry[kind]
	if !ok {
		return 0, fmt.Errorf("unknown job kind: %q", kind)
	}
	return entry.trigger(ctx, q, argsJSON)
}

// registerJobKind records a job args type for admin listing and manual trigger.
// It runs from init functions beside each args type so API-only processes have
// the catalog without constructing worker dependencies.
func registerJobKind[T river.JobArgs]() {
	var zero T
	kind := zero.Kind()
	if _, ok := kindRegistry[kind]; ok {
		duplicateJobKinds[kind]++
		return
	}
	schema, err := json.Marshal(zero)
	if err != nil {
		schema = []byte("{}")
	}
	kindRegistry[kind] = kindEntry{
		argsSchema: schema,
		trigger: func(ctx context.Context, q *Queue, argsJSON json.RawMessage) (int64, error) {
			var args T
			if err := json.Unmarshal(argsJSON, &args); err != nil {
				return 0, err
			}
			result, err := q.Insert(ctx, args, nil)
			if err != nil {
				return 0, err
			}
			return result.Job.ID, nil
		},
	}
}

// registeredJobKind reports whether worker args are visible to API-only
// processes for admin listing and manual trigger.
func registeredJobKind[T river.JobArgs]() (string, bool) {
	var zero T
	kind := zero.Kind()
	_, ok := kindRegistry[kind]
	return kind, ok
}

func logDuplicateJobKinds(log *logger.Logger) {
	if log == nil {
		return
	}
	for kind, count := range duplicateJobKinds {
		log.Error("river: duplicate job kind registration ignored", "kind", kind, "duplicates", count)
	}
}

// addWorkerWithCatalogCheck adds a worker and logs if its args type is missing
// from the API-visible trigger registry. Tests enforce this invariant; runtime
// should not stop job processing for admin catalog drift.
func addWorkerWithCatalogCheck[T river.JobArgs](log *logger.Logger, workers *river.Workers, worker river.Worker[T]) {
	if kind, ok := registeredJobKind[T](); !ok && log != nil {
		log.Error("river: worker args missing job kind registration", "kind", kind)
	}
	river.AddWorker(workers, worker)
}

// addWorkers registers all River workers.
// Returns the ReconcileWorker, AccountPurgeWorker, InsightsRefreshWorker,
// and MigrateDeploymentClusterWorker so the caller can set their queue references after client creation.
func addWorkers(workers *river.Workers, cfg Config) (*ReconcileWorker, *AccountPurgeWorker, *InsightsRefreshWorker, *MigrateDeploymentClusterWorker) {
	log := cfg.Logger
	logDuplicateJobKinds(log)

	billing := openmeter.NewBillingStateManager(cfg.OMClient, cfg.DB, log)

	addWorkerWithCatalogCheck(log, workers, &OpenmeterWorker{
		omClient: cfg.OMClient,
		db:       cfg.DB,
		log:      log,
		billing:  billing,
	})
	log.Info("river: registered worker", "worker", "OpenmeterWorker", "period", "5m")

	addWorkerWithCatalogCheck(log, workers, &WorkOSEventsWorker{
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

	var langfuseBaseURL string
	if cfg.ServerConfig != nil {
		langfuseBaseURL = cfg.ServerConfig.Deployment.LangfuseBaseURL
	}

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

	addWorkerWithCatalogCheck(log, workers, &DeployWorker{
		deployer:        dep,
		store:           store,
		datasetStore:    evaldatasetstore.NewStore(cfg.DB),
		langfuseStore:   cfg.LangfuseStore,
		langfuseBaseURL: langfuseBaseURL,
		log:             log,
		cache:           cfg.K8sCache,
		billing:         billing,
	})
	log.Info("river: registered worker", "worker", "DeployWorker")
	addWorkerWithCatalogCheck(log, workers, &UndeployWorker{
		deployer: dep,
		store:    store,
		ksStore:  knowledgestore.NewStore(cfg.DB),
		log:      log,
		cache:    cfg.K8sCache,
		billing:  billing,
	})
	log.Info("river: registered worker", "worker", "UndeployWorker")
	addWorkerWithCatalogCheck(log, workers, &WakeUpWorker{deployer: dep, store: store, log: log, cache: cfg.K8sCache, billing: billing})
	log.Info("river: registered worker", "worker", "WakeUpWorker")
	migrateWorker := &MigrateDeploymentClusterWorker{deployer: dep, store: store, log: log, cache: cfg.K8sCache}
	addWorkerWithCatalogCheck(log, workers, migrateWorker)
	log.Info("river: registered worker", "worker", "MigrateDeploymentClusterWorker")

	var dynClient dynamic.Interface
	if cfg.K8sRegistry != nil {
		dynClient, _ = dynamic.NewForConfig(cfg.K8sRegistry.Default().Config())
	}

	addWorkerWithCatalogCheck(log, workers, &MessageCountSyncWorker{
		promClient:   cfg.PromClient,
		accountStore: cfg.AccountStore,
		db:           cfg.DB,
		log:          log,
	})
	log.Info("river: registered worker", "worker", "MessageCountSyncWorker", "period", "5m")

	addWorkerWithCatalogCheck(log, workers, &ObsSummaryRefreshWorker{
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
	addWorkerWithCatalogCheck(log, workers, insightsDiscovery)
	log.Info("river: registered worker", "worker", "InsightsRefreshWorker", "period", insightscache.RefreshInterval.String())

	addWorkerWithCatalogCheck(log, workers, &InsightsRefreshAccountWorker{
		cache:    cfg.K8sCache,
		computer: cfg.InsightsSummaryComputer,
		log:      log,
	})
	log.Info("river: registered worker", "worker", "InsightsRefreshAccountWorker")

	addWorkerWithCatalogCheck(log, workers, &AvatarBackfillWorker{
		avatarStore: cfg.AvatarStore,
		db:          cfg.DB,
		log:         log,
	})
	log.Info("river: registered worker", "worker", "AvatarBackfillWorker", "period", "24h")

	addWorkerWithCatalogCheck(log, workers, &BlueprintAvatarBackfillWorker{
		avatarStore: cfg.AvatarStore,
		db:          cfg.DB,
		log:         log,
	})
	log.Info("river: registered worker", "worker", "BlueprintAvatarBackfillWorker", "period", "24h")

	addWorkerWithCatalogCheck(log, workers, &ProviderBackfillWorker{
		db:  cfg.DB,
		log: log,
	})
	log.Info("river: registered worker", "worker", "ProviderBackfillWorker", "period", "24h")

	var omDefaultPlan string
	if cfg.ServerConfig != nil {
		omDefaultPlan = cfg.ServerConfig.OpenMeterDefaultPlan
	}
	addWorkerWithCatalogCheck(log, workers, &OpenMeterBackfillWorker{
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
	addWorkerWithCatalogCheck(log, workers, pw)
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
	addWorkerWithCatalogCheck(log, workers, rw)
	log.Info("river: registered worker", "worker", "ReconcileWorker", "period", "10m")

	ksStoreForWorkers := knowledgestore.NewStore(cfg.DB)
	addWorkerWithCatalogCheck(log, workers, &KnowledgeReconcileWorker{
		ksStore:  ksStoreForWorkers,
		registry: cfg.K8sRegistry,
		log:      cfg.Logger,
		billing:  billing,
	})
	log.Info("river: registered worker", "worker", "KnowledgeReconcileWorker", "period", "30s")

	addWorkerWithCatalogCheck(log, workers, &PrivateLinkProvisionWorker{
		ksStore: ksStoreForWorkers,
		cfg:     cfg.ServerConfig,
		log:     cfg.Logger,
	})
	log.Info("river: registered worker", "worker", "PrivateLinkProvisionWorker")

	addWorkerWithCatalogCheck(log, workers, &PrivateLinkDeleteWorker{
		ksStore: ksStoreForWorkers,
		log:     cfg.Logger,
	})
	log.Info("river: registered worker", "worker", "PrivateLinkDeleteWorker")

	if cfg.PipesClient != nil && cfg.GitHubStore != nil && cfg.AgentIndex != nil {
		ghBuildWorker := NewGitHubBuildWorker(cfg.PipesClient, cfg.GitHubStore, cfg.AgentIndex, cfg.ReadmeAssetStore, cfg.K8sRegistry, cfg.ServerConfig, log, cfg.OMClient, cfg.DB, store, cfg.K8sCache)
		if err := ghBuildWorker.builder.EnsureInfrastructure(context.Background()); err != nil {
			log.Warn("github build: failed to ensure build infrastructure", "error", err)
		}
		addWorkerWithCatalogCheck(log, workers, ghBuildWorker)
		log.Info("river: registered worker", "worker", "GitHubBuildWorker")
	}

	return rw, pw, insightsDiscovery, migrateWorker
}
