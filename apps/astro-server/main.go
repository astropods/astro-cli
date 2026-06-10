package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	connectv1 "github.com/astropods/astro/packages/astro-proto/connect/v1"
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
	"github.com/astropods/astro/apps/astro-server/internal/accountvars"
	"github.com/astropods/astro/apps/astro-server/internal/admingrpc"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/authorizationstore"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/chatstore"
	"github.com/astropods/astro/apps/astro-server/internal/clusterstore"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/connectgrpc"
	"github.com/astropods/astro/apps/astro-server/internal/datasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/devicestore"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/githubwebhook"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/metricsstore"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	oapispec "github.com/astropods/astro/apps/astro-server/internal/openapi"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
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
	log.Info("Starting astro-server",
		"version", "1.0.0",
		"mode", cfg.Server.Mode,
		"run_mode", cfg.RunMode,
		"port", cfg.Server.Port,
	)

	// Open shared database connection
	db, err := sql.Open("postgres", cfg.Database.URL)
	if err != nil {
		log.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	if err := db.Ping(); err != nil {
		log.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	log.Info("Database connection established")

	// Initialize account store and agent index (needed by both API and worker)
	accountStore := account.NewAccountStore(db)
	agentIndex := agentindex.NewIndexWithDB(db)

	// Build a shared S3 client (respects S3_ENDPOINT for local MinIO / S3-compatible stores).
	sharedS3Client, sharedS3Err := newS3Client(cfg)
	if sharedS3Err != nil {
		log.Warn("Failed to initialize S3 client", "error", sharedS3Err)
	}

	// Initialize avatar store (S3 or local filesystem)
	var avatarStore *avatar.Store
	if cfg.Avatar.Enabled() {
		if cfg.Avatar.IsLocal() {
			backend := avatar.NewLocalBackend(cfg.Avatar.LocalDir)
			avatarStore = avatar.NewStore(backend, cfg.Avatar.AssetsURL)
			log.Info("Avatar store initialized (local)", "dir", cfg.Avatar.LocalDir)
		} else if sharedS3Client != nil {
			backend := avatar.NewS3Backend(sharedS3Client, cfg.Avatar.S3Bucket)
			avatarStore = avatar.NewStore(backend, cfg.Avatar.AssetsURL)
			log.Info("Avatar store initialized (S3)", "bucket", cfg.Avatar.S3Bucket)
		}
	}
	// Initialize WorkOS organization client
	var orgClient *org.Client
	var orgSync *org.Sync
	if cfg.Auth.WorkOSAPIKey != "" {
		orgClient = org.NewClient(cfg.Auth.WorkOSAPIKey)
		workosClient := auth.NewWorkOSClient(cfg.Auth.WorkOSAPIKey, cfg.Auth.WorkOSClientID, cfg.Auth.RedirectURI, cfg.Auth.FrontendURL)
		orgSync = org.NewSync(orgClient, accountStore, workosClient, db)
		log.Info("WorkOS organization client initialized")
	}

	// Initialize OpenMeter client (nil if OPENMETER_URL is empty)
	omClient := openmeter.NewClient(cfg.OpenMeterURL)
	if omClient != nil {
		log.Info("OpenMeter client initialized", "url", cfg.OpenMeterURL)

		// Validate that all required meters exist
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		missing, err := omClient.ValidateMeters(ctx)
		cancel()
		if err != nil {
			log.Error("Failed to validate OpenMeter meters", "error", err)
		} else if len(missing) > 0 {
			log.Error("OpenMeter is missing required meters", "missing", missing)
		} else {
			log.Info("All required OpenMeter meters verified", "meters", openmeter.RequiredMeters, "count", len(openmeter.RequiredMeters))
		}
	}

	// Entitlement enforcement (no-op when omClient is nil or enforce is false)
	ent := middleware.NewEntitlements(log, omClient, cfg.OpenMeterEnforce)

	// Initialize shared Redis client (nil when REDIS_URL is unset).
	// Pass this client to any feature that needs Redis; do not create additional clients.
	var redisClient *goredis.Client
	if cfg.RedisURL != "" {
		opts, redisErr := goredis.ParseURL(cfg.RedisURL)
		if redisErr != nil {
			log.Error("Failed to parse REDIS_URL, Redis features disabled", "error", redisErr)
		} else {
			redisClient = goredis.NewClient(opts)
			log.Info("Redis client initialized", "url", cfg.RedisURL)
		}
	}
	k8sCache := k8scache.New(redisClient)

	// Track components for graceful shutdown
	var httpSrv *http.Server
	var grpcServer *grpc.Server
	var fleetGRPCServer *grpc.Server
	var eventsCancel context.CancelFunc
	var probeHandler *handlers.ProbeHandler
	var adminSrv *admingrpc.Server
	var apiQueue *riverqueue.Queue

	// --- API mode: HTTP server + gRPC admin + gRPC connect ---
	if cfg.RunAPI() {
		httpSrv, grpcServer, fleetGRPCServer, probeHandler, adminSrv, apiQueue = runAPI(log, cfg, db, accountStore, agentIndex, orgClient, orgSync, omClient, ent, avatarStore, k8sCache)
	}

	// --- Worker mode: events consumer ---
	if cfg.RunWorker() {
		eventsCancel = runWorker(log, cfg, accountStore, agentIndex, db, omClient, orgClient, avatarStore, k8sCache, newImagePreflighter(cfg))
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
			log.Info("Worker health server listening", "address", httpSrv.Addr)
			if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("Worker health server failed", "error", err)
				os.Exit(1)
			}
		}()
	}

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Mark as not ready to stop receiving new traffic
	if probeHandler != nil {
		probeHandler.SetReady(false)
	}

	// Stop events consumer
	if eventsCancel != nil {
		eventsCancel()
	}

	// Graceful shutdown of gRPC servers
	if fleetGRPCServer != nil {
		fleetGRPCServer.GracefulStop()
	}
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
			log.Error("Server forced to shutdown", "error", err)
			os.Exit(1)
		}
	}

	log.Info("Server stopped gracefully")
}

// runAPI initializes and starts the HTTP API server, gRPC admin server, and Fleet gRPC server.
func runAPI(
	log *logger.Logger,
	cfg *config.Config,
	db *sql.DB,
	accountStore *account.AccountStore,
	agentIndex *agentindex.Index,
	orgClient *org.Client,
	orgSync *org.Sync,
	omClient *openmeter.Client,
	ent *middleware.Entitlements,
	avatarStore *avatar.Store,
	k8sCache k8scache.Cache,
) (*http.Server, *grpc.Server, *grpc.Server, *handlers.ProbeHandler, *admingrpc.Server, *riverqueue.Queue) {
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
			log.Error("Failed to set trusted proxies", "error", err)
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
	ksStore := knowledgestore.NewStore(db)
	log.Info("Agent index and stores initialized")

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
			log.Warn("source_account_id backfill failed",
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
			log.Info("source_account_id backfill complete",
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
			log.Warn("source_account_id stale rebind failed", "error", err)
		} else if rebind.Rebound > 0 {
			log.Info("source_account_id stale rebind complete", "rebound", rebind.Rebound)
		}
	}()

	// Initialize Kubernetes registry. The registry holds the primary
	// ClusterClient (built from env vars / kubeconfig) and is the seam
	// for per-deployment cluster_id resolution against the `clusters`
	// table. Handlers and workers without a per-deployment cluster_id
	// receive registry.Default(). See
	// docs/01-spec/multi-region-cluster-support-spec.md for the design.
	clientMode := k8s.ClientMode(cfg.Deployment.K8sClientMode)
	log.Info("Initializing Kubernetes registry", "mode", string(clientMode))
	registryCtx, registryCancel := context.WithTimeout(context.Background(), 30*time.Second)
	registry, registryErr := k8s.NewRegistry(registryCtx, clusterStore, k8s.RegistryConfig{
		Mode:             clientMode,
		Region:           cfg.Deployment.AWSRegion,
		KubeconfigPath:   cfg.Deployment.KubeconfigPath,
		KubeContext:      cfg.Deployment.KubeContext,
		EKSBootstrapName: cfg.Deployment.EKSClusterName,
		EKSBootstrapURL:  cfg.Deployment.K8sMasterURL,
	}, log)
	registryCancel()

	var k8sClient k8s.ClusterClient
	var k8sReg *k8s.Registry
	if registryErr != nil {
		log.Warn("Failed to initialize K8s registry", "error", registryErr)
		log.Warn("Kubernetes features will be unavailable")
	} else {
		k8sReg = registry
		k8sClient = registry.Default()
		if version, connErr := k8sClient.GetServerVersion(); connErr != nil {
			log.Warn("K8s client created but connection failed", "error", connErr)
			diag := k8sClient.DiagnoseConnection()
			for key, val := range diag {
				log.Debug("K8s diagnostic", key, val)
			}
		} else {
			log.Info("Kubernetes connection established",
				"mode", string(clientMode),
				"version", version,
			)
		}
	}

	// Resolve deployment log backend: explicit env var > auto-detect from LOKI_URL
	logBackend := cfg.DeploymentLogBackend
	if logBackend == "" {
		if cfg.LokiURL != "" {
			logBackend = "loki"
		} else {
			logBackend = "k8s"
		}
	}
	var lokiClient *loki.Client
	if logBackend == "loki" {
		if cfg.LokiURL == "" {
			log.Error("DEPLOYMENT_LOG_BACKEND=loki but LOKI_URL is not set")
		} else {
			lokiClient = loki.New(cfg.LokiURL)
			log.Info("Deployment log backend: loki", "url", cfg.LokiURL)
		}
	} else {
		log.Info("Deployment log backend: k8s (direct pod logs)")
	}

	// Initialize probe handler
	probeHandler := handlers.NewProbeHandler(log, agentIndex, k8sClient)

	// Create insert-only River queue for API (no workers, no periodic jobs — workers run in runWorker)
	rq, rqErr := riverqueue.NewInsertOnly(context.Background(), cfg.Database.URL, log)
	if rqErr != nil {
		log.Error("Failed to create River queue for API", "error", rqErr)
		os.Exit(1)
	}

	// Create audit log store
	auditStore := auditlog.NewStore(db)

	// Initialize GitHub connection store, webhook store, and WorkOS Pipes client.
	ghStore := githubconnection.New(db)
	webhookStore := githubwebhook.New(db)
	pipesClient := pipes.New(cfg.Auth.WorkOSAPIKey)
	slackIdentityStore := slackidentity.NewStore(db)

	// Initialize Prometheus query client (nil if PROMETHEUS_URL is empty)
	promClient := promquery.NewClient(cfg.PrometheusURL, cfg.Deployment.EKSClusterName)

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
		Ent:   ent,
		Probe: probeHandler,
		Stores: Stores{
			Account:      accountStore,
			Deployment:   deploymentStore,
			AccountVars:  accountVarsStore,
			Heart:        heartStore,
			AgentMetrics: agentMetricsStore,
			Cluster:      clusterStore,
			Audit:        auditStore,
			Avatar:       avatarStore,
			Knowledge:    ksStore,
			GH:           ghStore,
			Webhook:      webhookStore,
			SlackID:      slackIdentityStore,
		},
		Clients: Clients{
			AgentIndex: agentIndex,
			K8s:        k8sClient,
			Registry:   registry,
			Loki:       lokiClient,
			Org:        orgClient,
			OrgSync:    orgSync,
			OpenMeter:  omClient,
			Pipes:      pipesClient,
			Prom:       promClient,
			K8sCache:   k8sCache,
			Preflight:  imagePreflighter,
			Queue:      rq,
		},
	}
	setupRoutes(router, deps)

	// Start admin gRPC server
	adminSrv := admingrpc.New(log, deploymentStore, k8sClient, lokiClient, db, cfg.AdminGRPC.OpenMeterURL, cfg.Database.URL, rq, cfg.Deployment.IngressDomain, cfg.Deployment.IngestionIngressDomain, auditStore, clusterStore, k8sReg, k8sCache)
	grpcServer, grpcErr := startAdminGRPCServer(log, cfg, adminSrv)
	if grpcErr != nil {
		log.Error("Failed to start admin gRPC server", "error", grpcErr)
		os.Exit(1)
	}

	// Start Fleet gRPC server (QUIC, JWT auth)
	devStore := devicestore.New(db)
	workosClient := auth.NewWorkOSClient(cfg.Auth.WorkOSAPIKey, cfg.Auth.WorkOSClientID, cfg.Auth.RedirectURI, cfg.Auth.FrontendURL)
	jwksURL, _ := workosClient.GetJWKSURL()
	fleetJWTValidator := auth.NewJWTValidator(jwksURL, cfg.Auth.JWTIssuer, "")
	fleetServer, fleetSrv, fleetErr := startFleetGRPCServer(log, cfg, devStore, accountStore, fleetJWTValidator)
	if fleetErr != nil {
		log.Error("Failed to start Fleet gRPC server", "error", fleetErr)
		os.Exit(1)
	}

	// Wire gin router as HTTP handler for admin ProxyHTTP
	adminSrv.SetHTTPHandler(router)

	// Wire WorkOS client for admin GetAuthConfig and owner email resolution
	adminSrv.SetWorkOSClientID(cfg.Auth.WorkOSClientID)
	adminSrv.SetWorkOSClient(workosClient)

	// Wire Fleet gRPC server as command dispatcher for admin
	if fleetSrv != nil {
		adminSrv.SetCommandDispatcher(fleetSrv)
	}

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
		log.Info("Server listening", "address", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	return srv, grpcServer, fleetServer, probeHandler, adminSrv, rq
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
	omClient *openmeter.Client,
	orgClient *org.Client,
	avatarStore *avatar.Store,
	k8sCache k8scache.Cache,
	imagePreflighter *k8s.ImagePreflighter,
) context.CancelFunc {
	workerCtx, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel is returned to caller

	// Worker-side registry: same shape as the API-side one above. Both run
	// inside the same astro-server process when SERVER_MODE=all; with
	// SERVER_MODE=worker this is the only registry the process holds.
	clientMode := k8s.ClientMode(cfg.Deployment.K8sClientMode)
	initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
	clusterStore := clusterstore.New(db)
	registry, registryErr := k8s.NewRegistry(initCtx, clusterStore, k8s.RegistryConfig{
		Mode:             clientMode,
		Region:           cfg.Deployment.AWSRegion,
		KubeconfigPath:   cfg.Deployment.KubeconfigPath,
		KubeContext:      cfg.Deployment.KubeContext,
		EKSBootstrapName: cfg.Deployment.EKSClusterName,
		EKSBootstrapURL:  cfg.Deployment.K8sMasterURL,
	}, log)
	initCancel()

	var k8sReg *k8s.Registry
	if registryErr != nil {
		log.Warn("Worker: K8s registry unavailable, namespace scanner will skip K8s reconciliation", "error", registryErr)
	} else {
		k8sReg = registry
	}

	// Initialize Prometheus query client (nil if PROMETHEUS_URL is empty)
	promClient := promquery.NewClient(cfg.PrometheusURL, cfg.Deployment.EKSClusterName)
	if promClient != nil {
		log.Info("Prometheus query client initialized", "url", cfg.PrometheusURL)
	}

	// Initialize WorkOS client for background jobs (user lookups)
	var workosClient *auth.WorkOSClient
	if cfg.Auth.WorkOSAPIKey != "" {
		workosClient = auth.NewWorkOSClient(cfg.Auth.WorkOSAPIKey, cfg.Auth.WorkOSClientID, cfg.Auth.RedirectURI, cfg.Auth.FrontendURL)
	}

	// Initialize WorkOS Pipes client and GitHub connection store.
	pipesClient := pipes.New(cfg.Auth.WorkOSAPIKey)
	ghStore := githubconnection.New(db)

	// Stores the InsightsRefreshWorker's summary computer needs. Cheap to
	// construct here (Store wrappers just hold the *sql.DB).
	workerDeploymentStore := deploymentstore.NewStore(db)
	workerLangfuseStore := langfuse.NewStore(db)
	workerSlackStore := slackidentity.NewStore(db)

	// Start River queue (handles all periodic workers)
	rq, rqErr := riverqueue.New(workerCtx, cfg.Database.URL, riverqueue.Config{
		DB:                      db,
		OMClient:                omClient,
		AccountStore:            accountStore,
		AgentIndex:              agentIndex,
		AvatarStore:             avatarStore,
		K8sRegistry:             k8sReg,
		K8sCache:                k8sCache,
		ServerConfig:            cfg,
		WorkOSAPIKey:            cfg.Auth.WorkOSAPIKey,
		WorkOSClient:            workosClient,
		OrgClient:               orgClient,
		PromClient:              promClient,
		Logger:                  log,
		LangfuseStore:           workerLangfuseStore,
		PipesClient:             pipesClient,
		GitHubStore:             ghStore,
		ImagePreflighter:        imagePreflighter,
		InsightsSummaryComputer: handlers.NewInsightsSummaryComputer(log, cfg, workerLangfuseStore, workerDeploymentStore, accountStore, workerSlackStore),
	})
	if rqErr != nil {
		log.Error("Failed to create River queue", "error", rqErr)
	} else {
		if startErr := rq.Start(workerCtx); startErr != nil {
			log.Error("Failed to start River queue", "error", startErr)
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
	probeHandler := deps.Probe

	agentIndex := deps.Clients.AgentIndex
	k8sClient := deps.Clients.K8s
	k8sReg := deps.Clients.Registry
	lokiClient := deps.Clients.Loki
	orgClient := deps.Clients.Org
	orgSync := deps.Clients.OrgSync
	omClient := deps.Clients.OpenMeter
	pipesClient := deps.Clients.Pipes
	promClient := deps.Clients.Prom
	k8sCache := deps.Clients.K8sCache
	imagePreflighter := deps.Clients.Preflight
	queue := deps.Clients.Queue

	accountStore := deps.Stores.Account
	deploymentStore := deps.Stores.Deployment
	accountVarsStore := deps.Stores.AccountVars
	heartStore := deps.Stores.Heart
	chatStore := chatstore.New(db)
	agentMetricsStore := deps.Stores.AgentMetrics
	clusterStore := deps.Stores.Cluster
	auditStore := deps.Stores.Audit
	avatarStore := deps.Stores.Avatar
	ksStore := deps.Stores.Knowledge
	ghStore := deps.Stores.GH
	webhookStore := deps.Stores.Webhook
	slackIdentityStore := deps.Stores.SlackID

	billing := openmeter.NewBillingStateManager(omClient, db, log)

	// AI Gateway wiring for handler-side use (dev-key issuance). Worker side
	// constructs its own provisioner via the deployer; both read the same URL +
	// master key from config. Nil when AI_GATEWAY_URL is unset — the dev-key
	// handler returns 503 in that case.
	var aiGatewayProvisioner *aigateway.Provisioner
	var aiGatewayDevStore *aigateway.DevStore
	if cfg.Deployment.AIGatewayURL != "" {
		aiGatewayProvisioner = aigateway.NewProvisioner(
			aigateway.NewClient(cfg.Deployment.AIGatewayURL, cfg.Deployment.AIGatewayMasterKey),
		)
		aiGatewayDevStore = aigateway.NewDevStore(db)
	}

	// OpenAPI spec builder — routes registered via api.GET/POST/etc are
	// both added to gin AND documented in the generated spec.
	api := oapispec.New("Astro API", "1.0.0", "Platform for deploying and running AI agents. Provides agent-native infrastructure including models, knowledge bases, tool integrations, and observability.")
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
	authHandler := handlers.NewAuthHandler(log, cfg, accountStore)
	if orgSync != nil {
		authHandler.SetOrgSync(orgSync)
	}
	if avatarStore != nil {
		authHandler.SetAvatarStore(avatarStore)
	}

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
	)
	log.Info("Authentication enabled", "provider", "WorkOS")

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Deploy-token-authenticated routes — called by messaging containers
		deployTokenRoutes := v1.Group("")
		deployTokenRoutes.Use(middleware.RequireDeployToken(cfg.Security.DeployTokenSecret))
		{
			deployTokenRoutes.GET("/deployments/authorize", handlers.CheckDeploymentAuthorization(log, authzStore, slackIdentityStore))
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
		api.GET(v1, "/accounts/:account", "Get account details", handlers.GetAccount(log, accountStore, authHandler.GetWorkOSClient()),
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

			// Account management
			api.GET(protected, "/accounts/search", "Search accounts", handlers.SearchAccounts(log, accountStore),
				oapispec.Tags("Accounts"),
				oapispec.BearerAuth(),
				oapispec.QueryParam("q", "Search query (min 3 chars)", true),
				oapispec.QueryParam("type", "Filter by type: personal or organization", false),
				oapispec.QueryParam("limit", "Max results (default 10, max 10)", false),
				oapispec.Response(200, &handlers.SearchAccountsResponse{}),
			)
			api.POST(protected, "/accounts", "Create an account", handlers.CreateAccount(log, accountStore, orgClient, orgSync, omClient, cfg.OpenMeterDefaultPlan, auditStore),
				oapispec.Tags("Accounts"),
				oapispec.BearerAuth(),
				oapispec.Body(&handlers.CreateAccountRequest{}),
				oapispec.Response(201, &handlers.AccountResponse{}),
				oapispec.Response(409, &handlers.ErrorResponse{}),
			)

			// Account-scoped routes (owner/admin)
			accountAdmin := protected.Group("/accounts/:account")
			accountAdmin.Use(middleware.ResolveAccount(accountStore))
			accountAdmin.Use(middleware.RequireAccountPermission(accountStore, "org:admin"))
			{
				api.PATCH(accountAdmin, "", "Update account", handlers.UpdateAccount(log, accountStore, auditStore),
					oapispec.Tags("Accounts"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.UpdateAccountRequest{}),
					oapispec.Response(200, &handlers.MessageResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
				)
				api.PUT(accountAdmin, "", "Rename account", handlers.RenameAccount(log, accountStore, agentIndex, avatarStore, orgClient, omClient, auditStore),
					oapispec.Tags("Accounts"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.RenameAccountRequest{}),
					oapispec.Response(200, &handlers.RenameAccountResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
				)
				api.DELETE(accountAdmin, "", "Delete account", handlers.DeleteAccount(log, accountStore, deploymentStore, queue, orgClient, auditStore),
					oapispec.Tags("Accounts"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.MessageResponse{}),
					oapispec.Response(500, &handlers.ErrorResponse{}),
				)
				if avatarStore != nil {
					api.POST(accountAdmin, "/avatar", "Upload account avatar", handlers.UploadAvatar(log, accountStore, avatarStore, auditStore),
						oapispec.Tags("Avatars"),
						oapispec.BearerAuth(),
						oapispec.PathParam("account", "Account name"),
						oapispec.Response(200, &handlers.AvatarResponse{}),
						oapispec.Response(400, &handlers.ErrorResponse{}),
					)
					api.PUT(accountAdmin, "/avatar/preset/:index", "Set avatar to preset", handlers.SetAvatarPreset(log, accountStore, avatarStore, auditStore),
						oapispec.Tags("Avatars"),
						oapispec.BearerAuth(),
						oapispec.PathParam("account", "Account name"),
						oapispec.PathParam("index", "Preset index (1-25)"),
						oapispec.Response(200, &handlers.AvatarResponse{}),
						oapispec.Response(400, &handlers.ErrorResponse{}),
					)
					api.DELETE(accountAdmin, "/avatar", "Reset account avatar", handlers.ResetAvatar(log, accountStore, avatarStore, auditStore),
						oapispec.Tags("Avatars"),
						oapispec.BearerAuth(),
						oapispec.PathParam("account", "Account name"),
						oapispec.Response(200, &handlers.AvatarResponse{}),
					)
				}
				// Audit log
				api.GET(accountAdmin, "/audit-log/filters", "List audit log filter options", handlers.ListAuditLogFilters(log, auditStore),
					oapispec.Tags("Audit"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &auditlog.FilterOptions{}),
				)
				api.GET(accountAdmin, "/audit-log", "List audit log entries", handlers.ListAuditLog(log, auditStore),
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
			}

			// Account-scoped routes (admin or owner — org:manage)
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
			}

			// Account variables (vault) — WorkOS permissions variable:read / variable:write on owner + admin roles.
			accountVarsRead := protected.Group("/accounts/:account")
			accountVarsRead.Use(middleware.ResolveAccount(accountStore))
			accountVarsRead.Use(middleware.RequireAccountPermission(accountStore, "variable:read"))
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
			accountVarsWrite.Use(middleware.RequireAccountPermission(accountStore, "variable:write"))
			{
				api.POST(accountVarsWrite, "/variables", "Create account variables", handlers.CreateAccountVariable(log, accountVarsStore, cfg),
					oapispec.Tags("Variables"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.CreateAccountVariablesRequest{}),
					oapispec.Response(200, &handlers.CreateAccountVariablesResponse{}),
				)
				api.PUT(accountVarsWrite, "/variables/:varName", "Update account variable", handlers.UpdateAccountVariable(log, accountVarsStore, cfg),
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
				api.GET(accountMember, "/usage", "Get account usage", handlers.GetAccountUsage(log, omClient),
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
				api.POST(accountMember, "/ai-gateway-keys", "Issue an ephemeral AI Gateway key for local dev",
					handlers.IssueAIGatewayDevKey(log, aiGatewayProvisioner, aiGatewayDevStore, cfg),
					oapispec.Tags("AI Gateway"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.AIGatewayKeyResponse{}),
					oapispec.Response(502, &handlers.ErrorResponse{}),
					oapispec.Response(503, &handlers.ErrorResponse{}),
				)
				api.GET(accountMember, "/usage/infrastructure", "Get account infrastructure usage", handlers.GetInfrastructureUsage(log, omClient),
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
				api.POST(accountMember, "/knowledge", "Create a managed knowledge store", ent.Wrap(handlers.CreateKnowledgeStore(log, ksStore, k8sClient, k8sReg, cfg, omClient, db), "knowledge_stores", "knowledge_storage"),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(202, &handlers.KnowledgeResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
					oapispec.Response(402, &handlers.ErrorResponse{}),
					oapispec.Response(403, &handlers.ErrorResponse{}),
					oapispec.Response(409, &handlers.ErrorResponse{}),
				)
				api.POST(accountMember, "/knowledge/connect", "Connect an external knowledge store", ent.Wrap(handlers.ConnectKnowledgeStore(log, ksStore, cfg, queue, omClient, db, ent), "knowledge_stores"),
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
				api.GET(accountMember, "/knowledge/:name", "Get a knowledge store", handlers.GetKnowledgeStore(log, ksStore, k8sClient),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Store name"),
					oapispec.Response(200, &handlers.KnowledgeResponse{}),
					oapispec.Response(404, &handlers.ErrorResponse{}),
				)
				api.DELETE(accountMember, "/knowledge/:name", "Delete a knowledge store", handlers.DeleteKnowledgeStore(log, ksStore, k8sClient, queue, omClient, db, billing),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Store name"),
					oapispec.Response(200, &handlers.MessageResponse{}),
					oapispec.Response(404, &handlers.ErrorResponse{}),
				)
				api.GET(accountMember, "/knowledge/:name/logs", "Stream knowledge store logs", handlers.GetKnowledgeStoreLogs(log, ksStore, k8sClient, lokiClient),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Store name"),
					oapispec.Response(200, nil),
				)
				api.GET(accountMember, "/knowledge/:name/logs/stream", "Stream knowledge store logs (SSE)", handlers.StreamKnowledgeStoreLogs(log, ksStore, k8sClient, lokiClient),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Store name"),
					oapispec.Response(200, nil),
				)
				api.GET(accountMember, "/knowledge/:name/metrics", "Knowledge store infrastructure metrics", handlers.GetKnowledgeStoreMetrics(log, ksStore, promClient),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Store name"),
					oapispec.Response(200, &handlers.KnowledgeMetricsResponse{}),
				)
				api.GET(accountMember, "/knowledge/:name/events", "Stream knowledge store provisioning events", handlers.GetKnowledgeStoreEvents(log, ksStore, k8sClient),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Store name"),
					oapispec.Response(200, nil),
				)
				api.GET(accountMember, "/knowledge/:name/credentials", "Retrieve knowledge store credentials", handlers.GetKnowledgeStoreCredentials(log, ksStore, &k8s.KnowledgeSecretReader{Clientset: k8sClient.Clientset()}),
					oapispec.Tags("Knowledge"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Store name"),
					oapispec.Response(200, &handlers.KnowledgeCredentialsResponse{}),
					oapispec.Response(404, &handlers.ErrorResponse{}),
				)
			}

			// Member routes — list and self-removal are membership-only, other mutations require org:manage
			memberRoutes := protected.Group("/accounts/:account/members")
			memberRoutes.Use(middleware.ResolveAccount(accountStore))
			memberRoutes.Use(middleware.RequireAccountMember(accountStore))
			{
				api.GET(memberRoutes, "", "List account members", handlers.ListMembers(log, accountStore, orgClient, slackIdentityStore),
					oapispec.Tags("Members"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.ListMembersResponse{}),
				)
				// Remove member — handler allows self-removal for any member,
				// but requires org:manage to remove others.
				api.DELETE(memberRoutes, "/:user_id", "Remove a member", handlers.RemoveMember(log, orgSync, accountStore, omClient, db, auditStore),
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
					ent.Wrap(handlers.AddMember(log, orgSync, accountStore, omClient, db, auditStore), "members"),
					oapispec.Tags("Members"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.AddMemberRequest{}),
					oapispec.Response(201, &handlers.AddMemberResponse{}),
				)
				api.PUT(memberManageRoutes, "/:user_id", "Update member role", handlers.UpdateMemberRole(log, orgSync, accountStore, auditStore),
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
					ent.Wrap(handlers.CreateBlueprint(log, agentIndex, accountStore, auditStore, avatarStore, omClient, db), "agents"),
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
				api.GET(agentReadRoutes, "/usage/infrastructure", "Get agent infrastructure usage", handlers.GetInfrastructureUsage(log, omClient),
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
					ent.Wrap(handlers.RegisterAgent(log, agentIndex, omClient, cfg.Server.MinCLIVersion, db, auditStore, avatarStore, deploymentStore, k8sCache, cfg.Deployment.AIGatewayURL != ""), "agents", "agent_builds"),
					oapispec.Tags("Agents"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Agent name"),
					oapispec.Body(&handlers.RegisterAgentRequest{}),
					oapispec.Response(201, &handlers.RegisterAgentResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
					oapispec.Response(426, &handlers.ErrorResponse{}),
				)
				api.POST(agentWriteRoutes, "/archive", "Archive an agent template", handlers.ArchiveAgent(log, agentIndex, omClient, db, auditStore, ghStore, webhookStore, pipesClient),
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
				api.POST(agentWriteRoutes, "/transfer", "Transfer agent to another account", handlers.TransferAgent(log, agentIndex, accountStore, avatarStore, auditStore, deploymentStore, k8sCache),
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
			}

			// clusterStore validates optional `target.cluster_id` on deploy specs.
			api.POST(protected, "/deploy", "Deploy an agent", handlers.DeployAgent(log, agentIndex, accountStore, cfg, deploymentStore, accountVarsStore, clusterStore, k8sReg, ent, queue, avatarStore, omClient, db, auditStore, ksStore, authzStore, imagePreflighter, tmplCache, k8sCache),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.Desc("Accepts a fulfilled deployment spec (YAML or JSON) and schedules async deployment to Kubernetes."),
				oapispec.Response(202, &handlers.DeployResponseAlias{}),
			)
			api.POST(protected, "/deploy/validate", "Validate a deployment spec", handlers.ValidateDeployment(log, agentIndex, accountStore, cfg),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.Desc("Validates a fulfilled deployment spec without applying it."),
				oapispec.Response(200, &handlers.ValidateDeploymentResponse{}),
			)
			api.POST(protected, "/undeploy", "Undeploy an agent", handlers.UndeployAgent(log, agentIndex, accountStore, cfg, deploymentStore, queue, omClient, db, auditStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.Body(&deployment.UndeployRequest{}),
				oapispec.Response(202, &handlers.UndeployResponseAlias{}),
			)
			api.GET(protected, "/deployments/:id/status", "Get deployment status", handlers.GetDeploymentStatus(log, accountStore, k8sReg, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, &handlers.GetDeploymentStatusResponse{}),
			)
			api.PATCH(protected, "/deployments/:id", "Update deployment display name", handlers.UpdateDeploymentDisplayName(log, accountStore, deploymentStore, auditStore, k8sCache),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
			)
			api.POST(protected, "/deployments/:id/wakeup", "Wake up a scaled-down deployment", handlers.WakeUpDeployment(log, accountStore, deploymentStore, queue, auditStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(202, nil),
			)
			api.POST(protected, "/deployments/:id/stop", "Stop a running deployment", handlers.StopDeployment(log, accountStore, k8sReg, deploymentStore, auditStore, k8sCache),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(202, nil),
			)
			api.POST(protected, "/deployments/:id/rollback", "Rollback to a previous revision", handlers.RollbackDeployment(log, accountStore, deploymentStore, queue, auditStore, k8sCache),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(202, nil),
			)
			api.POST(protected, "/deployments/:id/restart", "Restart all pods in a deployment", handlers.RestartDeployment(log, accountStore, cfg, k8sReg, deploymentStore, auditStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.RestartDeploymentResponse{}),
			)
			api.POST(protected, "/deployments/:id/pods/:pod/restart", "Restart a pod", handlers.RestartPod(log, accountStore, cfg, k8sReg, deploymentStore, auditStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.PathParam("pod", "Pod name"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.RestartPodResponse{}),
			)
			api.POST(protected, "/deployments/:id/ingestion/:ingestion/trigger", "Trigger an ingestion job", handlers.TriggerIngestion(log, agentIndex, accountStore, k8sReg, deploymentStore, cfg, auditStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.PathParam("ingestion", "Ingestion name"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.TriggerIngestionResponse{}),
			)

			if avatarStore != nil {
				api.POST(protected, "/deployments/:id/avatar", "Upload deployment avatar",
					handlers.UploadDeploymentAvatar(log, accountStore, deploymentStore, avatarStore, auditStore, k8sCache),
					oapispec.Tags("Avatars"),
					oapispec.BearerAuth(),
					oapispec.PathParam("id", "Deployment ID"),
					oapispec.Response(200, &handlers.AvatarResponse{}),
				)
				api.DELETE(protected, "/deployments/:id/avatar", "Reset deployment avatar",
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
			api.GET(protected, "/agents/:account/:name/deployment/history", "Get deployment history", handlers.GetDeploymentHistory(log, accountStore, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("name", "Agent name"),
				oapispec.Response(200, &handlers.DeploymentHistoryResponse{}),
			)
			api.GET(protected, "/deployments/count", "Count deployments", handlers.CountDeployments(log, accountStore, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, nil),
			)
			api.GET(protected, "/deployments/summary", "List deployment summaries", handlers.ListDeploymentsSummary(log, accountStore, deploymentStore, avatarStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.Response(200, &handlers.DeploymentsSummaryResponse{}),
			)
			api.GET(protected, "/deployments", "List deployments", handlers.ListDeployments(log, accountStore, deploymentStore, agentIndex, avatarStore, auditStore, k8sCache),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.ListDeploymentsResponse{}),
			)
			api.GET(protected, "/deployments/:id", "Get deployment", handlers.GetDeployment(log, accountStore, cfg, deploymentStore, agentIndex, avatarStore, auditStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.GetDeploymentDetailResponse{}),
			)
			api.GET(protected, "/deployments/:id/runtime", "Get deployment runtime", handlers.GetDeploymentRuntime(log, accountStore, cfg, k8sReg, deploymentStore, agentIndex, k8sCache),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.GetDeploymentRuntimeResponse{}),
			)
			// Messaging proxy — send + SSE only; durable history is /deployments/:id/chat (see deployment-chat.md).
			// Validates Astro session, forwards to the deployment messaging sidecar, and
			// injects x-amzn-oidc-identity for upstream auth.
			messagingProxy := handlers.ProxyDeploymentMessaging(log, accountStore, deploymentStore, k8sReg, cfg, chatStore)
			protected.Any("/deployments/:id/messaging/*proxyPath", messagingProxy)
			api.GET(protected, "/deployments/:id/chat/conversations", "List deployment chat conversations",
				handlers.ListDeploymentChatConversations(log, accountStore, deploymentStore, chatStore),
				oapispec.Tags("Chat"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, &handlers.ListChatConversationsResponse{}),
			)
			api.GET(protected, "/deployments/:id/chat/conversations/:conversationId", "Get deployment chat conversation",
				handlers.GetDeploymentChatConversation(log, accountStore, deploymentStore, chatStore),
				oapispec.Tags("Chat"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("conversationId", "Conversation ID"),
				oapispec.QueryParam("limit", "Max messages to return (tail or older page; max 1000)", false),
				oapispec.QueryParam("before_seq", "Return messages older than this seq (requires limit)", false),
				oapispec.Response(200, &handlers.GetChatConversationResponse{}),
			)
			api.PUT(protected, "/deployments/:id/chat/conversations/:conversationId", "Upsert deployment chat conversation",
				handlers.UpsertDeploymentChatConversation(log, accountStore, deploymentStore, chatStore),
				oapispec.Tags("Chat"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("conversationId", "Conversation ID"),
			)
			api.PUT(protected, "/deployments/:id/chat/conversations/:conversationId/messages", "Replace deployment chat messages",
				handlers.ReplaceDeploymentChatMessages(log, accountStore, deploymentStore, chatStore),
				oapispec.Tags("Chat"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("conversationId", "Conversation ID"),
			)
			api.POST(protected, "/deployments/:id/chat/conversations/:conversationId/messages", "Append deployment chat message",
				handlers.AppendDeploymentChatMessage(log, accountStore, deploymentStore, chatStore),
				oapispec.Tags("Chat"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("conversationId", "Conversation ID"),
			)
			// Authorization is configured exclusively through `interfaces.auth`
			// in the deployment spec — no imperative endpoints here. The only
			// authorization endpoint is the messaging-facing
			// /deployments/authorize wired below behind RequireDeployToken.

			api.GET(protected, "/deployments/:id/logs", "Get deployment logs", handlers.GetDeploymentLogs(log, accountStore, cfg, k8sReg, deploymentStore, lokiClient),
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
			api.GET(protected, "/deployments/:id/logs/stream", "Stream deployment logs (SSE)", handlers.StreamDeploymentLogs(log, accountStore, k8sReg, deploymentStore, lokiClient),
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
			api.GET(protected, "/deployments/:id/events", "Get deployment K8s events", handlers.GetDeploymentEvents(log, accountStore, k8sReg, deploymentStore, k8sCache),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, &handlers.DeploymentEventsResponse{}),
			)
			api.GET(protected, "/deployments/:id/configmap/:cmname", "Get ConfigMap data", handlers.GetConfigMapData(log, accountStore, cfg, k8sReg, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.PathParam("cmname", "ConfigMap name"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.ConfigMapDataResponse{}),
			)
			api.GET(protected, "/deployments/:id/secret/:secretname/keys", "Get Secret key names", handlers.GetSecretKeys(log, accountStore, cfg, k8sReg, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.PathParam("secretname", "Secret name"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.SecretKeysResponse{}),
			)
			// Observability endpoints (deployment-scoped, backed by Langfuse)
			langfuseStore := langfuse.NewStore(db)
			api.GET(protected, "/deployments/:id/observability/metrics", "Get deployment metrics", handlers.GetLangfuseMetrics(log, cfg, accountStore, deploymentStore, langfuseStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("start_time", "Start time (RFC3339)", false),
				oapispec.QueryParam("end_time", "End time (RFC3339)", false),
				oapispec.Response(200, &handlers.ObservabilityMetricsResponse{}),
			)
			api.GET(protected, "/deployments/:id/observability/summary", "Get deployment summary", handlers.GetLangfuseSummary(log, cfg, accountStore, deploymentStore, langfuseStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("start_time", "Start time (RFC3339)", false),
				oapispec.QueryParam("end_time", "End time (RFC3339)", false),
				oapispec.Response(200, &handlers.ObservabilitySummaryResponse{}),
			)
			api.GET(protected, "/deployments/:id/observability/traces", "Get deployment traces", handlers.GetLangfuseTraces(log, cfg, accountStore, deploymentStore, langfuseStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("start_time", "Start time (RFC3339)", false),
				oapispec.QueryParam("end_time", "End time (RFC3339)", false),
				oapispec.QueryParam("limit", "Page size (default 50)", false),
				oapispec.QueryParam("offset", "Pagination offset (default 0)", false),
				oapispec.Response(200, &handlers.ObservabilityTracesResponse{}),
			)
			api.GET(protected, "/deployments/:id/observability/traces/:traceId", "Get a single trace with its observations", handlers.GetLangfuseTraceDetail(log, cfg, accountStore, deploymentStore, langfuseStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("traceId", "Trace ID"),
				oapispec.Response(200, &handlers.TraceDetailResponse{}),
			)
			api.GET(protected, "/deployments/:id/observability/observations/:observationId", "Get a single observation with full input/output/metadata", handlers.GetLangfuseObservationDetail(log, cfg, accountStore, deploymentStore, langfuseStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.PathParam("observationId", "Observation ID"),
				oapispec.Response(200, &handlers.Observation{}),
			)
			// Dataset endpoints (deployment-scoped, backed by Langfuse + eval_datasets)
			datasetStore := datasetstore.NewStore(db)
			api.GET(protected, "/deployments/:id/dataset", "Get deployment dataset", handlers.GetEvalDataset(log, accountStore, deploymentStore, datasetStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("page", "Page number (default 1)", false),
				oapispec.QueryParam("limit", "Page size (default 50, max 100)", false),
				oapispec.Response(200, nil),
			)
			api.GET(protected, "/deployments/:id/dataset/download", "Download deployment dataset as zip", handlers.DownloadEvalDataset(log, cfg, accountStore, deploymentStore, datasetStore, langfuseStore),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(200, nil),
			)
			api.POST(protected, "/deployments/:id/dataset/sync", "Trigger an immediate dataset sync", handlers.TriggerEvalDatasetSync(log, accountStore, deploymentStore, queue),
				oapispec.Tags("Dataset"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(202, nil),
			)
			// Account-scoped observability (aggregates across all account deployments)
			api.GET(protected, "/accounts/:account/observability/summary", "Get account observability summary", handlers.GetAccountLangfuseSummary(log, cfg, accountStore, deploymentStore, langfuseStore, slackIdentityStore, k8sCache),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.QueryParam("from", "Period start (RFC3339)", false),
				oapispec.QueryParam("to", "Period end (RFC3339)", false),
				oapispec.QueryParam("group_by", "Set to 'user' to include cost_over_time_by_user", false),
				oapispec.Response(200, &handlers.AccountObservabilitySummaryResponse{}),
			)
			api.GET(protected, "/accounts/:account/observability/deployments-summary", "Get per-deployment observability summary", handlers.GetAccountDeploymentsSummary(log, cfg, accountStore, deploymentStore, langfuseStore, slackIdentityStore, k8sCache),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.QueryParam("from", "Period start (RFC3339)", false),
				oapispec.QueryParam("to", "Period end (RFC3339)", false),
				oapispec.Response(200, &handlers.AccountDeploymentsSummaryResponse{}),
			)
			api.GET(protected, "/accounts/:account/observability/users-summary", "Get per-user observability summary", handlers.GetAccountUsersSummary(log, cfg, accountStore, deploymentStore, langfuseStore, slackIdentityStore, k8sCache),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.QueryParam("from", "Period start (RFC3339)", false),
				oapispec.QueryParam("to", "Period end (RFC3339)", false),
				oapispec.Response(200, &handlers.AccountUsersSummaryResponse{}),
			)

			api.GET(accountMember, "/observability/deployment-summaries", "Get bulk deployment observability summaries", handlers.GetLangfuseSummaries(log, deploymentStore, k8sCache),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.Response(200, &handlers.DeploymentSummariesResponse{}),
			)

			// Network endpoints (deployment-scoped, backed by Beyla eBPF metrics in Prometheus)
			api.GET(protected, "/deployments/:id/network/summary", "Get deployment network summary", handlers.GetNetworkSummary(log, accountStore, deploymentStore, k8sReg, promClient),
				oapispec.Tags("Network"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.QueryParam("start_time", "Start time (RFC3339); defaults to one hour ago", false),
				oapispec.QueryParam("end_time", "End time (RFC3339); defaults to now", false),
				oapispec.Response(200, &handlers.NetworkSummaryResponse{}),
			)
			api.GET(protected, "/deployments/:id/network/flows", "Get top network peers for a deployment", handlers.GetNetworkFlows(log, accountStore, deploymentStore, k8sReg, promClient),
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
			api.GET(protected, "/deployments/:id/network/timeseries", "Get bucketed network metrics for a deployment", handlers.GetNetworkTimeseries(log, accountStore, deploymentStore, k8sReg, promClient),
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
			api.GET(protected, "/deployments/:id/pods/:pod/metrics", "Get CPU, memory and storage time series for a pod", handlers.GetWorkloadMetrics(log, accountStore, deploymentStore, promClient, k8sReg),
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

		// GitHub webhook receiver (no auth — HMAC verified inside handler)
		router.POST("/webhooks/github", handlers.GitHubWebhook(log, ghStore, webhookStore, queue))
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
		if cfg.Deployment.Environment != "local" {
			log.Warn("Admin gRPC disabled — mTLS not configured (set ADMIN_GRPC_CERT_FILE, ADMIN_GRPC_KEY_FILE, ADMIN_GRPC_CA_FILE)")
			return nil, nil
		}
		log.Warn("Admin gRPC starting without TLS (local dev only)")
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
		log.Info("Admin gRPC server listening", "port", port)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Error("Admin gRPC server error", "error", err)
		}
	}()

	return grpcSrv, nil
}

// startFleetGRPCServer starts the Fleet gRPC server over QUIC for CLI device connections.
// Returns nil, nil if the port is empty (disabled) or TLS certs are not configured.
func startFleetGRPCServer(
	log *logger.Logger,
	cfg *config.Config,
	devStore *devicestore.Store,
	accountStore *account.AccountStore,
	jwtValidator *auth.JWTValidator,
) (*grpc.Server, *connectgrpc.Server, error) {
	port := cfg.FleetGRPC.Port
	if port == "" {
		return nil, nil, nil
	}

	// TLS certs provided by platform via fleet-tls K8s secret
	certFile := cfg.FleetGRPC.CertFile
	keyFile := cfg.FleetGRPC.KeyFile
	if certFile == "" || keyFile == "" {
		log.Warn("Fleet gRPC disabled — TLS not configured (set FLEET_TLS_CERT_PATH, FLEET_TLS_KEY_PATH)")
		return nil, nil, nil
	}

	// Load TLS config for QUIC (QUIC mandates TLS 1.3)
	tlsCert, err := connectgrpc.LoadTLSCert(certFile, keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("fleet gRPC TLS: %w", err)
	}

	tlsConf := connectgrpc.NewTLSConfig(tlsCert)

	// Create QUIC listener
	lis, err := connectgrpc.ListenQUIC(":"+port, tlsConf, log)
	if err != nil {
		return nil, nil, fmt.Errorf("fleet gRPC QUIC listen: %w", err)
	}

	// Create gRPC server with JWT stream interceptor
	// TLS is handled by QUIC, so gRPC uses insecure credentials over the QUIC stream
	grpcSrv := grpc.NewServer(
		grpc.StreamInterceptor(connectgrpc.JWTStreamInterceptor(jwtValidator, log)),
	)

	srv := connectgrpc.New(log, devStore, accountStore)
	connectv1.RegisterConnectServiceServer(grpcSrv, srv)

	// Start reaper to clean stale devices
	srv.StartReaper(context.Background())

	go func() {
		log.Info("Fleet gRPC server listening (QUIC/UDP)", "port", port)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Error("Fleet gRPC server error", "error", err)
		}
	}()

	return grpcSrv, srv, nil
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
