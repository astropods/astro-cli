package riverqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	billingpkg "github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/billing/metering"
	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaljudge"
	"github.com/astropods/astro/apps/astro-server/internal/insightscache"
	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/memberemails"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
	"github.com/astropods/astro/apps/astro-server/internal/observation"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
	"github.com/astropods/astro/apps/astro-server/internal/watcher"
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

// wiredWorkers are the workers whose *Queue reference must be set after the
// River client exists (the Queue wraps the client). New() sets .queue on each
// non-nil field once the client is built.
type wiredWorkers struct {
	purge           *AccountPurgeWorker
	insights        *InsightsRefreshWorker
	insightsRollup  *InsightsRollupWorker
	migrate         *MigrateDeploymentClusterWorker
	dunning         *DunningSweepWorker
	billingResume   *BillingResumeWorker
	metronomeHook   *MetronomeWebhookWorker
	stripeHook      *StripeWebhookWorker
	provisionSweep  *BillingProvisionSweepWorker
	provisionWorker *BillingProvisionWorker
	ghBuild         *GitHubBuildWorker
	observation     *ObservationSweepWorker
	undeploy        *UndeployWorker
	deploymentFGA   *DeploymentFGAReconcileWorker
}

// addWorkers registers all River workers and returns the ones needing a
// post-construction queue reference (see wiredWorkers).
func addWorkers(workers *river.Workers, cfg Config) wiredWorkers {
	log := cfg.Logger
	logDuplicateJobKinds(log)

	billing := metering.NewBillingStateManager(cfg.Billing, cfg.DB, log)

	addWorkerWithCatalogCheck(log, workers, &MeteringWorker{
		provider: cfg.Billing,
		db:       cfg.DB,
		log:      log,
		billing:  billing,
	})
	log.Info("river: registered worker", "worker", "MeteringWorker", "period", "5m")

	// Billing consumption-gating workers (hosted/metronome only): the webhook
	// workers apply Metronome/Stripe collection signals to the cached status, the
	// dunning sweep ages past_due→suspended (pure timer), and the suspend/resume
	// workers scale an account's deployments to zero and restore them.
	var dunningWorker *DunningSweepWorker
	var billingResumeWorker *BillingResumeWorker
	var metronomeHook *MetronomeWebhookWorker
	var stripeHook *StripeWebhookWorker
	var provisionSweep *BillingProvisionSweepWorker
	var provisionWorker *BillingProvisionWorker
	if cfg.BillingBackend == "metronome" {
		graceDays := 7
		if cfg.ServerConfig != nil {
			graceDays = cfg.ServerConfig.BillingDunningGraceDays
		}
		statusStore := billingpkg.NewStatusStore(cfg.DB, graceDays)
		billingDepStore := deploymentstore.NewStore(cfg.DB)
		dunningWorker = &DunningSweepWorker{
			status: statusStore,
			log:    log,
		}
		addWorkerWithCatalogCheck(log, workers, dunningWorker)
		log.Info("river: registered worker", "worker", "DunningSweepWorker", "period", "1h")

		addWorkerWithCatalogCheck(log, workers, &BillingSuspendWorker{
			store:  billingDepStore,
			status: statusStore,
			reg:    cfg.K8sRegistry,
			cache:  cfg.K8sCache,
			log:    log,
		})
		billingResumeWorker = &BillingResumeWorker{store: billingDepStore, log: log}
		addWorkerWithCatalogCheck(log, workers, billingResumeWorker)
		log.Info("river: registered worker", "worker", "BillingSuspend/ResumeWorker")

		metronomeHook = &MetronomeWebhookWorker{accounts: cfg.AccountStore, status: statusStore, cards: paymentCards(cfg.PaymentProvider), log: log}
		addWorkerWithCatalogCheck(log, workers, metronomeHook)
		stripeHook = &StripeWebhookWorker{accounts: cfg.AccountStore, status: statusStore, cards: paymentCards(cfg.PaymentProvider), log: log}
		addWorkerWithCatalogCheck(log, workers, stripeHook)
		log.Info("river: registered worker", "worker", "Metronome/StripeWebhookWorker")

		provisionWorker = &BillingProvisionWorker{
			accounts: cfg.AccountStore,
			provider: cfg.Billing,
			backend:  cfg.BillingBackend,
			status:   statusStore,
			log:      log,
		}
		addWorkerWithCatalogCheck(log, workers, provisionWorker)
		provisionSweep = &BillingProvisionSweepWorker{accounts: cfg.AccountStore, log: log}
		addWorkerWithCatalogCheck(log, workers, provisionSweep)
		log.Info("river: registered worker", "worker", "BillingProvision/SweepWorker")
	}

	memberEmailStore := memberemails.NewStore(cfg.DB)

	addWorkerWithCatalogCheck(log, workers, &MemberEmailReconcileWorker{
		workosClient: cfg.WorkOSClient,
		emails:       memberEmailStore,
		log:          log,
	})
	log.Info("river: registered worker", "worker", "MemberEmailReconcileWorker", "period", "24h")

	// Notification delivery. Falls back to the no-op provider when Novu is
	// unconfigured so the seam and worker still run (OSS/local).
	notifyProvider := cfg.NotifyProvider
	if notifyProvider == nil {
		notifyProvider = notify.NewNoopProvider(log)
	}
	// Preferences are owned and enforced by Novu; the Deliverer only resolves
	// recipients and triggers. appBaseURL absolutizes relative CTA links for email.
	var appBaseURL string
	if cfg.ServerConfig != nil {
		appBaseURL = cfg.ServerConfig.Auth.FrontendURL
	}
	// Manager audiences (billing/security/transfer) resolve owner+admin via WorkOS.
	// Pass a literal nil interface when unavailable so the Deliverer falls back to
	// the owner rather than dereferencing a typed-nil resolver.
	var notifyDeliverer *notify.Deliverer
	if cfg.OrgClient != nil && cfg.AccountStore != nil {
		mgr := &managerResolver{accounts: cfg.AccountStore, org: cfg.OrgClient}
		notifyDeliverer = notify.NewDeliverer(notifyProvider, memberEmailStore, cfg.AccountStore, mgr, appBaseURL, log)
	} else {
		notifyDeliverer = notify.NewDeliverer(notifyProvider, memberEmailStore, cfg.AccountStore, nil, appBaseURL, log)
	}
	// Deployment alerts resolve to that deployment's watchers, falling back to
	// managers above when nobody watches it.
	if cfg.DB != nil {
		notifyDeliverer = notifyDeliverer.WithWatchers(watcher.NewStore(cfg.DB))
	}
	addWorkerWithCatalogCheck(log, workers, &NotifyWorker{
		deliverer: notifyDeliverer,
		log:       log,
	})
	log.Info("river: registered worker", "worker", "NotifyWorker")

	store := deploymentstore.NewStore(cfg.DB)
	deploymentFGAWorker := &DeploymentFGAReconcileWorker{
		fga:           cfg.FGA,
		sync:          cfg.DeploymentFGASync,
		organizations: cfg.OrgClient,
		log:           log,
	}
	addWorkerWithCatalogCheck(log, workers, deploymentFGAWorker)
	log.Info("river: registered worker", "worker", "DeploymentFGAReconcileWorker", "period", "1m")

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
			KnowledgeStore:   knowledgestore.NewStore(cfg.DB, cfg.K8sCache),
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
					cfg.ServerConfig.Deployment.AIGatewayAdminURL,
					cfg.ServerConfig.Deployment.AIGatewayAdminAuth,
				),
				cfg.AccountStore,
				billingpkg.NewAliasSyncer(cfg.Billing, cfg.AccountStore, cfg.BillingBackend, cfg.Logger),
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
		reconcile:       cfg.ReconcileDeployment,
	})
	log.Info("river: registered worker", "worker", "DeployWorker")

	evalJudgeWorker := &EvalJudgePredictionWorker{
		datasets:    evaldatasetstore.NewStore(cfg.DB),
		predictions: judgmentstore.NewStore(cfg.DB),
		log:         log,
	}
	if cfg.ServerConfig != nil {
		langfuseStore := langfuse.NewStore(cfg.DB)
		langfuseBaseURL := cfg.ServerConfig.Deployment.LangfuseBaseURL
		evalJudgeWorker.loadLangfuse = func(ctx context.Context, accountID string) (*langfuse.AccountLangfuse, error) {
			return langfuseStore.GetDecrypted(ctx, cfg.KMSClient, accountID)
		}
		if langfuseBaseURL != "" {
			evalJudgeWorker.newTraceClient = func(credentials *langfuse.AccountLangfuse) evalJudgeTraceClient {
				return langfuse.NewClient(langfuseBaseURL, credentials.PublicKey, credentials.SecretKey)
			}
		}

		gatewayConfig := cfg.ServerConfig.Deployment
		if gatewayConfig.AIGatewayURL != "" && cfg.AccountStore != nil {
			provisioner := aigateway.NewProvisioner(
				aigateway.NewClient(
					gatewayConfig.AIGatewayURL,
					gatewayConfig.AIGatewayAdminURL,
					gatewayConfig.AIGatewayAdminAuth,
				),
				cfg.AccountStore,
				billingpkg.NewAliasSyncer(cfg.Billing, cfg.AccountStore, cfg.BillingBackend, cfg.Logger),
			)
			judgeStore := aigateway.NewJudgeStore(cfg.DB)
			evalJudgeWorker.ensureJudgeKey = func(ctx context.Context, accountID string) (string, string, error) {
				return provisioner.EnsureJudgeKey(
					ctx,
					judgeStore,
					gatewayConfig.KMSKeyARN,
					cfg.KMSClient,
					accountID,
				)
			}
			evalJudgeWorker.newPredictor = func(baseURL string) evalJudgePredictor {
				return evaljudge.New(aigateway.NewInvocationClient(baseURL))
			}
		}
	}
	addWorkerWithCatalogCheck(log, workers, evalJudgeWorker)
	log.Info("river: registered worker", "worker", "EvalJudgePredictionWorker")

	undeployWorker := &UndeployWorker{
		deployer: dep,
		store:    store,
		ksStore:  knowledgestore.NewStore(cfg.DB, cfg.K8sCache),
		log:      log,
		cache:    cfg.K8sCache,
		billing:  billing,
		fgaSync:  cfg.DeploymentFGASync,
	}
	addWorkerWithCatalogCheck(log, workers, undeployWorker)
	log.Info("river: registered worker", "worker", "UndeployWorker")
	addWorkerWithCatalogCheck(log, workers, &WakeUpWorker{deployer: dep, store: store, log: log, cache: cfg.K8sCache})
	log.Info("river: registered worker", "worker", "WakeUpWorker")
	migrateWorker := &MigrateDeploymentClusterWorker{deployer: dep, store: store, log: log, cache: cfg.K8sCache}
	addWorkerWithCatalogCheck(log, workers, migrateWorker)
	log.Info("river: registered worker", "worker", "MigrateDeploymentClusterWorker")

	addWorkerWithCatalogCheck(log, workers, &DeploymentWatchdogWorker{
		store: store,
		cache: cfg.K8sCache,
		log:   log,
	})
	log.Info("river: registered worker", "worker", "DeploymentWatchdogWorker", "period", "5m")

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
	// jobs. Queue reference is wired post-construction in New() below.
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

	// Rollup pipeline — builds the durable daily fact table alongside the v1
	// cache. Runs whether or not the v2 read path is enabled, so the flag can be
	// flipped against already-populated history rather than a cold table.
	insightsRollupDiscovery := &InsightsRollupWorker{
		langfuseStore: langfuse.NewStore(cfg.DB),
		log:           log,
	}
	addWorkerWithCatalogCheck(log, workers, insightsRollupDiscovery)
	log.Info("river: registered worker", "worker", "InsightsRollupWorker", "period", insightsrollup.RollupInterval.String())

	addWorkerWithCatalogCheck(log, workers, &InsightsRollupAccountWorker{
		producer: cfg.InsightsRollupProducer,
		rollups:  insightsrollup.NewStore(cfg.DB),
		log:      log,
	})
	log.Info("river: registered worker", "worker", "InsightsRollupAccountWorker")

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

	// Account purge worker — needs langfuse provisioner/store from deployer (if available)
	pw := &AccountPurgeWorker{
		db:            cfg.DB,
		deployStore:   store,
		fgaSync:       cfg.DeploymentFGASync,
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
		pw.aigwJudgeStore = aigateway.NewJudgeStore(cfg.DB)
	}
	// enqueueUndeploy is set after client creation in New() via SetPurgeQueue
	addWorkerWithCatalogCheck(log, workers, pw)
	log.Info("river: registered worker", "worker", "AccountPurgeWorker", "period", "1h")

	ksStoreForWorkers := knowledgestore.NewStore(cfg.DB, cfg.K8sCache)
	addWorkerWithCatalogCheck(log, workers, &KnowledgeReconcileWorker{
		ksStore:   ksStoreForWorkers,
		log:       cfg.Logger,
		localMode: cfg.ServerConfig != nil && cfg.ServerConfig.Deployment.IsLocal(),
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

	var ghBuildWorker *GitHubBuildWorker
	if cfg.PipesClient != nil && cfg.GitHubStore != nil && cfg.AgentIndex != nil {
		ghBuildWorker = NewGitHubBuildWorker(cfg.PipesClient, cfg.GitHubStore, cfg.AgentIndex, cfg.ReadmeAssetStore, cfg.K8sRegistry, cfg.ServerConfig, log, cfg.DB, store, cfg.K8sCache, cfg.AccountStore)
		if err := ghBuildWorker.builder.EnsureInfrastructure(context.Background()); err != nil {
			log.Warn("github build: failed to ensure build infrastructure", "error", err)
		}
		addWorkerWithCatalogCheck(log, workers, ghBuildWorker)
		log.Info("river: registered worker", "worker", "GitHubBuildWorker")
	}

	// Observation alert evaluator (metric-driven). Needs the metrics store; the
	// queue reference for emitting is wired post-construction (wiredWorkers).
	var observationSweep *ObservationSweepWorker
	if cfg.PromClient != nil {
		observationSweep = &ObservationSweepWorker{
			prom:     cfg.PromClient,
			deploys:  store,
			state:    observation.NewStore(cfg.DB),
			accounts: cfg.AccountStore,
			log:      log,
		}
		addWorkerWithCatalogCheck(log, workers, observationSweep)
		log.Info("river: registered worker", "worker", "ObservationSweepWorker")
	}

	return wiredWorkers{
		purge:           pw,
		insights:        insightsDiscovery,
		migrate:         migrateWorker,
		dunning:         dunningWorker,
		billingResume:   billingResumeWorker,
		metronomeHook:   metronomeHook,
		stripeHook:      stripeHook,
		provisionSweep:  provisionSweep,
		provisionWorker: provisionWorker,
		ghBuild:         ghBuildWorker,
		observation:     observationSweep,
		insightsRollup:  insightsRollupDiscovery,
		undeploy:        undeployWorker,
		deploymentFGA:   deploymentFGAWorker,
	}
}
