package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	goredis "github.com/redis/go-redis/v9"

	"github.com/astropods/astro/apps/astro-server/handlers"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/accountlifecycle"
	"github.com/astropods/astro/apps/astro-server/internal/accountvars"
	"github.com/astropods/astro/apps/astro-server/internal/admingrpc"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/appstore"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/authorizationadmin"
	"github.com/astropods/astro/apps/astro-server/internal/authorizationstore"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/billing/fake"
	"github.com/astropods/astro/apps/astro-server/internal/billing/metering"
	"github.com/astropods/astro/apps/astro-server/internal/billing/metronome"
	"github.com/astropods/astro/apps/astro-server/internal/billing/noop"
	"github.com/astropods/astro/apps/astro-server/internal/classification"
	"github.com/astropods/astro/apps/astro-server/internal/clusterconfig"
	"github.com/astropods/astro/apps/astro-server/internal/clusterid"
	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/connectapps"
	"github.com/astropods/astro/apps/astro-server/internal/dbhealth"
	"github.com/astropods/astro/apps/astro-server/internal/deploycontroller"
	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deployeval"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldismissalstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalitemstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalrunstore"
	"github.com/astropods/astro/apps/astro-server/internal/eventstream"
	"github.com/astropods/astro/apps/astro-server/internal/experiment"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/githubwebhook"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/imagecache"
	"github.com/astropods/astro/apps/astro-server/internal/ingesttoken"
	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/leaderelection"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/memberemails"
	"github.com/astropods/astro/apps/astro-server/internal/metricsstore"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
	"github.com/astropods/astro/apps/astro-server/internal/novu"
	"github.com/astropods/astro/apps/astro-server/internal/observation"
	oapispec "github.com/astropods/astro/apps/astro-server/internal/openapi"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/astropods/astro/apps/astro-server/internal/payment"
	"github.com/astropods/astro/apps/astro-server/internal/pgnotify"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
	"github.com/astropods/astro/apps/astro-server/internal/readmeassets"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
	"github.com/astropods/astro/apps/astro-server/internal/watcher"
	"github.com/astropods/astro/apps/astro-server/internal/workclassifier"
)

func main() {
	// Load .env file if it exists (ignore error if not found)
	_ = godotenv.Load()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(cfg.Log.Level, cfg.Log.Format)
	log.Info("server: starting",
		"version", "1.0.0",
		"mode", cfg.Server.Mode,
		"run_mode", cfg.RunMode,
		"port", cfg.Server.Port,
	)

	// Open shared database connection
	db, err := sql.Open("postgres", cfg.Database.URL)
	if err != nil {
		log.Error("database: open failed", "error", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	if err := db.Ping(); err != nil {
		log.Error("database: connect failed", "error", err)
		os.Exit(1)
	}
	log.Info("database: connected")

	// Initialize account store and agent index (needed by both API and worker)
	accountStore := account.NewAccountStoreWithClusters(db, clusterid.New(cfg.Deployment.DefaultClusterID))
	agentIndex := agentindex.NewIndexWithDB(db)
	var deploymentFGA authz.FGA
	var workosFGA *authz.WorkOSFGA
	if cfg.Auth.WorkOSAPIKey != "" {
		workosFGA = authz.NewWorkOSFGA(cfg.Auth.WorkOSAPIKey)
		deploymentFGA = workosFGA
	}
	accessAssignments, _ := deploymentFGA.(authz.AccessAssignments)
	var resourceAccessSync *authz.ResourceAccessSyncStore
	var accessReconciler *authz.AccessReconciler
	if accessAssignments != nil {
		resourceAccessSync = authz.NewResourceAccessSyncStore(db)
		accessReconciler = authz.NewAccessReconciler(accessAssignments, resourceAccessSync)
	}
	authorizationAdminStore := authorizationadmin.NewStore(db)
	var authorizationAdminService *authorizationadmin.Service
	if workosFGA != nil {
		authorizationAdminService = authorizationadmin.NewService(db, workosFGA, authorizationAdminStore)
	}

	// Build a shared S3 client (respects S3_ENDPOINT for local MinIO / S3-compatible stores).
	sharedS3Client, sharedS3Err := newS3Client(cfg)
	if sharedS3Err != nil {
		log.Warn("s3: client init failed", "error", sharedS3Err)
	}

	// Initialize avatar + readme-asset stores (S3 or local filesystem). Both
	// serve from the assets bucket/CDN and share the same storage backend.
	var avatarStore *avatar.Store
	var readmeAssetStore *readmeassets.Store
	if cfg.Avatar.Enabled() {
		if cfg.Avatar.IsLocal() {
			backend := avatar.NewLocalBackend(cfg.Avatar.LocalDir)
			avatarStore = avatar.NewStore(backend, cfg.Avatar.AssetsURL)
			readmeAssetStore = readmeassets.NewStore(backend, cfg.Avatar.AssetsURL)
			log.Info("avatars: store initialized", "backend", "local", "dir", cfg.Avatar.LocalDir)
		} else if sharedS3Client != nil {
			backend := avatar.NewS3Backend(sharedS3Client, cfg.Avatar.S3Bucket)
			avatarStore = avatar.NewStore(backend, cfg.Avatar.AssetsURL)
			readmeAssetStore = readmeassets.NewStore(backend, cfg.Avatar.AssetsURL)
			log.Info("avatars: store initialized", "backend", "s3", "bucket", cfg.Avatar.S3Bucket)
		}
	}
	// Initialize WorkOS organization client
	var orgClient *org.Client
	var orgSync *org.Sync
	if cfg.Auth.WorkOSAPIKey != "" {
		orgClient = org.NewClient(cfg.Auth.WorkOSAPIKey)
		workosClient := auth.NewWorkOSClient(cfg.Auth.WorkOSAPIKey, cfg.Auth.WorkOSClientID, cfg.Auth.RedirectURI, cfg.Auth.FrontendURL)
		orgSync = org.NewSync(orgClient, accountStore, workosClient, db, log)
		log.Info("workos: organization client initialized")
	}

	// Billing provider seam. BILLING_PROVIDER selects the backend; the
	// metering/customer paths depend only on billing.BillingProvider.
	var billingProvider billing.BillingProvider
	switch cfg.BillingBackend() {
	case config.BillingBackendNoop:
		billingProvider = noop.New()
		log.Info("billing: provider selected", "provider", "noop", "metered", false)
	case config.BillingBackendFake:
		billingProvider = fake.New()
		log.Warn("billing: provider selected, spend and invoices are canned", "provider", "fake")
	case config.BillingBackendMetronome:
		mp := metronome.New(metronome.Config{
			APIKey:            cfg.MetronomeAPIKey,
			PackageID:         cfg.MetronomePackageID,
			PackageIDNoCredit: cfg.MetronomePackageIDNoCredit,
		})
		if mp == nil {
			log.Error("billing: METRONOME_API_KEY unset, billing disabled")
		} else {
			billingProvider = mp
		}
		log.Info("billing: provider selected", "provider", "metronome")
	default:
		log.Error("billing: unsupported BILLING_PROVIDER, billing disabled", "provider", cfg.BillingBackend())
	}

	// Payment provider seam (card vault). Stripe collects and saves a card; the
	// billing provider (Metronome) charges it. Nil when STRIPE_SECRET_KEY is
	// unset — handlers report "payments not available".
	var paymentProvider payment.Provider
	if sp := payment.NewStripe(payment.StripeConfig{
		SecretKey:      cfg.StripeSecretKey,
		PublishableKey: cfg.StripePublishableKey,
	}); sp != nil {
		paymentProvider = sp
		log.Info("payment: provider selected", "provider", "stripe")
	}

	// Consumption gate. Reads the cached account_billing_status (written off-path
	// by the Metronome webhook + dunning sweep); nil store for non-metronome
	// backends → pass-through. BILLING_GATE_ENFORCE=false is observe mode.
	var billingStatus *billing.StatusStore
	switch cfg.BillingBackend() {
	case config.BillingBackendMetronome, config.BillingBackendFake:
		billingStatus = billing.NewStatusStore(db, cfg.BillingDunningGraceDays).
			WithExemptAccounts(cfg.BillingExemptAccounts)
	}
	ent := middleware.NewEntitlements(billingStatus, cfg.BillingGateEnforce, log)

	// Per-account resource quota (DB-backed, OSS + hosted). Over-limit blocking
	// respects QUOTA_ENFORCE; a disabled feature (limit 0) always blocks.
	quotaChecker := quota.NewDBChecker(db, log, cfg.QuotaDefaults, cfg.QuotaEnforce)

	// Initialize shared Redis client (nil when REDIS_URL is unset).
	// Pass this client to any feature that needs Redis; do not create additional clients.
	var redisClient *goredis.Client
	if cfg.RedisURL != "" {
		opts, redisErr := goredis.ParseURL(cfg.RedisURL)
		if redisErr != nil {
			log.Error("redis: REDIS_URL parse failed, redis features disabled", "error", redisErr)
		} else {
			redisClient = goredis.NewClient(opts)
			// Log host/db only — the parsed URL carries the password in userinfo.
			log.Info("redis: client initialized", "addr", opts.Addr, "db", opts.DB)
		}
	}
	k8sCache := k8scache.New(redisClient)
	agentIndex.WithCache(k8sCache)

	// Track components for graceful shutdown
	var httpSrv *http.Server
	var grpcServer *grpc.Server
	var eventsCancel context.CancelFunc
	var probeHandler *handlers.ProbeHandler
	var adminSrv *admingrpc.Server
	var apiQueue *riverqueue.Queue

	vault, vaultErr := envelope.Open(context.Background(), cfg.Deployment.IsLocal(), cfg.Deployment.KMSKeyARN)
	if vaultErr != nil {
		log.Error("vault: open failed", "error", vaultErr)
		os.Exit(1)
	}

	// --- API mode: HTTP server + gRPC admin ---
	if cfg.RunAPI() {
		httpSrv, grpcServer, probeHandler, adminSrv, apiQueue = runAPI(log, cfg, db, accountStore, agentIndex, orgClient, orgSync, billingProvider, paymentProvider, ent, billingStatus, quotaChecker, avatarStore, readmeAssetStore, k8sCache, resourceAccessSync, deploymentFGA, authorizationAdminService, authorizationAdminStore, vault)
	}

	// --- Worker mode: events consumer ---
	if cfg.RunWorker() {
		eventsCancel = runWorker(log, cfg, accountStore, agentIndex, db, billingProvider, paymentProvider, orgClient, avatarStore, readmeAssetStore, k8sCache, newImagePreflighter(cfg), resourceAccessSync, accessReconciler, deploymentFGA, authorizationAdminService, vault)
	}

	// In worker-only mode, start a minimal health server
	if !cfg.RunAPI() {
		mux := http.NewServeMux()
		mux.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "ok")
		})
		mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "ok")
		})
		httpSrv = &http.Server{
			Addr:              fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port),
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			log.Info("worker: health server listening", "address", httpSrv.Addr)
			if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("worker: health server failed", "error", err)
				os.Exit(1)
			}
		}()
	}

	dbHealthCtx, dbHealthCancel := context.WithCancel(context.Background())
	go dbhealth.New(db, log, dbhealth.DefaultInterval).Run(dbHealthCtx)

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	dbHealthCancel()

	log.Info("server: shutting down")

	// Mark as not ready to stop receiving new traffic
	if probeHandler != nil {
		probeHandler.SetReady(false)
	}

	// Stop events consumer
	if eventsCancel != nil {
		eventsCancel()
	}

	// Graceful shutdown of gRPC servers
	if grpcServer != nil {
		grpcServer.GracefulStop()
	}
	if adminSrv != nil {
		adminSrv.ShutdownRiverUI()
	}

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	// Close insert-only River queue pool
	if apiQueue != nil {
		apiQueue.Close()
	}

	// Attempt graceful shutdown of HTTP server
	if httpSrv != nil {
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Error("server: forced shutdown", "error", err)
			os.Exit(1)
		}
	}

	log.Info("server: stopped gracefully")
}

// exitOnLocalClusterMisconfig stops a local server whose cluster config cannot
// be read. That entry is the only source of the cluster's ingress domains and
// observability URLs, so booting without it produces deploys that resolve to
// nothing. Managed environments keep the warn-and-degrade path instead, where
// an operator is watching and the API is worth serving without Kubernetes.
func exitOnLocalClusterMisconfig(cfg *config.Config, log *logger.Logger, err error) {
	if err == nil || k8s.ClientMode(cfg.Deployment.K8sClientMode) != k8s.ClientModeLocal {
		return
	}
	log.Error("cluster config: unusable",
		"error", err,
		"path", cfg.Deployment.ClusterConfigPath,
		"default_cluster_id", cfg.Deployment.DefaultClusterID,
	)
	os.Exit(1)
}

func buildRegistryConfig(ctx context.Context, clusterStore *clusterstore.Store, cfg *config.Config, log *logger.Logger) (k8s.RegistryConfig, error) {
	regCfg := k8s.RegistryConfig{
		Mode:           k8s.ClientMode(cfg.Deployment.K8sClientMode),
		Region:         cfg.Deployment.AWSRegion,
		KubeconfigPath: cfg.Deployment.KubeconfigPath,
		KubeContext:    cfg.Deployment.KubeContext,
	}
	// A cluster's ingress domains and observability URLs live only in its
	// config entry, so an unset path leaves every one of them empty and the
	// failure surfaces later as an agent with no URL. Refuse at boot instead.
	// Local dev gets its entry generated by scripts/dev.sh.
	if cfg.Deployment.ClusterConfigPath == "" {
		return regCfg, fmt.Errorf("CLUSTER_CONFIG_PATH is required")
	}

	entries, err := clusterconfig.Load(cfg.Deployment.ClusterConfigPath)
	if err != nil {
		return regCfg, err
	}
	log.Info("cluster config: loaded", "path", cfg.Deployment.ClusterConfigPath, "entries", len(entries))

	// Resolve the default cluster before syncing — a DEFAULT_CLUSTER_ID that
	// doesn't match any entry (e.g. the default cluster was renamed in config
	// without updating the env var) must fail before Sync deletes/upserts any
	// rows, not after.
	defaultEntry, ok := clusterconfig.Find(entries, cfg.Deployment.DefaultClusterID)
	if !ok {
		return regCfg, fmt.Errorf("default cluster %q not found in cluster config", cfg.Deployment.DefaultClusterID)
	}
	clusterconfig.Sync(ctx, clusterStore, entries, cfg.Deployment.DefaultClusterID, log)

	regCfg.DefaultClusterID = defaultEntry.ID
	regCfg.PrometheusURL = defaultEntry.PrometheusURL

	// Local mode builds its ClusterClient from kubeconfig, so the entry's EKS
	// coordinates are inert there and its CA is empty. Every other field still
	// comes from the entry, which is the only source of a cluster's ingress
	// domains and observability URLs.
	if regCfg.Mode == k8s.ClientModeLocal {
		return regCfg, nil
	}

	ca, err := defaultEntry.DecodedCA()
	if err != nil {
		return regCfg, err
	}
	regCfg.Region = defaultEntry.Region
	regCfg.EKSBootstrapName = defaultEntry.EKSClusterName
	regCfg.EKSBootstrapURL = defaultEntry.EKSClusterEndpoint
	regCfg.EKSBootstrapCA = ca
	return regCfg, nil
}

// runAPI initializes and starts the HTTP API server and gRPC admin server.
func runAPI(
	log *logger.Logger,
	cfg *config.Config,
	db *sql.DB,
	accountStore *account.AccountStore,
	agentIndex *agentindex.Index,
	orgClient *org.Client,
	orgSync *org.Sync,
	billingProvider billing.BillingProvider,
	paymentProvider payment.Provider,
	ent *middleware.Entitlements,
	billingStatus *billing.StatusStore,
	quotaChecker *quota.DBChecker,
	avatarStore *avatar.Store,
	readmeAssetStore *readmeassets.Store,
	k8sCache k8scache.Cache,
	resourceAccessSync *authz.ResourceAccessSyncStore,
	deploymentFGA authz.FGA,
	authorizationAdminService *authorizationadmin.Service,
	authorizationAdminStore *authorizationadmin.Store,
	vault *envelope.Vault,
) (*http.Server, *grpc.Server, *handlers.ProbeHandler, *admingrpc.Server, *riverqueue.Queue) {
	resourceLifecycle, _ := deploymentFGA.(authz.ResourceLifecycle)

	// Set Gin mode
	gin.SetMode(cfg.Server.Mode)

	// Create router without default middleware
	router := gin.New()

	// Apply custom middleware
	router.Use(middleware.Recovery(log))
	router.Use(middleware.Logger(log))
	router.Use(middleware.CORS(cfg.Security.AllowedOrigins))
	router.Use(middleware.SecurityHeaders())

	// Set trusted proxies
	if len(cfg.Security.TrustedProxies) > 0 {
		if err := router.SetTrustedProxies(cfg.Security.TrustedProxies); err != nil {
			log.Error("server: set trusted proxies failed", "error", err)
		}
	}

	// Initialize stores. The deployment store is wired with agentIndex as
	// its LineageValidator so SaveDeploymentPending / UpdateDeploymentPending
	// reject any write whose (source_account_id, agent_name, build_id) tuple
	// does not match a published agent_versions row. Tests construct
	// deploymentstore.NewStore(db) without this wire and the gate becomes a
	// no-op, which preserves existing test fixtures.
	deploymentStore := deploymentstore.NewStore(db).WithLineageValidator(agentIndex)
	clusterStore := clusterstore.New(db)
	accountVarsStore := accountvars.NewStore(db)
	heartStore := heartstore.New(db)
	agentMetricsStore := metricsstore.New(db)
	ksStore := knowledgestore.NewStore(db, k8sCache)
	log.Info("stores: initialized")

	/*
	   One-shot, idempotent backfill of deployments.source_account_id for rows
	   created before the column existed. Runs in a goroutine so it can never
	   delay API startup or the readiness probe — the read path already
	   tolerates NULL via SourceAccountFromSpec fallback, so serving traffic
	   before the backfill finishes is safe. Already-populated rows are
	   skipped by the column-IS-NULL filter, so this is cheap on subsequent
	   restarts.
	*/
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		res, err := deploymentStore.BackfillSourceAccountIDs(ctx)
		if err != nil {
			log.Warn("backfill: source_account_id failed",
				"error", err,
				"scanned", res.Scanned,
				"from_spec", res.FromSpec,
				"from_self", res.FromSelf,
				"spec_misses", res.SpecMisses,
			)
			// Continue to the rebind pass anyway — it touches a disjoint
			// set of rows (non-NULL source_account_id) and a partial
			// failure of the NULL-fill pass should not block repair of
			// transferred-agent deployments.
		} else if res.Scanned > 0 {
			log.Info("backfill: source_account_id complete",
				"scanned", res.Scanned,
				"from_spec", res.FromSpec,
				"from_self", res.FromSelf,
				"spec_misses", res.SpecMisses,
			)
		}

		// Repair non-NULL but stale source_account_id values left behind
		// by pre-fix agentindex.Transfer calls. Idempotent — exits as a
		// no-op once every deployment's lineage tuple is consistent with
		// agent_versions.
		rebind, err := deploymentStore.RebindStaleSourceAccountIDs(ctx)
		if err != nil {
			log.Warn("backfill: source_account_id stale rebind failed", "error", err)
		} else if rebind.Rebound > 0 {
			log.Info("backfill: source_account_id stale rebind complete", "rebound", rebind.Rebound)
		}
	}()

	clientMode := k8s.ClientMode(cfg.Deployment.K8sClientMode)
	log.Info("k8s: initializing registry", "mode", string(clientMode))
	registryCtx, registryCancel := context.WithTimeout(context.Background(), 30*time.Second)
	registryCfg, regCfgErr := buildRegistryConfig(registryCtx, clusterStore, cfg, log)
	exitOnLocalClusterMisconfig(cfg, log, regCfgErr)
	var registry *k8s.Registry
	registryErr := regCfgErr
	if regCfgErr == nil {
		registry, registryErr = k8s.NewRegistry(registryCtx, clusterStore, registryCfg, log)
	}
	registryCancel()

	var k8sClient k8s.ClusterClient
	var k8sReg *k8s.Registry
	if registryErr != nil {
		log.Warn("k8s: registry init failed", "error", registryErr)
		log.Warn("k8s: features unavailable")
	} else {
		k8sReg = registry
		k8sClient = registry.Default()
		if version, connErr := k8sClient.GetServerVersion(); connErr != nil {
			log.Warn("k8s: client created but connection failed", "error", connErr)
			diag := k8sClient.DiagnoseConnection()
			for key, val := range diag {
				log.Debug("k8s: diagnostic", key, val)
			}
		} else {
			log.Info("k8s: connected",
				"mode", string(clientMode),
				"version", version,
			)
		}
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		backfillClusterPullCredentials(ctx, clusterStore, k8sReg, log)
	}()

	var lokiClient *loki.Client

	probeHandler := handlers.NewProbeHandler(log, agentIndex, k8sClient)

	// Create insert-only River queue for API (no workers, no periodic jobs — workers run in runWorker)
	rq, rqErr := riverqueue.NewInsertOnly(context.Background(), cfg.Database.URL, log, cfg.BillingGateEnforce)
	if rqErr != nil {
		log.Error("river: create api queue failed", "error", rqErr)
		os.Exit(1)
	}
	// Changes originate in the worker deployment, which holds no SSE connections.
	eventHub := eventstream.New()
	eventPublisher := eventstream.NewPublisher(db, deploymentStore, log)
	eventStore := eventstream.NewStore(db)
	go pgnotify.Listen(context.Background(), cfg.Database.URL, pgnotify.AgentEventChannel, log, func(payload string) {
		var n eventstream.Notification
		if err := json.Unmarshal([]byte(payload), &n); err != nil {
			log.Warn("events: decode notification failed", "error", err)
			return
		}
		eventstream.Fanout(eventHub, n)
	})

	// Roles Astro derives rather than a user choosing them: a resource
	// creator's admin role, and each member's account role. They ride the same
	// ledger as the access API, so they converge before enforcement turns on.
	var roleProjector *authz.RoleProjector
	if resourceAccessSync != nil {
		roleProjector = authz.NewRoleProjector(resourceAccessSync, accountStore, rq)
		if orgSync != nil {
			orgSync.SetAccountRoles(roleProjector)
		}
	}

	// Create audit log store. The watcher observer rides the audit trail so that
	// any handler auditing a deployment mutation enrolls its actor as a watcher
	// without a second call site to keep in sync.
	watcherStore := watcher.NewStore(db)
	auditStore := auditlog.NewStore(db).Observe(watcher.NewAuditObserver(watcherStore, log))
	experimentStore := experiment.NewStore(db)

	// Initialize GitHub connection store, webhook store, and WorkOS Pipes client.
	ghStore := githubconnection.New(db)
	webhookStore := githubwebhook.New(db)
	pipesClient := pipes.New(cfg.Auth.WorkOSAPIKey)
	slackIdentityStore := slackidentity.NewStore(db)
	appStore := appstore.NewStore(db)
	connectAppsClient := connectapps.New(cfg.Auth.WorkOSAPIKey)

	// Initialize Prometheus query client (nil if PROMETHEUS_URL is empty)
	promClient := promquery.NewClient(registryCfg.PrometheusURL, registryCfg.EKSBootstrapName)

	// Image preflighter: HEAD-checks tenant images before a deploy is enqueued
	// (handler) and again before K8s apply (worker via Deployer/Applier). One
	// instance is shared so the 60s positive-result cache absorbs duplicate
	// checks across both call sites.
	imagePreflighter := newImagePreflighter(cfg)

	// Register routes
	deps := &Deps{
		Log:   log,
		Cfg:   cfg,
		DB:    db,
		Vault: vault,
		Ent:   ent,
		Quota: quotaChecker,
		Probe: probeHandler,
		Stores: Stores{
			Account:            accountStore,
			App:                appStore,
			Deployment:         deploymentStore,
			AccountVars:        accountVarsStore,
			Heart:              heartStore,
			AgentMetrics:       agentMetricsStore,
			Cluster:            clusterStore,
			Audit:              auditStore,
			Avatar:             avatarStore,
			ReadmeAssets:       readmeAssetStore,
			Knowledge:          ksStore,
			GH:                 ghStore,
			Webhook:            webhookStore,
			SlackID:            slackIdentityStore,
			Watcher:            watcherStore,
			ResourceAccessSync: resourceAccessSync,
			Experiment:         experimentStore,
			BillingStatus:      billingStatus,
		},
		Clients: Clients{
			AgentIndex:    agentIndex,
			K8s:           k8sClient,
			Registry:      registry,
			Loki:          lokiClient,
			Org:           orgClient,
			OrgSync:       orgSync,
			Billing:       billingProvider,
			Payment:       paymentProvider,
			Pipes:         pipesClient,
			Prom:          promClient,
			K8sCache:      k8sCache,
			Preflight:     imagePreflighter,
			Queue:         rq,
			FGA:           deploymentFGA,
			Resources:     resourceLifecycle,
			RoleProjector: roleProjector,
			ConnectApps:   connectAppsClient,
			Events:        eventHub,
			EventStore:    eventStore,
			EventPub:      eventPublisher,
		},
	}
	setupRoutes(router, deps)

	// Start admin gRPC server
	adminSrv := admingrpc.New(log, deploymentStore, k8sClient, lokiClient, db, cfg.Database.URL, rq, auditStore, clusterStore, k8sReg, k8sCache)
	adminSrv.SetDeploymentAccessInspector(deploymentFGA, orgClient)
	adminSrv.SetAuthorizationAdmin(
		authorizationAdminService,
		authorizationAdminStore,
		cfg.AuthorizationAdminResetEnabled,
	)
	adminSrv.SetPrometheusClient(promClient)
	adminSrv.SetProxyRegistryHost(cfg.Deployment.ProxyRegistryHost)
	evalDeployer := &deployer.Deployer{
		Registry:     k8sReg,
		AccountStore: accountStore,
		Cfg:          cfg,
		Store:        deploymentStore,
		Log:          log,
		Vault:        vault,
	}
	adminSrv.SetEvaluators(deployeval.NewStore(db), deployeval.BuildAll(deployeval.Deps{Deployer: evalDeployer}))
	grpcServer, grpcErr := startAdminGRPCServer(log, cfg, adminSrv)
	if grpcErr != nil {
		log.Error("admin grpc: start failed", "error", grpcErr)
		os.Exit(1)
	}

	workosClient := auth.NewWorkOSClient(cfg.Auth.WorkOSAPIKey, cfg.Auth.WorkOSClientID, cfg.Auth.RedirectURI, cfg.Auth.FrontendURL)

	// Wire gin router as HTTP handler for admin ProxyHTTP
	adminSrv.SetHTTPHandler(router)

	// Wire WorkOS client for admin GetAuthConfig and owner email resolution
	adminSrv.SetWorkOSClientID(cfg.Auth.WorkOSClientID)
	adminSrv.SetWorkOSClient(workosClient)

	// Wire the quota reporter for the account detail view (usage + limits).
	adminSrv.SetQuotaReporter(quotaChecker)

	// Wire the billing provider for the Metronome ingest-alias health check.
	adminSrv.SetBillingProvider(billingProvider)

	// Wire the card vault, the gate flag, and the Metronome environment the
	// account billing view's deep links point at.
	adminSrv.SetBillingView(paymentProvider, cfg.BillingGateEnforce, cfg.MetronomeDashboardEnv)

	// Wire the observability provisioners for the account detail view's recover
	// actions (Langfuse project, Bifrost customer). Built in setupRoutes and
	// stashed on deps; nil when their backends are unconfigured.
	adminSrv.SetLangfuseProvisioner(deps.Clients.LangfuseProvisioner)
	adminSrv.SetAIGatewayProvisioner(deps.Clients.AIGateway)

	// Wire the account delete and purge sequences for the account detail view.
	// Both run in this process: the admin gRPC server lives on the API side,
	// whose queue is insert-only and registers no workers.
	adminSrv.SetAccountDeleter(deps.Clients.AccountDeleter)
	adminSrv.SetAccountPurger(deps.Clients.AccountPurger)

	// Wire ECR pull-through cache refresher for admin RefreshMessagingCache
	adminSrv.SetImageRefresher(imagecache.New(cfg.Deployment.AWSRegion))

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	// Start HTTP server
	go func() {
		log.Info("server: listening", "address", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server: start failed", "error", err)
			os.Exit(1)
		}
	}()

	return srv, grpcServer, probeHandler, adminSrv, rq
}

// backfillPrimaryBindings makes the account_clusters set exhaustive for every
// account, so readers that cannot materialize it, such as the registry's
// image-pull check, do not have to interpret an empty set.
func backfillPrimaryBindings(ctx context.Context, bindings *account.ClusterBindings, log *logger.Logger) {
	n, err := bindings.BackfillPrimaryBindings(ctx)
	if err != nil {
		log.Error("backfill: account cluster bindings failed, accounts without a binding are refused image pulls until the next leader election retries this",
			"error", err)
		return
	}
	if n > 0 {
		log.Info("backfill: account cluster bindings complete", "bound", n)
	}
}

func backfillPrimaryPlacement(ctx context.Context, store *deploymentstore.Store, clusters clusterid.Resolver, log *logger.Logger) {
	res, err := store.BackfillPrimaryClusterID(ctx, clusters.Primary())
	if err != nil {
		log.Error("backfill: cluster placement failed, deployments with no cluster recorded stay invisible to per-cluster queries until the next leader election retries this",
			"error", err)
		return
	}
	if res.Recorded > 0 {
		log.Info("backfill: cluster placement complete", "recorded", res.Recorded, "cluster_id", clusters.Primary())
	}
}

// backfillClusterPullCredentials is safe to run from every process/replica —
// EnsurePullCredential is a guarded, no-op-once-issued UPDATE. Refreshes the
// registry's cached entry for anything it backfills, since GetEntry caches
// lazily and never expires on its own — without this, a deploy that raced
// ahead of this goroutine and cached an empty credential would stay stuck
// with it until some unrelated admin action called Refresh.
func backfillClusterPullCredentials(ctx context.Context, clusterStore *clusterstore.Store, registry *k8s.Registry, log *logger.Logger) {
	clusters, err := clusterStore.List(ctx)
	if err != nil {
		log.Warn("backfill: cluster pull credential list clusters failed", "error", err)
		return
	}
	for _, c := range clusters {
		generated, err := clusterStore.EnsurePullCredential(ctx, c.ID)
		if err != nil {
			log.Warn("backfill: cluster pull credential failed", "cluster", c.ID, "error", err)
			continue
		}
		if generated {
			log.Info("backfill: cluster pull credential done", "cluster", c.ID)
			if registry != nil {
				_ = registry.Refresh(ctx, c.ID)
			}
		}
	}
}

// newImagePreflighter constructs the registry-HEAD preflighter used by both
// the deploy handler (synchronous 422 response) and the deploy worker
// (Applier defense-in-depth). Local mode treats 5xx responses as missing
// because the local astro-registry returns 500 for missing tags.
func newImagePreflighter(cfg *config.Config) *k8s.ImagePreflighter {
	localMode := cfg.Deployment.K8sClientMode == "local"
	return k8s.NewImagePreflighter(localMode)
}

// runWorker starts the River queue for all background job processing and returns a cancel func.
func runWorker(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	agentIndex *agentindex.Index,
	db *sql.DB,
	billingProvider billing.BillingProvider,
	paymentProvider payment.Provider,
	orgClient *org.Client,
	avatarStore *avatar.Store,
	readmeAssetStore *readmeassets.Store,
	k8sCache k8scache.Cache,
	imagePreflighter *k8s.ImagePreflighter,
	resourceAccessSync *authz.ResourceAccessSyncStore,
	accessReconciler *authz.AccessReconciler,
	deploymentFGA authz.FGA,
	authorizationAdminService *authorizationadmin.Service,
	vault *envelope.Vault,
) context.CancelFunc {
	workerCtx, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel is returned to caller

	initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
	clusterStore := clusterstore.New(db)
	registryCfg, regCfgErr := buildRegistryConfig(initCtx, clusterStore, cfg, log)
	exitOnLocalClusterMisconfig(cfg, log, regCfgErr)
	var registry *k8s.Registry
	registryErr := regCfgErr
	if regCfgErr == nil {
		registry, registryErr = k8s.NewRegistry(initCtx, clusterStore, registryCfg, log)
	}
	initCancel()

	var k8sReg *k8s.Registry
	if registryErr != nil {
		log.Warn("k8s: registry unavailable, namespace scanner will skip reconciliation", "error", registryErr)
	} else {
		k8sReg = registry
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		backfillClusterPullCredentials(ctx, clusterStore, k8sReg, log)
	}()

	// Initialize Prometheus query client (nil if PROMETHEUS_URL is empty)
	promClient := promquery.NewClient(registryCfg.PrometheusURL, registryCfg.EKSBootstrapName)
	if promClient != nil {
		log.Info("prometheus: client initialized", "url", registryCfg.PrometheusURL)
	}

	// Initialize WorkOS client for background jobs (user lookups)
	var workosClient *auth.WorkOSClient
	if cfg.Auth.WorkOSAPIKey != "" {
		workosClient = auth.NewWorkOSClient(cfg.Auth.WorkOSAPIKey, cfg.Auth.WorkOSClientID, cfg.Auth.RedirectURI, cfg.Auth.FrontendURL)
	}

	// Initialize WorkOS Pipes client and GitHub connection store.
	pipesClient := pipes.New(cfg.Auth.WorkOSAPIKey)
	ghStore := githubconnection.New(db)

	// Stores the roll-up producer needs. Cheap to construct here (Store wrappers
	// just hold the *sql.DB).
	workerDeploymentStore := deploymentstore.NewStore(db)
	workerLangfuseStore := langfuse.NewStore(db)
	workerSlackStore := slackidentity.NewStore(db)

	// Start the event-driven deployment controller before the River queue so the
	// DeployWorker can trigger an immediate reconcile when it marks a deployment
	// "deploying" (otherwise a no-change redeploy waits for the resync). It
	// watches managed K8s workloads via informers, persists observed health to
	// deployment_runtime_status, and drives deploying → active/failed — starting
	// compute billing on the real active transition. Wired to workerCtx.
	//
	// The controller is the sole writer of the deployment read-model, so it must
	// run on exactly one replica. Rather than depend on astro-worker staying
	// replicas: 1, it runs under a Postgres advisory-lock leader election: only
	// the replica holding the lock runs its informers and DB writes; the rest
	// idle as hot standbys and take over when the leader's connection drops. At
	// replicas: 1 the sole replica is always leader, so behavior is unchanged.
	var reconcileDeployment func(namespace string)
	if k8sReg != nil {
		billingState := metering.NewBillingStateManager(billingProvider, db, log)
		controller := deploycontroller.New(log, k8sReg, workerDeploymentStore, billingState)
		// The DeployWorker may run on any replica, but only the leader drains the
		// controller's queue. Route the immediate-reconcile nudge through Postgres
		// LISTEN/NOTIFY so it reaches the leader regardless of which replica
		// applied the deployment; a dropped nudge falls back to the resync.
		dsn := cfg.Database.URL
		reconcileDeployment = func(namespace string) {
			if err := pgnotify.Notify(workerCtx, db, pgnotify.DeployReconcileChannel, namespace); err != nil {
				log.Warn("deploycontroller: publish reconcile nudge failed", "namespace", namespace, "error", err)
			}
		}
		go leaderelection.Run(workerCtx, db, leaderelection.Config{
			LockKey: leaderelection.Key("deploy-controller"),
			Name:    "deploy-controller",
			Logger:  log,
		}, func(leaderCtx context.Context) {
			backfillPrimaryPlacement(leaderCtx, workerDeploymentStore, clusterid.New(cfg.Deployment.DefaultClusterID), log)
			backfillPrimaryBindings(leaderCtx, account.NewClusterBindings(db, clusterid.New(cfg.Deployment.DefaultClusterID)), log)
			// Feed leader-received nudges into the controller queue; the listener's
			// lifetime is tied to leadership via leaderCtx.
			go pgnotify.Listen(leaderCtx, dsn, pgnotify.DeployReconcileChannel, log, controller.EnqueueNamespace)
			controller.Run(leaderCtx)
		})
	}

	// Notification provider: Novu on the hosted path, no-op when unconfigured.
	var notifyProvider notify.Provider
	if cfg.Notify.NovuAPIURL != "" && cfg.Notify.NovuSecretKey != "" {
		notifyProvider = notify.NewNovuProvider(novu.NewClient(cfg.Notify.NovuAPIURL, cfg.Notify.NovuSecretKey))
		log.Info("notify: provider selected", "provider", "novu", "api_url", cfg.Notify.NovuAPIURL)
	} else {
		notifyProvider = notify.NewNoopProvider(log)
		log.Warn("notify: provider selected, notifications will be dropped", "provider", "noop")
	}

	rq, rqErr := riverqueue.New(workerCtx, cfg.Database.URL, riverqueue.Config{
		DB:                             db,
		NotifyProvider:                 notifyProvider,
		Billing:                        billingProvider,
		BillingBackend:                 cfg.BillingBackend(),
		PaymentProvider:                paymentProvider,
		AccountStore:                   accountStore,
		AgentIndex:                     agentIndex,
		AvatarStore:                    avatarStore,
		ReadmeAssetStore:               readmeAssetStore,
		K8sRegistry:                    k8sReg,
		K8sCache:                       k8sCache,
		ServerConfig:                   cfg,
		ResourceAccessSync:             resourceAccessSync,
		AccessReconciler:               accessReconciler,
		AuthorizationAdmin:             authorizationAdminService,
		AuthorizationAdminResetEnabled: cfg.AuthorizationAdminResetEnabled,
		FGA:                            deploymentFGA,
		WorkOSClient:                   workosClient,
		OrgClient:                      orgClient,
		PromClient:                     promClient,
		Logger:                         log,
		LangfuseStore:                  workerLangfuseStore,
		PipesClient:                    pipesClient,
		GitHubStore:                    ghStore,
		ImagePreflighter:               imagePreflighter,
		InsightsRollupProducer: &handlers.InsightsRollupProducer{
			Log:           log,
			Cfg:           cfg,
			AccountStore:  accountStore,
			LangfuseStore: workerLangfuseStore,
			SlackStore:    workerSlackStore,
			MemberEmails:  memberemails.NewStore(db),
			Rollups:       insightsrollup.NewStore(db),
		},
		ClassificationProducer: &handlers.ClassificationProducer{
			Log:           log,
			Cfg:           cfg,
			LangfuseStore: workerLangfuseStore,
			MemberEmails:  memberemails.NewStore(db),
			Classifier: workclassifier.NewClient(
				cfg.Deployment.FoundryInferenceURL,
				cfg.Deployment.WorkClassifierVersion,
			),
			Classifications: classification.NewStore(db),
		},
		ReconcileDeployment: reconcileDeployment,
		Vault:               vault,
	})
	if rqErr != nil {
		log.Error("river: create queue failed", "error", rqErr)
	} else {
		if startErr := rq.Start(workerCtx); startErr != nil {
			log.Error("river: start queue failed", "error", startErr)
		} else {
			// Stop River on context cancellation
			go func() {
				<-workerCtx.Done()
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = rq.Stop(stopCtx)
			}()

		}
	}

	return cancel
}

func setupRoutes(router *gin.Engine, deps *Deps) {
	// Local aliases keep the route registration body unchanged while the
	// outer signature stays small. If you're adding a new dependency, add
	// it to Deps (or one of its sub-structs) rather than re-introducing a
	// positional parameter here.
	log := deps.Log
	cfg := deps.Cfg
	db := deps.DB
	ent := deps.Ent
	quotaChecker := deps.Quota
	probeHandler := deps.Probe

	agentIndex := deps.Clients.AgentIndex
	k8sClient := deps.Clients.K8s
	k8sReg := deps.Clients.Registry
	lokiClient := deps.Clients.Loki
	orgClient := deps.Clients.Org
	orgSync := deps.Clients.OrgSync
	billingProvider := deps.Clients.Billing
	paymentProvider := deps.Clients.Payment
	pipesClient := deps.Clients.Pipes
	promClient := deps.Clients.Prom
	k8sCache := deps.Clients.K8sCache
	imagePreflighter := deps.Clients.Preflight
	queue := deps.Clients.Queue
	roleProjector := deps.Clients.RoleProjector
	deploymentFGA := deps.Clients.FGA
	resourceLifecycle := deps.Clients.Resources
	resourceAccessSync := deps.Stores.ResourceAccessSync
	experimentStore := deps.Stores.Experiment

	// Novu client for the browser Inbox config (HMAC subscriber hash) and the
	// per-user notification-preference proxy. Novu owns the catalog and
	// preferences. Nil when unconfigured; the handlers then report
	// disabled/empty rather than erroring.
	var notifyNovuClient *novu.Client
	if cfg.Notify.NovuAPIURL != "" && cfg.Notify.NovuSecretKey != "" {
		notifyNovuClient = novu.NewClient(cfg.Notify.NovuAPIURL, cfg.Notify.NovuSecretKey)
	}

	accountStore := deps.Stores.Account
	deploymentStore := deps.Stores.Deployment
	// Signup provisions the account's WorkOS organization inline; the worker's
	// hourly sweep covers whatever that attempt fails to link.
	orgProvisioner := org.NewProvisioner(orgClient, accountStore, log)
	if orgProvisioner != nil {
		orgProvisioner.SetAccountRoles(roleProjector)
	}
	resourceAccounts := authz.NewResourceAccountResolver(db)
	fgaExperiment := experiment.NewGate(experimentStore, experiment.FineGrainedAccess)
	classificationExperiment := experiment.NewGate(experimentStore, experiment.PromptClassificationStats)
	// Shared so the TTL holds across requests. Left nil without WorkOS: a typed
	// nil in the interface would defeat the lookup's own nil guard.
	var insightsOrgRoles *handlers.CachedOrgRoles
	if orgSync != nil {
		insightsOrgRoles = handlers.NewCachedOrgRoles(orgSync)
	}
	deploymentFGALiveRollout := authz.NewConditionalResourceGate(
		deploymentFGA != nil && (cfg.FGAShadowEnabled || cfg.FGAEnforcementEnabled),
		resourceAccounts,
	)
	deploymentFGAShadowRollout := authz.NewConditionalResourceGate(
		deploymentFGA != nil && cfg.FGAShadowEnabled,
		resourceAccounts,
	)
	deploymentFGAOrganizationRollout := authz.NewAccountExperimentResourceGate(
		deploymentFGALiveRollout,
		resourceAccounts,
		fgaExperiment,
	)
	deploymentAccessRollout := authz.NewAccountExperimentResourceGate(
		authz.NewConditionalResourceGate(
			deploymentFGA != nil && cfg.FGAEnforcementEnabled,
			resourceAccounts,
		),
		resourceAccounts,
		fgaExperiment,
	)
	deploymentResourceDiscovery, _ := deploymentFGA.(authz.ResourceDiscovery)
	deploymentDiscovery := authz.NewDeploymentDiscovery(
		log,
		deploymentFGA != nil && cfg.FGAEnforcementEnabled && deploymentResourceDiscovery != nil,
		deploymentResourceDiscovery,
		fgaExperiment,
		accountStore,
	)
	membershipChecker := authz.NewMembershipChecker(accountStore, resourceAccounts)
	deploymentFGALiveChecker := authz.NewFGAChecker(deploymentFGA, deploymentFGALiveRollout, resourceAccounts)
	deploymentFGAEnforcementChecker := authz.NewFGAChecker(deploymentFGA, deploymentFGAOrganizationRollout, resourceAccounts)
	deploymentCapabilities := authz.NewCapabilityService(
		log,
		deploymentFGAOrganizationRollout,
		membershipChecker,
		deploymentFGALiveChecker,
		authz.ActionDeploymentRead,
	)
	accessAssignments, _ := deploymentFGA.(authz.AccessAssignments)
	accessGroups, _ := deploymentFGA.(authz.Groups)
	deploymentAccess := authz.NewAccessService(
		accessAssignments,
		deploymentAccessRollout,
		resourceAccounts,
		resourceAccounts,
		accountStore,
		accessGroups,
		resourceAccessSync,
	)
	alertStore := observation.NewStore(db)
	accountVarsStore := deps.Stores.AccountVars
	heartStore := deps.Stores.Heart
	agentMetricsStore := deps.Stores.AgentMetrics
	clusterStore := deps.Stores.Cluster
	auditStore := deps.Stores.Audit
	accessGroupHandler := handlers.NewAccessGroupHandler(
		log,
		accessGroups,
		fgaExperiment,
		accountStore,
		auditStore,
		cfg.FGAEnforcementEnabled && accessGroups != nil,
	)
	appHandler := handlers.NewAppHandler(log, deps.Stores.App, deps.Clients.ConnectApps, auditStore)
	watcherStore := deps.Stores.Watcher
	avatarStore := deps.Stores.Avatar
	readmeAssetStore := deps.Stores.ReadmeAssets
	ksStore := deps.Stores.Knowledge
	ghStore := deps.Stores.GH
	webhookStore := deps.Stores.Webhook
	slackIdentityStore := deps.Stores.SlackID

	// AI Gateway wiring for handler-side use (dev-key issuance). Worker side
	// constructs its own provisioner via the deployer; both read the same URL +
	// master key from config. Nil when AI_GATEWAY_URL is unset — the dev-key
	// handler returns 503 in that case.
	var aiGatewayProvisioner *aigateway.Provisioner
	var aiGatewayDevStore *aigateway.DevStore
	var aiGatewayJudgeStore *aigateway.JudgeStore
	if cfg.Deployment.AIGatewayURL != "" {
		aiGatewayProvisioner = aigateway.NewProvisioner(
			aigateway.NewClient(cfg.Deployment.AIGatewayURL, cfg.Deployment.AIGatewayAdminURL, cfg.Deployment.AIGatewayAdminAuth),
			accountStore,
			billing.NewAliasSyncer(billingProvider, accountStore, cfg.BillingBackend(), log),
		)
		aiGatewayDevStore = aigateway.NewDevStore(db)
		aiGatewayJudgeStore = aigateway.NewJudgeStore(db)
	}
	deps.Clients.AIGateway = aiGatewayProvisioner

	// OTel ingest keys (account-scoped telemetry credential for local coding
	// tools). At key creation we best-effort ensure the account's Langfuse
	// project exists so the trace leg has a destination. The provisioner is nil
	// when its database isn't configured.
	ingestTokenStore := ingesttoken.NewStore(db)
	var ingestLangfuseProvisioner *langfuse.Provisioner
	var ingestLangfuseStore *langfuse.Store
	if cfg.Deployment.LangfuseDBURL != "" {
		if p, err := langfuse.NewProvisioner(cfg.Deployment.LangfuseDBURL, cfg.Deployment.LangfuseSalt, cfg.Deployment.LangfuseOrgID); err != nil {
			log.Warn("langfuse: provisioner init failed, ingest-key creation will skip project ensure", "error", err)
		} else {
			ingestLangfuseProvisioner = p
			ingestLangfuseStore = langfuse.NewStore(db)
		}
	}
	deps.Clients.LangfuseProvisioner = ingestLangfuseProvisioner

	// OpenAPI spec builder — routes registered via api.GET/POST/etc are
	// both added to gin AND documented in the generated spec.
	api := oapispec.New("Astro API", "1.0.0", "Platform for deploying and running AI agents. Provides agent-native infrastructure including models, knowledge bases, tool integrations, and observability.")
	deploymentRouteCatalog := middleware.NewDeploymentRouteCatalog()
	authzStore := authorizationstore.NewStore(db)

	// Serve OpenAPI spec
	router.GET("/openapi.json", api.JSON())

	// Kubernetes-style health probe endpoints (at root, not part of API spec)
	router.GET("/livez", probeHandler.Livez())
	router.GET("/readyz", probeHandler.Readyz())
	router.GET("/healthz", probeHandler.Healthz())

	// JSON Schema for astropods.yml (public, no auth)
	router.GET("/schema/package.json", handlers.AstroAISpecSchema())

	// Local avatar storage: serve the on-disk assets dir so {ASSETS_CDN_URL}/{key}
	// resolves without a real CDN. Only enabled when LocalDir is configured;
	// production uses S3 + CloudFront and never hits this route.
	if cfg.Avatar.IsLocal() {
		router.Static("/assets", cfg.Avatar.LocalDir)
	}

	handlers.RegisterCLIRoutes(router, cfg)

	// Setup authentication
	memberEmailStore := memberemails.NewStore(db)
	// Names pending invitees, who have no local profile to read.
	var inviteeLookup *auth.WorkOSClient
	if cfg.Auth.WorkOSAPIKey != "" {
		inviteeLookup = auth.NewWorkOSClient(cfg.Auth.WorkOSAPIKey, cfg.Auth.WorkOSClientID, cfg.Auth.RedirectURI, cfg.Auth.FrontendURL)
	}
	authHandler := handlers.NewAuthHandler(log, cfg, accountStore)
	if orgSync != nil {
		authHandler.SetOrgSync(orgSync)
	}
	if avatarStore != nil {
		authHandler.SetAvatarStore(avatarStore)
	}
	authHandler.SetMemberEmails(memberEmailStore)

	// Auth routes (no auth required)
	authRoutes := router.Group("/auth")
	{
		authRoutes.GET("/login", authHandler.Login())
		authRoutes.GET("/callback", authHandler.Callback())
		authRoutes.GET("/logout", authHandler.Logout())
		authRoutes.GET("/me", authHandler.Me())
		authRoutes.POST("/refresh", authHandler.Refresh())
		authRoutes.POST("/switch-org", authHandler.SwitchOrg())
	}

	// Create auth middleware
	authMw := middleware.NewAuthMiddleware(
		log,
		cfg,
		authHandler.GetSessionManager(),
		authHandler.GetJWTValidator(),
		deps.Stores.App,
	)
	log.Info("auth: enabled", "provider", "WorkOS")

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Deploy-token-authenticated routes — called by messaging containers
		deployTokenRoutes := v1.Group("")
		deployTokenRoutes.Use(middleware.RequireDeployToken(cfg.Security.DeployTokenSecret))
		{
			feedbackLangfuseStore := langfuse.NewStore(db)
			deployTokenRoutes.GET("/deployments/authorize", handlers.CheckDeploymentAuthorization(log, authzStore, slackIdentityStore))
			deployTokenRoutes.POST("/deployments/feedback/scores", handlers.PostDeploymentFeedbackScore(log, cfg, deploymentStore, feedbackLangfuseStore))
		}

		// Health check endpoint (public)
		api.GET(v1, "/health", "Health check", handlers.HealthCheck(log),
			oapispec.Tags("Health"),
			oapispec.Response(200, &handlers.HealthResponse{}),
		)

		// Readiness check endpoint (public)
		api.GET(v1, "/ready", "Readiness check", handlers.ReadinessCheck(log),
			oapispec.Tags("Health"),
			oapispec.Response(200, &handlers.HealthResponse{}),
		)

		// Agent registry endpoints (public read, with optional auth for visibility)
		api.GET(v1, "/agents", "List public agents", handlers.ListAgents(log, agentIndex, accountStore, heartStore, agentMetricsStore, deploymentStore, avatarStore, auditStore, authHandler.GetWorkOSClient()),
			oapispec.Tags("Agents"),
			oapispec.QueryParam("q", "Search by name or description", false),
			oapispec.QueryParam("tag", "Filter by exact tag", false),
			oapispec.QueryParam("sort", "Sort: name or newest", false),
			oapispec.QueryParam("limit", "Page size (default 50, max 100)", false),
			oapispec.QueryParam("offset", "Page offset (max 10000)", false),
			oapispec.Response(200, &handlers.ListAgentsResponse{}),
		)
		agentDetail := v1.Group("")
		agentDetail.Use(authMw.OptionalAuth())
		{
			api.GET(agentDetail, "/agents/:account", "List agents for account", handlers.ListAccountAgents(log, agentIndex, accountStore, heartStore, agentMetricsStore, deploymentStore, avatarStore, auditStore, authHandler.GetWorkOSClient()),
				oapispec.Tags("Agents"),
				oapispec.PathParam("account", "Account name"),
				oapispec.QueryParam("q", "Search by name or description", false),
				oapispec.QueryParam("tag", "Filter by exact tag", false),
				oapispec.QueryParam("visibility", "Filter: public or private", false),
				oapispec.QueryParam("sort", "Sort: name or newest", false),
				oapispec.QueryParam("limit", "Page size (default 50, max 100)", false),
				oapispec.QueryParam("offset", "Page offset (max 10000)", false),
				oapispec.Response(200, &handlers.ListAgentsResponse{}),
				oapispec.Response(404, &handlers.ErrorResponse{}),
			)
			api.GET(agentDetail, "/agents/:account/:name", "Get agent details", handlers.GetAgent(log, agentIndex, accountStore, heartStore, agentMetricsStore, deploymentStore, avatarStore, auditStore, authHandler.GetWorkOSClient()),
				oapispec.Tags("Agents"),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("name", "Agent name"),
				oapispec.Response(200, &handlers.AgentResponse{}),
				oapispec.Response(404, &handlers.ErrorResponse{}),
			)
		}

		// Account endpoints (public read)
		api.GET(v1, "/accounts/:account", "Get account details", handlers.GetAccount(log, accountStore, avatarStore, authHandler.GetWorkOSClient(), clusterid.New(cfg.Deployment.DefaultClusterID)),
			oapispec.Tags("Accounts"),
			oapispec.PathParam("account", "Account name"),
			oapispec.Response(200, &handlers.AccountResponse{}),
			oapispec.Response(404, &handlers.ErrorResponse{}),
		)
		api.GET(v1, "/accounts/:account/orgs", "Get org memberships for an account", handlers.GetAccountOrgs(log, accountStore),
			oapispec.Tags("Accounts"),
			oapispec.PathParam("account", "Account name"),
			oapispec.Response(200, gin.H{"orgs": []handlers.AccountOrgResponse{}}),
			oapispec.Response(404, &handlers.ErrorResponse{}),
		)
		api.GET(v1, "/accounts/:account/hearts", "List blueprints hearted by an account", handlers.ListHearted(log, heartStore, accountStore),
			oapispec.Tags("Accounts"),
			oapispec.PathParam("account", "Account name"),
			oapispec.QueryParam("cursor", "Pagination cursor", false),
			oapispec.QueryParam("limit", "Page size (default 20, max 100)", false),
			oapispec.Response(200, gin.H{"items": []heartstore.HeartedAgent{}}),
			oapispec.Response(404, &handlers.ErrorResponse{}),
		)
		api.GET(v1, "/accounts/check/:name", "Check account name availability", handlers.CheckAccountName(log, accountStore),
			oapispec.Tags("Accounts"),
			oapispec.PathParam("name", "Account name to check"),
			oapispec.Response(200, &handlers.CheckAccountNameResponse{}),
		)

		// Protected endpoints (require authentication)
		protected := v1.Group("")
		protected.Use(authMw.RequireAuth())
		deploymentRoutes := middleware.NewDeploymentRoutes(api, protected, deploymentRouteCatalog)
		if cfg.FGAEnforcementEnabled && cfg.Auth.WorkOSAPIKey != "" {
			protected.Use(middleware.EnforceDeploymentAuthorization(log, deploymentFGAEnforcementChecker, deploymentRouteCatalog))
			log.Info("authz: deployment FGA mutation enforcement enabled")
		}
		if cfg.FGAShadowEnabled && cfg.Auth.WorkOSAPIKey != "" {
			protected.Use(middleware.ObserveDeploymentAuthorization(
				log,
				authz.NewGatedShadowChecker(log, deploymentFGAShadowRollout, membershipChecker, deploymentFGALiveChecker),
				deploymentRouteCatalog,
			))
			log.Info("authz: deployment FGA shadow checks enabled")
		}
		{
			// Profile
			api.GET(protected, "/me", "Get current user profile", handlers.GetProfile(log, accountStore, agentIndex),
				oapispec.Tags("Profile"),
				oapispec.BearerAuth(),
				oapispec.Response(200, &handlers.ProfileResponse{}),
			)
			api.PATCH(protected, "/me", "Update current user profile", handlers.UpdateProfile(log, accountStore, auditStore),
				oapispec.Tags("Profile"),
				oapispec.BearerAuth(),
				oapispec.Body(&handlers.UpdateProfileRequest{}),
				oapispec.Response(200, &handlers.UpdateProfileResponse{}),
			)

			// Browser Inbox connection config for the current user (in-app feed).
			api.GET(protected, "/notifications/inbox-config", "Get the in-app notification Inbox config",
				handlers.GetNotificationInboxConfig(log, notifyNovuClient, cfg.Notify.AppIdentifier, cfg.Notify.NovuAPIURL, cfg.Notify.SocketURL),
				oapispec.Tags("Notifications"),
				oapispec.BearerAuth(),
				oapispec.Response(200, &handlers.NotificationInboxConfig{}),
			)

			// Account management
			api.GET(protected, "/accounts/search", "Search accounts", handlers.SearchAccounts(log, accountStore, avatarStore),
				oapispec.Tags("Accounts"),
				oapispec.BearerAuth(),
				oapispec.QueryParam("q", "Search query (min 3 chars)", true),
				oapispec.QueryParam("type", "Filter by type: personal or organization", false),
				oapispec.QueryParam("limit", "Max results (default 10, max 10)", false),
				oapispec.Response(200, &handlers.SearchAccountsResponse{}),
			)
			api.GET(protected, "/me/blueprints", "List blueprints visible to the current user", handlers.ListUserBlueprints(log, agentIndex, accountStore, avatarStore, k8sCache),
				oapispec.Tags("Accounts", "Agents"),
				oapispec.BearerAuth(),
				oapispec.QueryParam("account", "Repeated account name; required unless scope=all", false),
				oapispec.QueryParam("scope", "Set to all for an explicit all-memberships read", false),
				oapispec.QueryParam("q", "Search name, description, and tags", false),
				oapispec.QueryParam("tag", "Filter by exact tag", false),
				oapispec.QueryParam("visibility", "Filter by public or private visibility", false),
				oapispec.QueryParam("sort", "Sort by name or newest", false),
				oapispec.QueryParam("limit", "Global page size (default 50, max 100)", false),
				oapispec.QueryParam("cursor", "Opaque keyset pagination cursor returned by the previous page", false),
				oapispec.Response(200, &handlers.UserBlueprintsResponse{}),
			)
			api.GET(protected, "/me/knowledge", "List knowledge stores visible to the current user", handlers.ListUserKnowledgeStores(log, accountStore, ksStore, k8sCache),
				oapispec.Tags("Accounts", "Knowledge"),
				oapispec.BearerAuth(),
				oapispec.QueryParam("account", "Repeated account name; required unless scope=all", false),
				oapispec.QueryParam("scope", "Set to all for an explicit all-memberships read", false),
				oapispec.QueryParam("q", "Search knowledge store name", false),
				oapispec.QueryParam("limit", "Global page size (default 50, max 100)", false),
				oapispec.QueryParam("cursor", "Opaque keyset pagination cursor returned by the previous page", false),
				oapispec.Response(200, &handlers.UserKnowledgeResponse{}),
			)
			api.GET(protected, "/me/deployments", "List deployments visible to the current user", handlers.ListUserDeployments(log, accountStore, deploymentStore, agentIndex, auditStore, k8sCache, deploymentDiscovery),
				oapispec.Tags("Accounts", "Deployments"),
				oapispec.BearerAuth(),
				oapispec.QueryParam("account", "Repeated account name; required unless scope=all", false),
				oapispec.QueryParam("scope", "Set to all for an explicit all-memberships read", false),
				oapispec.QueryParam("q", "Search agent name or display name", false),
				oapispec.QueryParam("limit", "Global page size (default 50, max 100)", false),
				oapispec.QueryParam("cursor", "Opaque cursor returned by the previous page", false),
				oapispec.Response(200, &handlers.UserDeploymentsResponse{}),
			)
			api.GET(protected, "/me/deployment-summaries", "Get cached summaries for visible deployments", handlers.ListUserDeploymentSummaries(log, accountStore, deploymentStore, k8sCache, deploymentDiscovery),
				oapispec.Tags("Accounts", "Deployments", "Observability"),
				oapispec.BearerAuth(),
				oapispec.QueryParam("deployment", "Repeated visible deployment ID (max 100)", true),
				oapispec.Response(200, &handlers.DeploymentSummariesResponse{}),
			)
			api.POST(protected, "/accounts", "Create an account", handlers.CreateAccount(log, accountStore, orgProvisioner, orgSync, memberEmailStore, billingProvider, auditStore, queue, resourceLifecycle),
				oapispec.Tags("Accounts"),
				oapispec.BearerAuth(),
				oapispec.Body(&handlers.CreateAccountRequest{}),
				oapispec.Response(201, &handlers.AccountResponse{}),
				oapispec.Response(409, &handlers.ErrorResponse{}),
			)

			accountExperiments := protected.Group("/accounts/:account")
			accountExperiments.Use(middleware.ResolveAccount(accountStore))
			accountExperiments.Use(middleware.RequireAccountPermission(accountStore, "org:manage"))

			// Deleting an account is the one action the org:manage role cannot
			// take: it archives billing and tears down every deployment, and
			// nothing undoes it after the retention window. Only the owner, or
			// an operator through the admin console, runs this sequence.
			deps.Clients.AccountDeleter = &accountlifecycle.Deleter{
				Log:         log,
				Accounts:    accountStore,
				Deployments: deploymentStore,
				Undeploy: func(ctx context.Context, deploymentID, clusterID string) error {
					return handlers.EnqueueUndeploy(ctx, deploymentStore, queue, deploymentID, clusterID)
				},
				Org:            orgClient,
				Resources:      resourceLifecycle,
				Billing:        billingProvider,
				BillingBackend: cfg.BillingBackend(),
				AIGateway:      aiGatewayProvisioner,
				JudgeKeys:      aiGatewayJudgeStore,
			}

			deps.Clients.AccountPurger = accountlifecycle.NewPurger(accountlifecycle.PurgerDeps{
				Log:         log,
				DB:          db,
				Deployments: deploymentStore,
				Langfuse:    ingestLangfuseProvisioner,
				AIGateway:   aiGatewayProvisioner,
				Undeploy:    queue.UndeployFunc(deploymentStore),
			})

			accountOwner := protected.Group("/accounts/:account")
			accountOwner.Use(middleware.ResolveAccount(accountStore))
			accountOwner.Use(middleware.RequireAccountOwner(accountStore))
			{
				api.DELETE(accountOwner, "", "Delete account", handlers.DeleteAccount(log, deps.Clients.AccountDeleter, auditStore, accountStore, billingProvider, deps.Stores.BillingStatus, cfg.BillingBackend()),
					oapispec.Tags("Accounts"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.MessageResponse{}),
					oapispec.Response(409, &handlers.ErrorResponse{}),
					oapispec.Response(500, &handlers.ErrorResponse{}),
				)
			}

			accountSettings := protected.Group("/accounts/:account")
			accountSettings.Use(middleware.ResolveAccount(accountStore))
			accountSettings.Use(middleware.RequireAccountPermission(accountStore, "org:manage"))
			{
				api.PATCH(accountSettings, "", "Update account", handlers.UpdateAccount(log, accountStore, auditStore),
					oapispec.Tags("Accounts"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.UpdateAccountRequest{}),
					oapispec.Response(200, &handlers.MessageResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
				)
				api.PUT(accountSettings, "", "Rename account", handlers.RenameAccount(log, accountStore, agentIndex, avatarStore, orgClient, auditStore),
					oapispec.Tags("Accounts"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.RenameAccountRequest{}),
					oapispec.Response(200, &handlers.RenameAccountResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
				)
				if avatarStore != nil {
					api.POST(accountSettings, "/avatar", "Upload account avatar", handlers.UploadAvatar(log, accountStore, avatarStore, auditStore),
						oapispec.Tags("Avatars"),
						oapispec.BearerAuth(),
						oapispec.PathParam("account", "Account name"),
						oapispec.Response(200, &handlers.AvatarResponse{}),
						oapispec.Response(400, &handlers.ErrorResponse{}),
					)
					api.PUT(accountSettings, "/avatar/preset/:index", "Set avatar to preset", handlers.SetAvatarPreset(log, accountStore, avatarStore, auditStore),
						oapispec.Tags("Avatars"),
						oapispec.BearerAuth(),
						oapispec.PathParam("account", "Account name"),
						oapispec.PathParam("index", "Preset index (1-25)"),
						oapispec.Response(200, &handlers.AvatarResponse{}),
						oapispec.Response(400, &handlers.ErrorResponse{}),
					)
					api.DELETE(accountSettings, "/avatar", "Reset account avatar", handlers.ResetAvatar(log, accountStore, avatarStore, auditStore),
						oapispec.Tags("Avatars"),
						oapispec.BearerAuth(),
						oapispec.PathParam("account", "Account name"),
						oapispec.Response(200, &handlers.AvatarResponse{}),
					)
				}
				// Audit log
				api.GET(accountSettings, "/audit-log/filters", "List audit log filter options", handlers.ListAuditLogFilters(log, auditStore),
					oapispec.Tags("Audit"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &auditlog.FilterOptions{}),
				)
				api.GET(accountSettings, "/audit-log", "List audit log entries", handlers.ListAuditLog(log, auditStore),
					oapispec.Tags("Audit"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.QueryParam("resource_type", "Filter by resource type", false),
					oapispec.QueryParam("resource_id", "Filter by resource ID", false),
					oapispec.QueryParam("action", "Filter by action", false),
					oapispec.QueryParam("actor_id", "Filter by actor ID", false),
					oapispec.QueryParam("before", "Cursor for pagination (RFC3339)", false),
					oapispec.QueryParam("limit", "Page size (default 50, max 200)", false),
					oapispec.Response(200, &handlers.AuditLogListResponse{}),
				)
				api.GET(accountExperiments, "/experiments/:experiment", "Get an account experiment", handlers.GetAccountExperiment(log, experimentStore, accountStore),
					oapispec.Tags("Accounts", "Experiments"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.ExperimentResponse{}),
				)
				api.PUT(accountExperiments, "/experiments/:experiment", "Update an account experiment", handlers.UpdateAccountExperiment(log, experimentStore, auditStore, deploymentDiscovery, accountStore),
					oapispec.Tags("Accounts", "Experiments"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.UpdateExperimentRequest{}),
					oapispec.Response(200, &handlers.ExperimentResponse{}),
				)
				api.POST(accountSettings, "/access-groups", "Create access group", accessGroupHandler.Create,
					oapispec.Tags("Authorization"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.AccessGroupRequest{}),
					oapispec.Response(201, &handlers.AccessGroupResponse{}),
				)
				api.PATCH(accountSettings, "/access-groups/:group_id", "Update access group", accessGroupHandler.Update,
					oapispec.Tags("Authorization"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"), oapispec.PathParam("group_id", "WorkOS group ID"),
					oapispec.Body(&handlers.AccessGroupRequest{}),
					oapispec.Response(200, &handlers.AccessGroupResponse{}),
				)
				api.DELETE(accountSettings, "/access-groups/:group_id", "Delete access group", accessGroupHandler.Delete,
					oapispec.Tags("Authorization"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"), oapispec.PathParam("group_id", "WorkOS group ID"),
					oapispec.Response(204, nil),
				)
				api.GET(accountSettings, "/access-groups/:group_id/members", "List access group members", accessGroupHandler.ListMembers,
					oapispec.Tags("Authorization"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"), oapispec.PathParam("group_id", "WorkOS group ID"),
					oapispec.QueryParam("limit", "Page size (default 50, max 100)", false),
					oapispec.QueryParam("cursor", "Opaque cursor returned by the previous page", false),
					oapispec.Response(200, &handlers.AccessGroupMemberPageResponse{}),
				)
				api.POST(accountSettings, "/access-groups/:group_id/members", "Add access group member", accessGroupHandler.AddMember,
					oapispec.Tags("Authorization"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"), oapispec.PathParam("group_id", "WorkOS group ID"),
					oapispec.Body(&handlers.AddAccessGroupMemberRequest{}), oapispec.Response(204, nil),
				)
				api.DELETE(accountSettings, "/access-groups/:group_id/members/:user_id", "Remove access group member", accessGroupHandler.RemoveMember,
					oapispec.Tags("Authorization"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"), oapispec.PathParam("group_id", "WorkOS group ID"),
					oapispec.PathParam("user_id", "Astro user ID"), oapispec.Response(204, nil),
				)

				api.GET(accountSettings, "/app-scopes", "List scopes an app may hold", appHandler.ListScopes,
					oapispec.Tags("Apps"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.AppScopesResponse{}),
				)
				api.GET(accountSettings, "/apps", "List machine apps", appHandler.List,
					oapispec.Tags("Apps"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.AppListResponse{}),
				)
				api.POST(accountSettings, "/apps", "Create a machine app", appHandler.Create,
					oapispec.Tags("Apps"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.CreateAppRequest{}),
					oapispec.Response(201, &handlers.CreateAppResponse{}),
				)
				api.PATCH(accountSettings, "/apps/:app_id", "Update an app's scopes", appHandler.UpdateScopes,
					oapispec.Tags("Apps"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"), oapispec.PathParam("app_id", "App ID"),
					oapispec.Body(&handlers.UpdateAppScopesRequest{}),
					oapispec.Response(200, &handlers.AppResponse{}),
				)
				api.DELETE(accountSettings, "/apps/:app_id", "Delete a machine app", appHandler.Delete,
					oapispec.Tags("Apps"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"), oapispec.PathParam("app_id", "App ID"),
					oapispec.Response(204, nil),
				)
				api.POST(accountSettings, "/apps/:app_id/secrets", "Create an app secret", appHandler.CreateSecret,
					oapispec.Tags("Apps"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"), oapispec.PathParam("app_id", "App ID"),
					oapispec.Response(201, &connectapps.NewSecret{}),
				)
				api.DELETE(accountSettings, "/apps/:app_id/secrets/:secret_id", "Revoke an app secret", appHandler.DeleteSecret,
					oapispec.Tags("Apps"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"), oapispec.PathParam("app_id", "App ID"),
					oapispec.PathParam("secret_id", "Secret ID"), oapispec.Response(204, nil),
				)
			}

			// Account-scoped routes (admin or owner — org:manage).
			// Billing (usage, invoices, balances, payment methods) lives here:
			// financial data and card management are restricted to org
			// admins/owners. Personal accounts have a single member who is the
			// owner and thus holds org:manage implicitly.
			accountManage := protected.Group("/accounts/:account")
			accountManage.Use(middleware.ResolveAccount(accountStore))
			accountManage.Use(middleware.RequireAccountPermission(accountStore, "org:manage"))
			{
				api.GET(accountManage, "/quota-increase", "List quota increase requests", handlers.ListQuotaIncreaseRequests(log, db),
					oapispec.Tags("Usage"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.QuotaIncreaseListResponse{}),
				)
				api.POST(accountManage, "/quota-increase", "Request quota increase", handlers.RequestQuotaIncrease(log, db),
					oapispec.Tags("Usage"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(201, &handlers.QuotaIncreaseResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
				)

				// OTel ingest keys — account-scoped telemetry credential for
				// local coding tools (e.g. Claude Code). Requires org:manage:
				// the key is forced org-wide onto developer machines.
				api.GET(accountManage, "/otel-keys", "List OTel ingest keys", handlers.ListOtelIngestTokens(log, ingestTokenStore, cfg),
					oapispec.Tags("Observability"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.ListOtelIngestTokensResponse{}),
				)
				api.POST(accountManage, "/otel-keys", "Create an OTel ingest key", handlers.CreateOtelIngestToken(log, ingestTokenStore, ingestLangfuseProvisioner, ingestLangfuseStore, cfg, queue),
					oapispec.Tags("Observability"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.CreateOtelIngestTokenRequest{}),
					oapispec.Response(201, &handlers.CreateOtelIngestTokenResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
				)
				api.DELETE(accountManage, "/otel-keys/:tokenID", "Revoke an OTel ingest key", handlers.RevokeOtelIngestToken(log, ingestTokenStore, queue),
					oapispec.Tags("Observability"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("tokenID", "Ingest key ID"),
					oapispec.Response(200, &handlers.MessageResponse{}),
					oapispec.Response(404, &handlers.ErrorResponse{}),
				)
				api.PATCH(accountManage, "/otel-keys/:tokenID/name", "Rename an OTel ingest key", handlers.RenameOtelIngestToken(log, ingestTokenStore),
					oapispec.Tags("Observability"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("tokenID", "Ingest key ID"),
					oapispec.Body(&handlers.RenameOtelIngestTokenRequest{}),
					oapispec.Response(200, &handlers.RenameOtelIngestTokenResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
					oapispec.Response(404, &handlers.ErrorResponse{}),
				)
				api.PATCH(accountManage, "/otel-keys/:tokenID/exclusions", "Update an OTel ingest key's exclusion list", handlers.UpdateOtelIngestTokenExclusions(log, ingestTokenStore),
					oapispec.Tags("Observability"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("tokenID", "Ingest key ID"),
					oapispec.Body(&handlers.UpdateOtelIngestTokenExclusionsRequest{}),
					oapispec.Response(200, &handlers.UpdateOtelIngestTokenExclusionsResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
					oapispec.Response(404, &handlers.ErrorResponse{}),
				)

				// Local-only: force an immediate classification pass instead of
				// waiting for the hourly tick. Deliberately not registered in
				// deployed environments — it drives load onto the shared Foundry
				// inference pool and exists purely for local iteration.
				if cfg.Deployment.IsLocal() {
					api.POST(accountManage, "/classification/run", "Run a classification pass now (local only)", handlers.RunClassification(log, queue),
						oapispec.Tags("Observability"),
						oapispec.BearerAuth(),
						oapispec.PathParam("account", "Account name"),
						oapispec.Response(202, &handlers.RunClassificationResponse{}),
						oapispec.Response(503, &handlers.ErrorResponse{}),
					)
				}

				api.GET(accountManage, "/billing/usage", "Get billing usage", handlers.GetBillingUsage(log, accountStore, billingProvider, cfg.BillingBackend()),
					oapispec.Tags("Billing"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.QueryParam("from", "Start of period (RFC3339, defaults to start of current month)", false),
					oapispec.QueryParam("to", "End of period (RFC3339, defaults to now)", false),
					oapispec.Response(200, &handlers.BillingDataResponse{}),
					oapispec.Response(502, &handlers.ErrorResponse{}),
				)

				api.GET(accountManage, "/billing/usage/daily-spend", "Get billing spend per day", handlers.GetBillingDailySpend(log, accountStore, billingProvider, cfg.BillingBackend()),
					oapispec.Tags("Billing"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.QueryParam("from", "Start of period (RFC3339, defaults to start of current month)", false),
					oapispec.QueryParam("to", "End of period (RFC3339, defaults to now)", false),
					oapispec.Response(200, &handlers.BillingDataResponse{}),
					oapispec.Response(502, &handlers.ErrorResponse{}),
				)

				api.GET(accountManage, "/billing/invoices", "Get billing invoices", handlers.GetBillingInvoices(log, accountStore, billingProvider, cfg.BillingBackend()),
					oapispec.Tags("Billing"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.BillingDataResponse{}),
					oapispec.Response(502, &handlers.ErrorResponse{}),
				)

				// Binary PDF stream — registered directly (not via the OpenAPI
				// helper) since it returns application/pdf, not JSON.
				accountManage.GET("/billing/invoices/:invoiceId/pdf", handlers.GetBillingInvoicePDF(log, accountStore, billingProvider, cfg.BillingBackend()))

				api.GET(accountManage, "/billing/balances", "Get billing credits and commits", handlers.GetBillingBalances(log, accountStore, billingProvider, cfg.BillingBackend()),
					oapispec.Tags("Billing"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.BillingDataResponse{}),
					oapispec.Response(502, &handlers.ErrorResponse{}),
				)

				api.GET(accountManage, "/billing/spend", "Get current spend and the account's own thresholds", handlers.GetBillingSpend(log, accountStore, billingProvider, cfg.BillingBackend()),
					oapispec.Tags("Billing"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.BillingDataResponse{}),
				)

				api.PUT(accountManage, "/billing/spend/thresholds", "Set the account's own spend warning and limit", handlers.SetBillingSpendThresholds(log, accountStore, billingProvider, cfg.BillingBackend(), deps.Stores.BillingStatus, queue),
					oapispec.Tags("Billing"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.BillingDataResponse{}),
				)

				api.PUT(accountManage, "/billing/usage/thresholds", "Set the account's own usage warning and limit for one metric", handlers.SetBillingUsageThresholds(log, accountStore, billingProvider, cfg.BillingBackend(), deps.Stores.BillingStatus, queue),
					oapispec.Tags("Billing"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.BillingDataResponse{}),
				)

				api.GET(accountManage, "/billing/status", "Get billing gating status", handlers.GetBillingStatus(log, deps.Stores.BillingStatus, deps.Stores.Deployment, cfg.BillingGateEnforce),
					oapispec.Tags("Billing"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.BillingStatusResponse{}),
					oapispec.Response(500, &handlers.ErrorResponse{}),
				)

				// Payment method (Stripe card vault). A SetupIntent collects the
				// card client-side; the confirm endpoint saves it and links the
				// Stripe customer to the billing provider for charging.
				api.POST(accountManage, "/billing/setup-intent", "Start payment-method setup", handlers.CreateSetupIntent(log, accountStore, paymentProvider),
					oapispec.Tags("Billing"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.SetupIntentResponse{}),
					oapispec.Response(502, &handlers.ErrorResponse{}),
				)

				api.POST(accountManage, "/billing/payment-method", "Confirm and save a payment method", handlers.ConfirmPaymentMethod(log, accountStore, auditStore, paymentProvider, billingProvider, cfg.BillingBackend(), deps.Stores.BillingStatus, queue),
					oapispec.Tags("Billing"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.PaymentMethodResponse{}),
					oapispec.Response(502, &handlers.ErrorResponse{}),
				)

				api.GET(accountManage, "/billing/payment-method", "Get the saved payment method", handlers.GetPaymentMethod(log, accountStore, paymentProvider),
					oapispec.Tags("Billing"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.PaymentMethodResponse{}),
					oapispec.Response(502, &handlers.ErrorResponse{}),
				)

				api.DELETE(accountManage, "/billing/payment-method", "Remove the saved payment method", handlers.DeletePaymentMethod(log, accountStore, deploymentStore, auditStore, paymentProvider, billingProvider, cfg.BillingBackend(), deps.Stores.BillingStatus, queue),
					oapispec.Tags("Billing"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.MessageResponse{}),
					oapispec.Response(409, &handlers.ErrorResponse{}),
					oapispec.Response(500, &handlers.ErrorResponse{}),
					oapispec.Response(502, &handlers.ErrorResponse{}),
				)

				// Notification preferences are per-user, owned by Novu (subscriber =
				// current user). Any member manages their own; the account scope here
				// is just the auth/membership boundary + test-send context.
				api.GET(accountManage, "/notification-preferences", "Get the current user's notification preferences",
					handlers.GetNotificationPreferences(log, notifyNovuClient),
					oapispec.Tags("Notifications"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.NotificationPreferencesResponse{}),
					oapispec.Response(500, &handlers.ErrorResponse{}),
				)
				api.PATCH(accountManage, "/notification-preferences", "Update a notification preference",
					handlers.UpdateNotificationPreference(log, notifyNovuClient),
					oapispec.Tags("Notifications"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.UpdateNotificationPreferenceRequest{}),
					oapispec.Response(200, &handlers.MessageResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
				)
				api.POST(accountManage, "/notification-preferences/test", "Send a test notification to the current user",
					handlers.SendTestNotification(log, queue, cfg.Notify.TestWorkflowID),
					oapispec.Tags("Notifications"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(202, &handlers.MessageResponse{}),
					oapispec.Response(500, &handlers.ErrorResponse{}),
				)
			}

			// Account variables (vault). The FGA model's variable:read and
			// variable:manage are not org role permissions yet, so reads ride on
			// deployments:read and writes on org:manage until FGA enforces them.
			accountVarsRead := protected.Group("/accounts/:account")
			accountVarsRead.Use(middleware.ResolveAccount(accountStore))
			accountVarsRead.Use(middleware.RequireAccountPermission(accountStore, "deployments:read"))
			{
				api.GET(accountVarsRead, "/variables", "List account variables", handlers.ListAccountVariables(log, accountVarsStore),
					oapispec.Tags("Variables"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.ListAccountVariablesResponse{}),
				)
				api.GET(accountVarsRead, "/variables/:varName", "Get account variable", handlers.GetAccountVariable(log, accountVarsStore),
					oapispec.Tags("Variables"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("varName", "Variable name"),
					oapispec.Response(200, &accountvars.VariableMetadata{}),
				)
			}
			accountVarsWrite := protected.Group("/accounts/:account")
			accountVarsWrite.Use(middleware.ResolveAccount(accountStore))
			accountVarsWrite.Use(middleware.RequireAccountPermission(accountStore, "org:manage"))
			{
				api.POST(accountVarsWrite, "/variables", "Create account variables", handlers.CreateAccountVariable(log, accountVarsStore, deps.Vault),
					oapispec.Tags("Variables"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.CreateAccountVariablesRequest{}),
					oapispec.Response(200, &handlers.CreateAccountVariablesResponse{}),
				)
				api.PUT(accountVarsWrite, "/variables/:varName", "Update account variable", handlers.UpdateAccountVariable(log, accountVarsStore, deps.Vault),
					oapispec.Tags("Variables"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("varName", "Variable name"),
					oapispec.Body(&handlers.UpdateAccountVariableRequest{}),
					oapispec.Response(200, nil),
				)
				api.DELETE(accountVarsWrite, "/variables/:varName", "Delete account variable", handlers.DeleteAccountVariable(log, accountVarsStore),
					oapispec.Tags("Variables"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("varName", "Variable name"),
					oapispec.Response(200, nil),
				)
			}

			// Account-scoped routes (any member)
			accountMember := protected.Group("/accounts/:account")
			accountMember.Use(middleware.ResolveAccount(accountStore))
			accountMember.Use(middleware.RequireAccountMember(accountStore))
			{
				api.GET(accountMember, "/access-groups", "List access groups", accessGroupHandler.List,
					oapispec.Tags("Authorization"), oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.QueryParam("limit", "Page size (default 50, max 100)", false),
					oapispec.QueryParam("cursor", "Opaque cursor returned by the previous page", false),
					oapispec.Response(200, &handlers.AccessGroupPageResponse{}),
				)
				api.GET(accountMember, "/usage", "Get account usage", handlers.GetAccountUsage(log, quotaChecker),
					oapispec.Tags("Usage"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.QueryParam("from", "Start of period (RFC3339, defaults to start of current month)", false),
					oapispec.QueryParam("to", "End of period (RFC3339, defaults to now)", false),
					oapispec.Response(200, &handlers.UsageResponse{}),
					oapispec.Response(503, &handlers.ErrorResponse{}),
				)

				// AI Gateway dev key issuance — astro CLI calls this on `astro
				// dev` startup. Each call mints a fresh short-lived key; the
				// LiteLLM-side TTL is the only lifecycle mechanism, so the CLI
				// has no cleanup responsibility.
				//
				// Gated: this is the one spend path a suspended account can
				// still reach, because unlike a deployment it needs no running
				// workload.
				api.POST(accountMember, "/ai-gateway-keys", "Issue an ephemeral AI Gateway key for local dev",
					ent.Wrap(handlers.IssueAIGatewayDevKey(log, aiGatewayProvisioner, aiGatewayDevStore, deps.Vault)),
					oapispec.Tags("AI Gateway"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.AIGatewayKeyResponse{}),
					oapispec.Response(402, &handlers.ErrorResponse{}),
					oapispec.Response(502, &handlers.ErrorResponse{}),
					oapispec.Response(503, &handlers.ErrorResponse{}),
				)
				api.GET(accountMember, "/usage/infrastructure", "Get account infrastructure usage", handlers.GetInfrastructureUsage(log),
					oapispec.Tags("Usage"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.QueryParam("from", "Start of period (RFC3339, defaults to start of current month)", false),
					oapispec.QueryParam("to", "End of period (RFC3339, defaults to now)", false),
					oapispec.Response(200, &handlers.InfrastructureUsageResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
					oapispec.Response(503, &handlers.ErrorResponse{}),
				)

				// Knowledge store routes
				api.POST(accountMember, "/knowledge/connect", "Connect an external knowledge store", ent.Wrap(quotaChecker.Wrap(handlers.ConnectKnowledgeStore(log, ksStore, pipesClient, cfg, deps.Vault, queue, db, quotaChecker, resourceLifecycle), "knowledge_stores")),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.KnowledgeResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
					oapispec.Response(402, &handlers.ErrorResponse{}),
					oapispec.Response(409, &handlers.ErrorResponse{}),
				)
				api.GET(accountMember, "/knowledge", "List knowledge stores", handlers.ListKnowledgeStores(log, ksStore),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &[]handlers.KnowledgeResponse{}),
				)
				api.GET(accountMember, "/knowledge/:name", "Get a knowledge store", handlers.GetKnowledgeStore(log, ksStore),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Store name"),
					oapispec.Response(200, &handlers.KnowledgeResponse{}),
					oapispec.Response(404, &handlers.ErrorResponse{}),
				)
				api.POST(accountMember, "/knowledge/:name/recheck", "Recheck a connected knowledge store and fix its host", handlers.RecheckKnowledgeStore(log, ksStore, nil, deps.Vault),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Store name"),
					oapispec.Response(200, &handlers.KnowledgeResponse{}),
					oapispec.Response(404, &handlers.ErrorResponse{}),
				)
				api.DELETE(accountMember, "/knowledge/:name", "Delete a knowledge store", handlers.DeleteKnowledgeStore(log, ksStore, queue, resourceLifecycle),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Store name"),
					oapispec.Response(200, &handlers.MessageResponse{}),
					oapispec.Response(404, &handlers.ErrorResponse{}),
				)
				api.GET(accountMember, "/knowledge/:name/credentials", "Retrieve knowledge store credentials", handlers.GetKnowledgeStoreCredentials(log, ksStore, deps.Vault),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Store name"),
					oapispec.Response(200, &handlers.KnowledgeCredentialsResponse{}),
					oapispec.Response(404, &handlers.ErrorResponse{}),
				)
				api.PUT(accountMember, "/knowledge/:name/credentials", "Update connected knowledge store credentials", handlers.UpdateKnowledgeStoreCredentials(log, ksStore, pipesClient, cfg, deps.Vault),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Store name"),
					oapispec.Response(200, &handlers.KnowledgeResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
					oapispec.Response(404, &handlers.ErrorResponse{}),
				)
			}

			// Member routes — list and self-removal are membership-only, other mutations require org:manage
			memberRoutes := protected.Group("/accounts/:account/members")
			memberRoutes.Use(middleware.ResolveAccount(accountStore))
			memberRoutes.Use(middleware.RequireAccountMember(accountStore))
			{
				api.GET(memberRoutes, "", "List account members", handlers.ListMembers(log, accountStore, avatarStore, orgClient, slackIdentityStore, memberEmailStore, inviteeLookup),
					oapispec.Tags("Members"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.ListMembersResponse{}),
				)
				// Remove member — handler allows self-removal for any member,
				// but requires org:manage to remove others.
				api.DELETE(memberRoutes, "/:user_id", "Remove a member", handlers.RemoveMember(log, orgSync, accountStore, db, auditStore, queue),
					oapispec.Tags("Members"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("user_id", "User ID"),
					oapispec.Response(200, &handlers.MessageResponse{}),
				)
			}
			memberManageRoutes := memberRoutes.Group("")
			memberManageRoutes.Use(middleware.RequireAccountPermission(accountStore, "org:manage"))
			{
				api.POST(memberManageRoutes, "", "Add a member",
					quotaChecker.Wrap(handlers.AddMember(log, orgSync, accountStore, db, auditStore, queue), "members"),
					oapispec.Tags("Members"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.AddMemberRequest{}),
					oapispec.Response(201, &handlers.AddMemberResponse{}),
				)
				api.PUT(memberManageRoutes, "/:user_id", "Update member role", handlers.UpdateMemberRole(log, orgSync, accountStore, auditStore, queue),
					oapispec.Tags("Members"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("user_id", "User ID"),
					oapispec.Body(&handlers.ChangeMemberRoleRequest{}),
					oapispec.Response(200, &handlers.MessageResponse{}),
				)
			}

			// Invitation management (requires org:manage permission)
			invitationRoutes := protected.Group("/accounts/:account/invitations")
			invitationRoutes.Use(middleware.ResolveAccount(accountStore))
			invitationRoutes.Use(middleware.RequireAccountPermission(accountStore, "org:manage"))
			{
				api.GET(invitationRoutes, "", "List account invitations", handlers.ListAccountInvitations(log, orgClient),
					oapispec.Tags("Invitations"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.ListInvitationsResponse{}),
				)
				api.POST(invitationRoutes, "", "Send invitations", handlers.CreateInvitations(log, orgSync, auditStore),
					oapispec.Tags("Invitations"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.BulkInvitationRequest{}),
					oapispec.Response(201, &handlers.BulkInvitationResponse{}),
				)
				api.DELETE(invitationRoutes, "/:id", "Revoke an invitation", handlers.RevokeInvitation(log, orgClient, auditStore),
					oapispec.Tags("Invitations"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("id", "Invitation ID"),
					oapispec.Response(200, &handlers.MessageResponse{}),
				)
			}

			// Deployment template generation
			tmplCache := handlers.NewTemplateCache()
			api.POST(protected, "/agents/:account/:name/deployment-template", "Interactive deployment template",
				handlers.PostDeploymentTemplate(log, agentIndex, accountStore, cfg, deploymentStore, ksStore, authzStore, tmplCache),
				oapispec.Tags("Agents"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("name", "Agent name"),
				oapispec.Desc("Accepts deploy-time inputs (adapters, variables), shapes the template, and returns validation. An empty body produces the same template as the GET endpoint."),
				oapispec.Response(200, nil),
			)

			// Agent heart toggle (requires auth, no account permission needed)
			api.POST(protected, "/agents/:account/:name/heart", "Toggle heart on an agent", handlers.ToggleHeart(log, heartStore, accountStore),
				oapispec.Tags("Agents"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("name", "Agent name"),
				oapispec.Response(200, &handlers.HeartResponse{}),
			)

			// Feedback
			api.POST(protected, "/feedback", "Submit feedback", handlers.SubmitFeedback(log, db),
				oapispec.Tags("Feedback"),
				oapispec.BearerAuth(),
				oapispec.Body(&handlers.FeedbackInput{}),
				oapispec.Response(201, &handlers.FeedbackResponse{}),
				oapispec.Response(400, &handlers.ErrorResponse{}),
				oapispec.Response(429, &handlers.ErrorResponse{}),
			)

			// Create blueprint shell (no build required)
			createBlueprintRoutes := protected.Group("/agents/:account")
			createBlueprintRoutes.Use(middleware.ResolveAccount(accountStore))
			createBlueprintRoutes.Use(middleware.RequireAccountPermission(accountStore, "agents:write"))
			{
				api.POST(createBlueprintRoutes, "", "Create a blueprint",
					quotaChecker.Wrap(handlers.CreateBlueprint(log, agentIndex, accountStore, auditStore, avatarStore, db, resourceLifecycle, roleProjector), quota.ResourceBlueprints),
					oapispec.Tags("Agents"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.CreateBlueprintRequest{}),
					oapispec.Response(201, &handlers.CreateBlueprintResponse{}),
				)
			}

			// Base group for all per-agent routes — resolves account once, shared by read and write sub-groups.
			agentRoutes := protected.Group("/agents/:account/:name")
			agentRoutes.Use(middleware.ResolveAccount(accountStore))

			// Agent read operations (requires account membership)
			agentReadRoutes := agentRoutes.Group("")
			agentReadRoutes.Use(middleware.RequireAccountMember(accountStore))
			{
				api.GET(agentReadRoutes, "/usage/infrastructure", "Get agent infrastructure usage", handlers.GetInfrastructureUsage(log),
					oapispec.Tags("Usage"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Agent name"),
					oapispec.QueryParam("from", "Start of period (RFC3339, defaults to start of current month)", false),
					oapispec.QueryParam("to", "End of period (RFC3339, defaults to now)", false),
					oapispec.Response(200, &handlers.InfrastructureUsageResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
					oapispec.Response(404, &handlers.ErrorResponse{}),
					oapispec.Response(503, &handlers.ErrorResponse{}),
				)
			}

			// Agent write operations (requires agents:write permission)
			agentWriteRoutes := agentRoutes.Group("")
			agentWriteRoutes.Use(middleware.RequireAccountPermission(accountStore, "agents:write"))
			{
				api.POST(agentWriteRoutes, "/register", "Register an agent build",
					quotaChecker.WrapRegister(handlers.RegisterAgent(log, agentIndex, cfg.Server.MinCLIVersion, db, auditStore, avatarStore, deploymentStore, k8sCache, cfg.Deployment.AIGatewayURL != "", resourceLifecycle, roleProjector)),
					oapispec.Tags("Agents"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Agent name"),
					oapispec.Body(&handlers.RegisterAgentRequest{}),
					oapispec.Response(201, &handlers.RegisterAgentResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
					oapispec.Response(426, &handlers.ErrorResponse{}),
				)
				api.POST(agentWriteRoutes, "/archive", "Archive an agent template", handlers.ArchiveAgent(log, agentIndex, db, auditStore, ghStore, webhookStore, pipesClient, resourceLifecycle),
					oapispec.Tags("Agents"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Agent name"),
					oapispec.Response(204, nil),
					oapispec.Response(500, &handlers.ErrorResponse{}),
				)
				api.PUT(agentWriteRoutes, "/visibility", "Set agent visibility", handlers.SetAgentVisibility(log, agentIndex, auditStore),
					oapispec.Tags("Agents"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Agent name"),
					oapispec.Body(&handlers.SetAgentVisibilityRequest{}),
					oapispec.Response(200, &handlers.SetVisibilityResponse{}),
				)
				api.POST(agentWriteRoutes, "/transfer", "Transfer agent to another account", handlers.TransferAgent(log, agentIndex, accountStore, avatarStore, auditStore, deploymentStore, k8sCache, queue, resourceLifecycle, roleProjector),
					oapispec.Tags("Agents"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Source account name"),
					oapispec.PathParam("name", "Agent name"),
					oapispec.Body(&handlers.TransferAgentRequest{}),
					oapispec.Response(200, &handlers.TransferAgentResponse{}),
					oapispec.Response(409, &handlers.ErrorResponse{}),
				)

				if avatarStore != nil {
					api.POST(agentWriteRoutes, "/avatar", "Upload blueprint avatar",
						handlers.UploadBlueprintAvatar(log, avatarStore, agentIndex, auditStore),
						oapispec.Tags("Avatars"),
						oapispec.BearerAuth(),
						oapispec.PathParam("account", "Account name"),
						oapispec.PathParam("name", "Agent name"),
						oapispec.Response(200, &handlers.AvatarResponse{}),
					)
					api.DELETE(agentWriteRoutes, "/avatar", "Reset blueprint avatar",
						handlers.ResetBlueprintAvatar(log, avatarStore, agentIndex, auditStore),
						oapispec.Tags("Avatars"),
						oapispec.BearerAuth(),
						oapispec.PathParam("account", "Account name"),
						oapispec.PathParam("name", "Agent name"),
						oapispec.Response(200, &handlers.AvatarResponse{}),
					)
				}

				if readmeAssetStore != nil {
					api.POST(agentWriteRoutes, "/readme-assets", "Upload AGENT.md images",
						handlers.UploadReadmeAssets(log, readmeAssetStore),
						oapispec.Tags("Agents"),
						oapispec.BearerAuth(),
						oapispec.PathParam("account", "Account name"),
						oapispec.PathParam("name", "Agent name"),
						oapispec.Response(200, &handlers.ReadmeAssetsResponse{}),
					)
				}
			}

			// clusterStore validates optional `target.cluster_id` on deploy specs.
			api.POST(protected, "/deploy", "Deploy an agent", handlers.DeployAgent(log, agentIndex, accountStore, cfg, deps.Vault, deploymentStore, accountVarsStore, clusterStore, k8sReg, ent, quotaChecker, queue, avatarStore, resourceLifecycle, auditStore, ksStore, authzStore, imagePreflighter, tmplCache, k8sCache, roleProjector),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.Desc("Accepts a fulfilled deployment spec (YAML or JSON) and schedules async deployment to Kubernetes."),
				oapispec.Response(202, &handlers.DeployResponseAlias{}),
			)
			api.POST(protected, "/deploy/validate", "Validate a deployment spec", handlers.ValidateDeployment(log, agentIndex, accountStore, cfg, deps.Vault),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.Desc("Validates a fulfilled deployment spec without applying it."),
				oapispec.Response(200, &handlers.ValidateDeploymentResponse{}),
			)
			api.POST(protected, "/undeploy", "Undeploy an agent", handlers.UndeployAgent(log, agentIndex, accountStore, cfg, deploymentStore, queue, db, auditStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.Body(&deployment.UndeployRequest{}),
				oapispec.Response(202, &handlers.UndeployResponseAlias{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/status", "Get deployment status", handlers.GetDeploymentStatus(log, accountStore, k8sReg, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, &handlers.GetDeploymentStatusResponse{}),
			)
			deploymentRoutes.ObservedPATCH(authz.ActionDeploymentEdit, "/deployments/:id", "Update deployment display name", handlers.UpdateDeploymentDisplayName(log, accountStore, deploymentStore, auditStore, k8sCache),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
			)
			deploymentRoutes.ObservedPOST(authz.ActionDeploymentOperate, "/deployments/:id/wakeup", "Wake up a scaled-down deployment", handlers.WakeUpDeployment(log, accountStore, deploymentStore, queue, auditStore, ent),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(202, nil),
			)
			deploymentRoutes.ObservedPOST(authz.ActionDeploymentOperate, "/deployments/:id/stop", "Stop a running deployment", handlers.StopDeployment(log, accountStore, k8sReg, deploymentStore, auditStore, k8sCache),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(202, nil),
			)
			deploymentRoutes.ObservedPOST(authz.ActionDeploymentOperate, "/deployments/:id/cancel", "Cancel an in-progress deployment", handlers.CancelDeployment(log, accountStore, deploymentStore, auditStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(202, nil),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/watchers", "List deployment alert watchers", handlers.ListDeploymentWatchers(log, accountStore, deploymentStore, watcherStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, nil),
			)
			// Alert subscriptions are self-service preferences, not deployment edits:
			// anyone who can read a deployment may watch or unwatch it for themselves.
			deploymentRoutes.ObservedPOST(authz.ActionDeploymentRead, "/deployments/:id/watchers/me", "Watch a deployment's alerts", handlers.WatchDeployment(log, accountStore, deploymentStore, watcherStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, nil),
			)
			deploymentRoutes.ObservedDELETE(authz.ActionDeploymentRead, "/deployments/:id/watchers/me", "Stop watching a deployment's alerts", handlers.UnwatchDeployment(log, accountStore, deploymentStore, watcherStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, nil),
			)
			deploymentRoutes.ObservedPOST(authz.ActionDeploymentOperate, "/deployments/:id/rollback", "Rollback to a previous revision", handlers.RollbackDeployment(log, accountStore, deploymentStore, queue, auditStore, k8sCache, ent),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(202, nil),
			)
			deploymentRoutes.ObservedPOST(authz.ActionDeploymentOperate, "/deployments/:id/restart", "Restart all pods in a deployment", handlers.RestartDeployment(log, accountStore, cfg, k8sReg, deploymentStore, auditStore, ent),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.RestartDeploymentResponse{}),
			)
			deploymentRoutes.ObservedPOST(authz.ActionDeploymentOperate, "/deployments/:id/pods/:pod/restart", "Restart a pod", handlers.RestartPod(log, accountStore, cfg, k8sReg, deploymentStore, auditStore, ent),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.PathParam("pod", "Pod name"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.RestartPodResponse{}),
			)
			deploymentRoutes.ObservedPOST(authz.ActionDeploymentOperate, "/deployments/:id/ingestion/:ingestion/trigger", "Trigger an ingestion job", handlers.TriggerIngestion(log, agentIndex, accountStore, k8sReg, deploymentStore, cfg, auditStore, ent),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.PathParam("ingestion", "Ingestion name"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.TriggerIngestionResponse{}),
			)

			if avatarStore != nil {
				deploymentRoutes.ObservedPOST(authz.ActionDeploymentEdit, "/deployments/:id/avatar", "Upload deployment avatar",
					handlers.UploadDeploymentAvatar(log, accountStore, deploymentStore, avatarStore, auditStore, k8sCache),
					oapispec.Tags("Avatars"),
					oapispec.BearerAuth(),
					oapispec.PathParam("id", "Deployment ID"),
					oapispec.Response(200, &handlers.AvatarResponse{}),
				)
				deploymentRoutes.ObservedDELETE(authz.ActionDeploymentEdit, "/deployments/:id/avatar", "Reset deployment avatar",
					handlers.ResetDeploymentAvatar(log, accountStore, deploymentStore, avatarStore, auditStore, k8sCache),
					oapispec.Tags("Avatars"),
					oapispec.BearerAuth(),
					oapispec.PathParam("id", "Deployment ID"),
					oapispec.Response(200, &handlers.AvatarResponse{}),
				)
			}

			// Deployment read (status, logs, observability)
			api.GET(protected, "/agents/:account/:name/deployment", "Get active deployment spec", handlers.GetActiveDeploymentSpec(log, accountStore, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("name", "Agent name"),
				oapispec.Response(200, &handlers.ActiveDeploymentResponse{}),
				oapispec.Response(404, &handlers.ErrorResponse{}),
			)
			api.GET(protected, "/agents/:account/:name/deployment/history", "Get deployment history", handlers.GetDeploymentHistory(log, accountStore, deploymentStore, deploymentDiscovery),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("name", "Agent name"),
				oapispec.Response(200, &handlers.DeploymentHistoryResponse{}),
			)
			api.GET(protected, "/agents/:account/:name/evaluation-set", "Get the agent's evaluation set", handlers.GetAgentEvaluationSet(log, accountStore, agentIndex),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("name", "Agent name"),
				oapispec.Response(200, &handlers.EvaluationSetResponse{}),
			)
			api.GET(protected, "/deployments/count", "Count deployments", handlers.CountDeployments(log, accountStore, deploymentStore, deploymentDiscovery),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, nil),
			)
			api.GET(protected, "/deployments/summary", "List deployment summaries", handlers.ListDeploymentsSummary(log, accountStore, deploymentStore, avatarStore, deploymentDiscovery),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.Response(200, &handlers.DeploymentsSummaryResponse{}),
			)
			api.GET(protected, "/deployments", "List deployments", handlers.ListDeployments(log, accountStore, deploymentStore, agentIndex, avatarStore, auditStore, k8sCache, deploymentDiscovery),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.ListDeploymentsResponse{}),
			)
			// The detail record includes deployed env names and non-secret values. Secret
			// values are redacted, but the route remains configuration-sensitive.
			deploymentRoutes.ObservedGET(authz.ActionDeploymentRead, "/deployments/:id", "Get deployment", handlers.GetDeployment(log, accountStore, cfg, deps.Vault, deploymentStore, agentIndex, avatarStore, auditStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.GetDeploymentDetailResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/capabilities", "Get effective deployment capabilities", handlers.GetDeploymentCapabilities(log, deploymentCapabilities),
				oapispec.Tags("Deployments", "Authorization"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, &handlers.DeploymentCapabilitiesResponse{}),
				oapispec.Response(404, &handlers.ErrorResponse{}),
				oapispec.Response(503, &handlers.ErrorResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentManageAccess, "/deployments/:id/access", "List deployment access", handlers.ListDeploymentAccess(log, deploymentAccess),
				oapispec.Tags("Deployments", "Authorization"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, &handlers.DeploymentAccessResponse{}),
				oapispec.Response(404, &handlers.ErrorResponse{}),
				oapispec.Response(503, &handlers.ErrorResponse{}),
			)
			deploymentRoutes.DeferredPUT(authz.ActionDeploymentManageAccess, "/deployments/:id/access", "Set deployment access", handlers.SetDeploymentAccess(log, deploymentAccess, queue, auditStore),
				oapispec.Tags("Deployments", "Authorization"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Body(&handlers.SetDeploymentAccessRequest{}),
				oapispec.Response(202, &handlers.DeploymentAccessMutationResponse{}),
				oapispec.Response(400, &handlers.ErrorResponse{}),
				oapispec.Response(404, &handlers.ErrorResponse{}),
				oapispec.Response(409, &handlers.ErrorResponse{}),
				oapispec.Response(503, &handlers.ErrorResponse{}),
			)
			deploymentRoutes.DeferredDELETE(authz.ActionDeploymentManageAccess, "/deployments/:id/access/:subject_type/:subject_id", "Remove deployment access", handlers.RemoveDeploymentAccess(log, deploymentAccess, queue, auditStore),
				oapispec.Tags("Deployments", "Authorization"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("subject_type", "Subject type: member or group"),
				oapispec.PathParam("subject_id", "Astro user ID or WorkOS group ID"),
				oapispec.Response(202, &handlers.DeploymentAccessMutationResponse{}),
				oapispec.Response(400, &handlers.ErrorResponse{}),
				oapispec.Response(404, &handlers.ErrorResponse{}),
				oapispec.Response(409, &handlers.ErrorResponse{}),
				oapispec.Response(503, &handlers.ErrorResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/runtime", "Get deployment runtime", handlers.GetDeploymentRuntime(log, accountStore, cfg, deploymentStore, promClient, k8sReg),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.GetDeploymentRuntimeResponse{}),
			)
			// Messaging proxy — pure in-transit forward to the deployment's
			// messaging sidecar (send + SSE). No chat content is persisted here.
			messagingProxy := handlers.ProxyDeploymentMessaging(log, accountStore, deploymentStore, k8sReg, cfg, ent)
			deploymentRoutes.DataPlaneAny("/deployments/:id/messaging/*proxyPath", messagingProxy)
			// Chat API — authenticates the session and forwards to the sidecar,
			// which owns chat persistence (deployment-local SQLite on the agent's
			// shared persistent disk). astro-server stores no chat metadata or
			// message bodies.
			deploymentRoutes.DataPlaneGET("/deployments/:id/chat/conversations", "List deployment chat conversations",
				handlers.ListDeploymentChatConversations(log, cfg, k8sReg, accountStore, deploymentStore, ent),
				oapispec.Tags("Chat"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, &handlers.ListChatConversationsResponse{}),
			)
			deploymentRoutes.DataPlaneGET("/deployments/:id/chat/conversations/:conversationId", "Get deployment chat conversation",
				handlers.GetDeploymentChatConversation(log, cfg, k8sReg, accountStore, deploymentStore, ent),
				oapispec.Tags("Chat"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("conversationId", "Conversation ID"),
				oapispec.QueryParam("limit", "Max messages to return (tail or older page; max 1000)", false),
				oapispec.QueryParam("before_seq", "Return messages older than this seq (requires limit)", false),
				oapispec.Response(200, &handlers.GetChatConversationResponse{}),
			)
			deploymentRoutes.DataPlanePUT("/deployments/:id/chat/conversations/:conversationId/title", "Rename deployment chat conversation",
				handlers.SetDeploymentChatConversationTitle(log, cfg, k8sReg, accountStore, deploymentStore, ent),
				oapispec.Tags("Chat"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("conversationId", "Conversation ID"),
			)
			deploymentRoutes.DataPlaneDELETE("/deployments/:id/chat/conversations/:conversationId", "Delete deployment chat conversation",
				handlers.DeleteDeploymentChatConversation(log, cfg, k8sReg, accountStore, deploymentStore, ent),
				oapispec.Tags("Chat"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("conversationId", "Conversation ID"),
			)
			// Files API — authenticates the session and forwards to the sidecar,
			// which owns file storage (per-deployment persistent disk). Content
			// endpoints stream bytes and preserve upstream redirects so a future
			// presigned-object store needs no client change. astro-server stores
			// no file bytes or metadata.
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/files", "List deployment files",
				handlers.ListDeploymentFiles(log, cfg, k8sReg, accountStore, deploymentStore),
				oapispec.Tags("Files"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, &handlers.ListDeploymentFilesResponse{}),
			)
			deploymentRoutes.ObservedPOST(authz.ActionDeploymentEdit, "/deployments/:id/files", "Create a deployment file",
				handlers.CreateDeploymentFile(log, cfg, k8sReg, accountStore, deploymentStore),
				oapispec.Tags("Files"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Desc("Reserves an opaque file key and returns an upload descriptor; does not carry the file bytes."),
				oapispec.Response(200, &handlers.CreateDeploymentFileResponse{}),
			)
			// Registered before the :fileKey route so the static "usage" segment
			// resolves to this handler, not a file key lookup.
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/files/usage", "Get deployment storage usage",
				handlers.GetDeploymentStorageUsage(log, cfg, k8sReg, accountStore, deploymentStore),
				oapispec.Tags("Files"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Desc("Capacity of the volume backing the deployment's file store, for storage-full warnings."),
				oapispec.Response(200, &handlers.DeploymentStorageUsageResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/files/:fileKey", "Get deployment file metadata",
				handlers.GetDeploymentFile(log, cfg, k8sReg, accountStore, deploymentStore),
				oapispec.Tags("Files"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("fileKey", "File key"),
				oapispec.Response(200, &handlers.DeploymentFileMetaResponse{}),
			)
			deploymentRoutes.ObservedDELETE(authz.ActionDeploymentEdit, "/deployments/:id/files/:fileKey", "Delete a deployment file",
				handlers.DeleteDeploymentFile(log, cfg, k8sReg, accountStore, deploymentStore),
				oapispec.Tags("Files"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("fileKey", "File key"),
			)
			deploymentRoutes.ObservedPUT(authz.ActionDeploymentEdit, "/deployments/:id/files/:fileKey/content", "Upload deployment file content",
				handlers.UploadDeploymentFileContent(log, cfg, k8sReg, accountStore, deploymentStore),
				oapispec.Tags("Files"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("fileKey", "File key"),
				oapispec.Desc("Streams the file bytes to the reserved key (server-received upload path)."),
				oapispec.Response(200, &handlers.DeploymentFileMetaResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/files/:fileKey/content", "Download deployment file content",
				handlers.DownloadDeploymentFileContent(log, cfg, k8sReg, accountStore, deploymentStore),
				oapispec.Tags("Files"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("fileKey", "File key"),
				oapispec.Desc("Streams the file bytes, or redirects (3xx) to a direct object URL when the store supports it."),
				oapispec.Response(200, nil),
			)
			// Authorization is configured exclusively through `interfaces.auth`
			// in the deployment spec — no imperative endpoints here. The only
			// authorization endpoint is the messaging-facing
			// /deployments/authorize wired below behind RequireDeployToken.

			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/logs", "Get deployment logs", handlers.GetDeploymentLogs(log, accountStore, cfg, k8sReg, deploymentStore, lokiClient),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.QueryParam("pod", "Pod name", true),
				oapispec.QueryParam("container", "Container name", false),
				oapispec.QueryParam("tailLines", "Number of log lines (default 200)", false),
				oapispec.Desc("Returns raw log text (text/plain) for a pod in the deployment namespace."),
				oapispec.Response(200, nil),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/logs/stream", "Stream deployment logs (SSE)", handlers.StreamDeploymentLogs(log, accountStore, k8sReg, deploymentStore, lokiClient),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("workload", "Workload name", false),
				oapispec.QueryParam("container", "Container name", false),
				oapispec.QueryParam("pod", "Pod name", false),
				oapispec.Desc("Streams log lines as Server-Sent Events. Uses Loki when available, falls back to K8s pod logs. WRITE_TIMEOUT must be 0."),
				oapispec.Response(200, nil),
				oapispec.Response(501, &handlers.ErrorResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/events", "Get deployment K8s events", handlers.GetDeploymentEvents(log, accountStore, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, &handlers.DeploymentEventsResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/alerts", "Get deployment observation alerts and state", handlers.GetDeploymentAlerts(log, accountStore, deploymentStore, alertStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, &handlers.DeploymentAlertsResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/configmap/:cmname", "Get ConfigMap data", handlers.GetConfigMapData(log, accountStore, cfg, k8sReg, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.PathParam("cmname", "ConfigMap name"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.ConfigMapDataResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/secret/:secretname/keys", "Get Secret key names", handlers.GetSecretKeys(log, accountStore, cfg, k8sReg, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.PathParam("secretname", "Secret name"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.SecretKeysResponse{}),
			)
			// Observability endpoints (deployment-scoped, backed by Langfuse)
			langfuseStore := langfuse.NewStore(db)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/observability/metrics", "Get deployment metrics", handlers.GetLangfuseMetrics(log, cfg, accountStore, deploymentStore, langfuseStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("start_time", "Start time (RFC3339)", false),
				oapispec.QueryParam("end_time", "End time (RFC3339)", false),
				oapispec.Response(200, &handlers.ObservabilityMetricsResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/observability/summary", "Get deployment summary", handlers.GetLangfuseSummary(log, cfg, accountStore, deploymentStore, langfuseStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("start_time", "Start time (RFC3339)", false),
				oapispec.QueryParam("end_time", "End time (RFC3339)", false),
				oapispec.Response(200, &handlers.ObservabilitySummaryResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/observability/traces", "Get deployment traces", handlers.GetLangfuseTraces(log, cfg, accountStore, deploymentStore, langfuseStore, slackIdentityStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("start_time", "Start time (RFC3339)", false),
				oapispec.QueryParam("end_time", "End time (RFC3339)", false),
				oapispec.QueryParam("limit", "Page size (default 50)", false),
				oapispec.QueryParam("offset", "Pagination offset (default 0)", false),
				oapispec.Response(200, &handlers.ObservabilityTracesResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/observability/trace-users", "Get deployment trace users", handlers.GetLangfuseTraceUsers(log, cfg, accountStore, deploymentStore, langfuseStore, slackIdentityStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("start_time", "Start time (RFC3339)", false),
				oapispec.QueryParam("end_time", "End time (RFC3339)", false),
				oapispec.Response(200, &handlers.TraceUserFacetsResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/observability/traces/:traceId", "Get a single trace with its observations", handlers.GetLangfuseTraceDetail(log, cfg, accountStore, deploymentStore, langfuseStore, slackIdentityStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("traceId", "Trace ID"),
				oapispec.Response(200, &handlers.TraceDetailResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/observability/observations/:observationId", "Get a single observation with full input/output/metadata", handlers.GetLangfuseObservationDetail(log, cfg, accountStore, deploymentStore, langfuseStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("observationId", "Observation ID"),
				oapispec.Response(200, &handlers.Observation{}),
			)
			// Dataset endpoints (deployment-scoped, backed by Langfuse + eval_datasets)
			datasetStore := evaldatasetstore.NewStore(db)
			judgmentStore := judgmentstore.NewStore(db)
			evalItemStore := evalitemstore.NewStore(db)
			evalRunStore := evalrunstore.NewStore(db)
			evalDismissalStore := evaldismissalstore.NewStore(db)
			deploymentRoutes.ModelDeferredGET("/deployments/:id/dataset", "Get deployment dataset", handlers.GetEvalDataset(log, accountStore, deploymentStore, datasetStore, evalItemStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, nil),
			)
			deploymentRoutes.ModelDeferredGET("/deployments/:id/dataset/items", "List dataset items", handlers.GetEvalDatasetItems(log, cfg, accountStore, deploymentStore, datasetStore, langfuseStore, evalItemStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("page", "Unfiltered page number (default 1)", false),
				oapispec.QueryParam("limit", "Page size (default 50, max 100)", false),
				oapispec.Response(200, nil),
			)
			deploymentRoutes.ModelDeferredPOST("/deployments/:id/dataset/items", "Add a trace to the dataset", handlers.PostDatasetItem(log, cfg, accountStore, deploymentStore, datasetStore, langfuseStore, evalItemStore, evalRunStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Body(&handlers.DatasetItemRequest{}),
				oapispec.Response(201, &handlers.DatasetItemResponse{}),
			)
			deploymentRoutes.ModelDeferredPUT("/deployments/:id/dataset/items/:trace_id/evaluator-outputs", "Replace a dataset item's evaluator outputs", handlers.PutDatasetItemEvaluatorOutputs(log, accountStore, deploymentStore, datasetStore, evalItemStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("trace_id", "Trace ID"),
				oapispec.Body(&handlers.DatasetItemOutputsRequest{}),
				oapispec.Response(200, &handlers.DatasetItemOutputsResponse{}),
			)
			deploymentRoutes.ModelDeferredDELETE("/deployments/:id/dataset/items/:trace_id", "Remove a trace from the dataset", handlers.DeleteDatasetItem(log, cfg, accountStore, deploymentStore, datasetStore, langfuseStore, evalItemStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("trace_id", "Trace ID"),
				oapispec.Response(200, &handlers.DatasetItemResponse{}),
			)
			deploymentRoutes.ModelDeferredGET("/deployments/:id/dataset/download", "Download deployment dataset as zip", handlers.DownloadEvalDataset(log, cfg, accountStore, deploymentStore, datasetStore, langfuseStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, nil),
			)
			deploymentRoutes.ModelDeferredGET("/deployments/:id/dataset/review-queue", "Get dataset review queue", handlers.GetDatasetReviewQueue(log, cfg, accountStore, deploymentStore, datasetStore, langfuseStore, evalItemStore, evalRunStore, evalDismissalStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("limit", "Page size (default 50, max 100)", false),
				oapispec.QueryParam("evaluation", "Evaluation filter: evaluated or not_evaluated", false),
				oapispec.QueryParam("cursor", "Opaque continuation cursor returned by the previous page", false),
				oapispec.Response(200, &handlers.DatasetReviewQueueResponse{}),
			)
			deploymentRoutes.ModelDeferredGET("/deployments/:id/dataset/review-queue/:trace_id/evaluation", "Get a trace's evaluator results", handlers.GetDatasetTraceEvaluation(log, cfg, accountStore, deploymentStore, datasetStore, langfuseStore, evalRunStore, slackIdentityStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("trace_id", "Trace ID"),
				oapispec.Response(200, &handlers.DatasetTraceEvaluationResponse{}),
			)
			deploymentRoutes.ModelDeferredPOST("/deployments/:id/dataset/review-queue/:trace_id/dismiss", "Dismiss a trace from the review queue", handlers.PostReviewQueueDismissal(log, accountStore, deploymentStore, datasetStore, evalDismissalStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("trace_id", "Trace ID"),
				oapispec.Response(200, &handlers.ReviewQueueDismissalResponse{}),
			)
			deploymentRoutes.ModelDeferredDELETE("/deployments/:id/dataset/review-queue/:trace_id/dismiss", "Restore a dismissed trace to the review queue", handlers.DeleteReviewQueueDismissal(log, accountStore, deploymentStore, datasetStore, evalDismissalStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("trace_id", "Trace ID"),
				oapispec.Response(200, &handlers.ReviewQueueDismissalResponse{}),
			)
			deploymentRoutes.ModelDeferredGET("/deployments/:id/dataset/evaluations/status", "Get dataset evaluation status", handlers.GetDatasetEvaluationStatus(log, accountStore, deploymentStore, datasetStore, evalRunStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, &handlers.DatasetEvaluationStatusResponse{}),
			)
			deploymentRoutes.ModelDeferredPOST("/deployments/:id/dataset/evaluations", "Queue dataset evaluations", handlers.PostDatasetEvaluations(log, cfg, accountStore, deploymentStore, datasetStore, langfuseStore, evalItemStore, evalRunStore, evalDismissalStore, queue, ent),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(202, &handlers.DatasetEvaluationsResponse{}),
				oapispec.Response(500, &handlers.DatasetEvaluationsResponse{}),
			)
			deploymentRoutes.ModelDeferredPOST("/deployments/:id/dataset/judgments", "Submit dataset judgment", handlers.PostDatasetJudgment(log, cfg, accountStore, deploymentStore, datasetStore, langfuseStore, judgmentStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Body(&handlers.DatasetJudgmentRequest{}),
				oapispec.Response(201, &handlers.DatasetJudgmentResponse{}),
			)
			deploymentRoutes.ModelDeferredPATCH("/deployments/:id/dataset/judgments/:trace_id", "Change dataset judgment", handlers.PatchDatasetJudgment(log, cfg, accountStore, deploymentStore, datasetStore, langfuseStore, judgmentStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("trace_id", "Trace ID"),
				oapispec.Body(&handlers.DatasetJudgmentRequest{}),
				oapispec.Response(200, &handlers.DatasetJudgmentResponse{}),
			)
			deploymentRoutes.ModelDeferredPUT("/deployments/:id/dataset/judgments/:trace_id/criteria", "Replace dataset judgment criteria", handlers.PutDatasetJudgmentCriteria(log, cfg, accountStore, deploymentStore, datasetStore, langfuseStore, judgmentStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("trace_id", "Trace ID"),
				oapispec.Body(&handlers.DatasetJudgmentCriteriaRequest{}),
				oapispec.Response(200, &handlers.DatasetJudgmentCriteriaResponse{}),
			)
			deploymentRoutes.ModelDeferredDELETE("/deployments/:id/dataset/judgments/:trace_id", "Undo dataset judgment", handlers.DeleteDatasetJudgment(log, cfg, accountStore, deploymentStore, datasetStore, langfuseStore, judgmentStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("trace_id", "Trace ID"),
				oapispec.Response(200, &handlers.DatasetJudgmentResponse{}),
			)
			// Account-scoped observability (aggregates across all account deployments)
			api.GET(protected, "/accounts/:account/observability/summary", "Get account observability summary", handlers.GetAccountLangfuseSummary(log, cfg, accountStore, deploymentStore, langfuseStore, slackIdentityStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.QueryParam("from", "Period start (RFC3339)", false),
				oapispec.QueryParam("to", "Period end (RFC3339)", false),
				oapispec.QueryParam("group_by", "Set to 'user' to include cost_over_time_by_user", false),
				oapispec.Response(200, &handlers.AccountObservabilitySummaryResponse{}),
			)
			api.GET(protected, "/accounts/:account/observability/deployments-summary", "Get per-deployment observability summary", handlers.GetAccountDeploymentsSummary(log, cfg, accountStore, deploymentStore, langfuseStore, slackIdentityStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.QueryParam("from", "Period start (RFC3339)", false),
				oapispec.QueryParam("to", "Period end (RFC3339)", false),
				oapispec.Response(200, &handlers.AccountDeploymentsSummaryResponse{}),
			)
			api.GET(protected, "/accounts/:account/observability/users-summary", "Get per-user observability summary", handlers.GetAccountUsersSummary(log, cfg, accountStore, deploymentStore, langfuseStore, slackIdentityStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.QueryParam("from", "Period start (RFC3339)", false),
				oapispec.QueryParam("to", "Period end (RFC3339)", false),
				oapispec.Response(200, &handlers.AccountUsersSummaryResponse{}),
			)
			// Membership-gated: the stream names every agent in the account.
			accountEvents := protected.Group("/accounts/:account")
			accountEvents.Use(middleware.ResolveAccount(accountStore), middleware.RequireAccountMember(accountStore))
			api.GET(accountEvents, "/events", "Stream account agent events (SSE)", handlers.StreamAccountEvents(log, deps.Clients.Events, deps.Clients.EventStore),
				oapispec.Tags("Accounts"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.Desc("Streams agent build changes as Server-Sent Events. Each event names the agent that changed; clients refetch rather than read state off the event."),
				oapispec.Response(200, nil),
			)
			api.GET(protected, "/accounts/:account/insights", "Get account Insights page model", handlers.GetAccountInsights(log, accountStore, deploymentStore, slackIdentityStore, insightsrollup.NewStore(db), k8sCache, insightsOrgRoles, classificationExperiment),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.QueryParam("q", "Search query for agent and people table rows", false),
				oapispec.QueryParam("agents_limit", "Maximum agent table rows to return", false),
				oapispec.QueryParam("agents_offset", "Agent table row offset", false),
				oapispec.QueryParam("agents_sort", "Agent table sort key", false),
				oapispec.QueryParam("agents_direction", "Agent table sort direction: asc or desc", false),
				oapispec.QueryParam("people_limit", "Maximum people table rows to return", false),
				oapispec.QueryParam("people_offset", "People table row offset", false),
				oapispec.QueryParam("people_sort", "People table sort key", false),
				oapispec.QueryParam("people_direction", "People table sort direction: asc or desc", false),
				oapispec.QueryParam("skip_ranges", "Set to true to omit chart range data for table-only refreshes", false),
				oapispec.QueryParam("hide_sources", "Comma-separated source keys (or 'agents') to exclude", false),
				oapispec.QueryParam("days", "Range the tables cover: 7, 14, 30 or 90; omitted means account-wide", false),
				oapispec.Response(200, &handlers.InsightsResponse{}),
			)
			api.GET(protected, "/accounts/:account/insights/sources/:source", "Get a dev-tool source's classification breakdown", handlers.GetAccountInsightsSource(log, accountStore, classification.NewStore(db), insightsOrgRoles, classificationExperiment),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("source", "Dev-tool source key (e.g. claude-code)"),
				oapispec.QueryParam("day", "Narrow the per-developer breakdown to one UTC day (YYYY-MM-DD)", false),
				oapispec.Response(200, &handlers.InsightsSourceResponse{}),
			)

			api.GET(accountMember, "/observability/deployment-summaries", "Get bulk deployment observability summaries", handlers.GetLangfuseSummaries(log, deploymentStore, k8sCache),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.Response(200, &handlers.DeploymentSummariesResponse{}),
			)

			// Network endpoints (deployment-scoped, backed by Beyla eBPF metrics in Prometheus)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/network/summary", "Get deployment network summary", handlers.GetNetworkSummary(log, accountStore, deploymentStore, k8sReg, promClient),
				oapispec.Tags("Network"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("start_time", "Start time (RFC3339); defaults to one hour ago", false),
				oapispec.QueryParam("end_time", "End time (RFC3339); defaults to now", false),
				oapispec.Response(200, &handlers.NetworkSummaryResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/network/flows", "Get top network peers for a deployment", handlers.GetNetworkFlows(log, accountStore, deploymentStore, k8sReg, promClient),
				oapispec.Tags("Network"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("direction", "inbound | outbound | database", true),
				oapispec.QueryParam("start_time", "Start time (RFC3339); defaults to one hour ago", false),
				oapispec.QueryParam("end_time", "End time (RFC3339); defaults to now", false),
				oapispec.QueryParam("limit", "Max rows (default 50, max 200)", false),
				oapispec.QueryParam("sort", "requests | latency_p95 | errors (default requests)", false),
				oapispec.Response(200, &handlers.NetworkFlowsResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/network/timeseries", "Get bucketed network metrics for a deployment", handlers.GetNetworkTimeseries(log, accountStore, deploymentStore, k8sReg, promClient),
				oapispec.Tags("Network"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("direction", "inbound | outbound | database", true),
				oapispec.QueryParam("metric", "rate | errors | latency_p95 | bytes", true),
				oapispec.QueryParam("start_time", "Start time (RFC3339); defaults to one hour ago", false),
				oapispec.QueryParam("end_time", "End time (RFC3339); defaults to now", false),
				oapispec.QueryParam("step", "Bucket size as a Go duration; default 30s, minimum 15s", false),
				oapispec.QueryParam("group_by", "peer | status_class — capped at 8 series for peer", false),
				oapispec.Response(200, &handlers.NetworkTimeseriesResponse{}),
			)
			deploymentRoutes.DeferredGET(authz.ActionDeploymentRead, "/deployments/:id/pods/:pod/metrics", "Get CPU, memory and storage time series for a pod", handlers.GetWorkloadMetrics(log, accountStore, deploymentStore, promClient, k8sReg),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("pod", "Pod name (matches the cAdvisor `pod` label)"),
				oapispec.QueryParam("range", "Window preset: 1h, 6h, 24h, or 7d (default 1h)", false),
				oapispec.Response(200, &handlers.WorkloadMetricsResponse{}),
			)
		}

		// GitHub connection routes
		callbackBase := cfg.GitHub.CallbackURL
		if callbackBase == "" {
			callbackBase = cfg.Auth.FrontendURL
		}
		githubCfg := handlers.GitHubHandlerConfig{
			WebhookBaseURL: callbackBase,
			FrontendURL:    cfg.Auth.FrontendURL,
		}
		githubRoutes := protected.Group("/agents/:account/:name")
		githubRoutes.Use(middleware.ResolveAccount(accountStore))
		githubRoutes.Use(middleware.RequireAccountMember(accountStore))
		{
			api.POST(githubRoutes, "/github/link", "Link a GitHub repo to an agent",
				handlers.GitHubLink(log, pipesClient, ghStore, webhookStore, githubCfg),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
				oapispec.Body(&handlers.GitHubLinkRequest{}),
			)
			api.DELETE(githubRoutes, "/github", "Disconnect GitHub repo from agent",
				handlers.GitHubDisconnect(log, pipesClient, ghStore, webhookStore),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
			)
			api.GET(githubRoutes, "/github", "Get GitHub connection status and builds",
				handlers.GitHubStatus(log, ghStore, pipesClient, k8sCache),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
			)
			api.POST(githubRoutes, "/github/rebuild", "Trigger a manual rebuild",
				handlers.GitHubRebuild(log, pipesClient, ghStore, queue),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
			)
			api.GET(githubRoutes, "/github/builds/:build_id/logs", "Get build job logs",
				handlers.GitHubBuildLogs(log, ghStore, k8sClient, cfg.GitHub.BuildNamespace),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
			)
		}

		// Account-level GitHub connection routes (blueprint-agnostic, for the new blueprint wizard)
		accountGitHubRoutes := protected.Group("/accounts/:account")
		accountGitHubRoutes.Use(middleware.ResolveAccount(accountStore))
		accountGitHubRoutes.Use(middleware.RequireAccountMember(accountStore))
		{
			api.GET(accountGitHubRoutes, "/github", "Get GitHub connection status for account",
				handlers.GitHubAccountStatus(log, pipesClient),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.DELETE(accountGitHubRoutes, "/github", "Disconnect GitHub from account",
				handlers.GitHubAccountDisconnect(log, pipesClient, ghStore, webhookStore, k8sCache),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.POST(accountGitHubRoutes, "/github/connect", "Start account-level GitHub OAuth",
				handlers.GitHubAccountConnect(log, pipesClient, githubCfg),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.GET(accountGitHubRoutes, "/github/repos", "List GitHub repos for account",
				handlers.GitHubAccountListRepos(log, pipesClient, k8sCache),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.GET(accountGitHubRoutes, "/github/scan", "Scan repo for astropods.yml",
				handlers.GitHubAccountScan(log, pipesClient),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.GET(accountGitHubRoutes, "/github/connections", "List repos already linked to agents under this account",
				handlers.GitHubAccountListConnections(log, ghStore),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.GET(accountGitHubRoutes, "/github/orgs", "List GitHub orgs the user has granted access to",
				handlers.GitHubAccountListOrgs(log, pipesClient),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.GET(accountGitHubRoutes, "/github/branches", "List branches for a GitHub repo",
				handlers.GitHubAccountListBranches(log, pipesClient),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.QueryParam("repo", "GitHub repo full name (owner/name)", true),
			)
			// Callback is a browser GET from the OAuth redirect — same auth middleware, no body
			api.GET(accountGitHubRoutes, "/github/callback", "Account-level GitHub OAuth callback",
				handlers.GitHubAccountCallback(log, pipesClient, githubCfg, k8sCache),
				oapispec.Tags("GitHub"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
		}

		// Account-scoped Slack identity routes. Identity link uses raw
		// slack OAuth (not WorkOS Pipes) because we need the user token
		// — Pipes only surfaces the bot token, whose auth.test resolves
		// to the bot user, not the human installer.
		//
		// SLACK_CALLBACK_URL is independent of GITHUB_CALLBACK_URL: in
		// production they're typically the same value, but in dev each
		// integration may need its own ngrok tunnel registered with a
		// different OAuth app.
		slackCallbackBase := cfg.Slack.CallbackURL
		if slackCallbackBase == "" {
			slackCallbackBase = cfg.Auth.FrontendURL
		}
		slackCfg := handlers.SlackHandlerConfig{
			ClientID:       cfg.Slack.ClientID,
			ClientSecret:   cfg.Slack.ClientSecret,
			WebhookBaseURL: slackCallbackBase,
			FrontendURL:    cfg.Auth.FrontendURL,
		}
		accountSlackRoutes := protected.Group("/accounts/:account")
		accountSlackRoutes.Use(middleware.ResolveAccount(accountStore))
		accountSlackRoutes.Use(middleware.RequireAccountMember(accountStore))
		{
			api.GET(accountSlackRoutes, "/slack", "Get Slack identity link status",
				handlers.SlackAccountStatus(log, slackIdentityStore),
				oapispec.Tags("Slack"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.DELETE(accountSlackRoutes, "/slack", "Disconnect Slack identity",
				handlers.SlackAccountDisconnect(log, slackIdentityStore),
				oapispec.Tags("Slack"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.POST(accountSlackRoutes, "/slack/connect", "Start raw Slack OAuth flow",
				handlers.SlackAccountConnect(log, slackCfg),
				oapispec.Tags("Slack"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			// Browser GET from slack.com after the user authorizes — same
			// auth middleware (the session cookie is still present), no body.
			api.GET(accountSlackRoutes, "/slack/callback", "Slack OAuth callback",
				handlers.SlackAccountCallback(log, slackIdentityStore, slackCfg),
				oapispec.Tags("Slack"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
		}

		// Account-scoped Supabase OAuth routes. Used to auto-import a user's
		// Supabase projects when connecting a Supabase (PostgreSQL) knowledge
		// store. OAuth is brokered by WorkOS Pipes as a custom provider — WorkOS
		// stores and refreshes the tokens; we only ever hold a short-lived access
		// token to call the Supabase API.
		supabaseCfg := handlers.SupabaseHandlerConfig{
			WebhookBaseURL: cfg.Auth.FrontendURL,
			FrontendURL:    cfg.Auth.FrontendURL,
		}
		accountSupabaseRoutes := protected.Group("/accounts/:account")
		accountSupabaseRoutes.Use(middleware.ResolveAccount(accountStore))
		accountSupabaseRoutes.Use(middleware.RequireAccountMember(accountStore))
		{
			api.POST(accountSupabaseRoutes, "/supabase/connect", "Start Supabase OAuth flow",
				handlers.SupabaseConnect(log, pipesClient, supabaseCfg),
				oapispec.Tags("Supabase"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.GET(accountSupabaseRoutes, "/supabase/status", "Get Supabase connection status",
				handlers.SupabaseStatus(log, pipesClient),
				oapispec.Tags("Supabase"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.GET(accountSupabaseRoutes, "/supabase/projects", "List Supabase projects",
				handlers.SupabaseListProjects(log, pipesClient),
				oapispec.Tags("Supabase"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.GET(accountSupabaseRoutes, "/supabase/projects/:ref/health", "Get Supabase project health",
				handlers.SupabaseProjectHealth(log, pipesClient),
				oapispec.Tags("Supabase"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			api.DELETE(accountSupabaseRoutes, "/supabase", "Disconnect Supabase",
				handlers.SupabaseDisconnect(log, pipesClient),
				oapispec.Tags("Supabase"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
			// WorkOS returns the browser here after OAuth; the handler confirms
			// the token and bounces to the frontend. Authenticated + account-scoped
			// like the GitHub callback (the session cookie rides the redirect).
			api.GET(accountSupabaseRoutes, "/supabase/callback", "Supabase OAuth callback",
				handlers.SupabaseCallback(log, pipesClient, supabaseCfg),
				oapispec.Tags("Supabase"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
			)
		}

		// GitHub webhook receiver (no auth — HMAC verified inside handler)
		router.POST("/webhooks/github", handlers.GitHubWebhook(log, ghStore, webhookStore, queue))

		// Billing webhook receivers (no auth — signatures verified inside the
		// handlers). Each verifies then enqueues a River job; account mapping and
		// cached-status recompute run in the worker. Metronome delivers
		// usage/alert lifecycle; Stripe delivers payment-collection lifecycle
		// (payment failure, 3DS, uncollectible, void) that Metronome does not relay.
		//
		// Each is registered only where its worker is (riverqueue.addWorkers), so
		// an unhandled event 404s rather than enqueueing a job nothing drains.
		// Metronome's resolves accounts by Metronome customer id; Stripe's
		// resolves by Stripe customer id, which the payment provider persists
		// whatever the billing backend is.
		if cfg.BillingBackend() == config.BillingBackendMetronome {
			router.POST("/webhooks/metronome", handlers.MetronomeWebhook(log, cfg.MetronomeWebhookSecret, queue))
		}
		if config.BillingBackendHasCustomers(cfg.BillingBackend()) {
			router.POST("/webhooks/stripe", handlers.StripeWebhook(log, cfg.StripeWebhookSecret, queue))
		}
	}

	if err := deploymentRouteCatalog.Validate(router.Routes()); err != nil {
		log.Error("authz: invalid deployment authorization routes", "error", err)
		os.Exit(1)
	}

}

// startAdminGRPCServer starts the admin gRPC server and returns it for graceful shutdown.
// Returns nil, nil if the port is empty (disabled).
func startAdminGRPCServer(
	log *logger.Logger,
	cfg *config.Config,
	adminSrv *admingrpc.Server,
) (*grpc.Server, error) {
	port := cfg.AdminGRPC.Port
	if port == "" {
		return nil, nil
	}

	// Build TLS credentials (nil = insecure, certs not configured)
	creds, err := admingrpc.ServerCredentials(admingrpc.TLSConfig{
		CertFile: cfg.AdminGRPC.CertFile,
		KeyFile:  cfg.AdminGRPC.KeyFile,
		CAFile:   cfg.AdminGRPC.CAFile,
	})
	if err != nil {
		return nil, fmt.Errorf("admin gRPC TLS: %w", err)
	}

	var opts []grpc.ServerOption
	if creds == nil {
		if !cfg.Deployment.IsLocal() {
			log.Warn("admin grpc: disabled, mTLS not configured (set ADMIN_GRPC_CERT_FILE, ADMIN_GRPC_KEY_FILE, ADMIN_GRPC_CA_FILE)")
			return nil, nil
		}
		log.Warn("admin grpc: starting without TLS, local dev only")
		opts = append(opts, grpc.Creds(insecure.NewCredentials()))
	} else {
		opts = append(opts, grpc.Creds(creds))
	}

	grpcSrv := grpc.NewServer(opts...)
	adminv1.RegisterAdminServiceServer(grpcSrv, adminSrv)

	// Bind only to loopback when running insecurely so the port is unreachable
	// from other hosts even if ENVIRONMENT=local is set on a routable interface.
	listenAddr := ":" + port
	if creds == nil {
		listenAddr = "127.0.0.1:" + port
	}
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("admin gRPC listen: %w", err)
	}

	go func() {
		log.Info("admin grpc: listening", "port", port)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Error("admin grpc: server error", "error", err)
		}
	}()

	return grpcSrv, nil
}

// newS3Client creates an S3 client using the shared AWS config, applying a custom
// endpoint when S3_ENDPOINT is set (e.g. MinIO for local dev).
func newS3Client(cfg *config.Config) (*s3.Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, err
	}
	opts := []func(*s3.Options){}
	if cfg.S3.Endpoint != "" {
		endpoint := cfg.S3.Endpoint
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = &endpoint
			o.UsePathStyle = cfg.S3.PathStyle
		})
	}
	return s3.NewFromConfig(awsCfg, opts...), nil
}
