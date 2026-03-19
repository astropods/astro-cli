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

	"github.com/astropods/astro/apps/astro-server/handlers"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/admingrpc"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/connectgrpc"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/devicestore"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/loki"
	"github.com/astropods/astro/apps/astro-server/internal/metricsstore"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	oapispec "github.com/astropods/astro/apps/astro-server/internal/openapi"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	"github.com/astropods/astro/apps/astro-server/internal/waitlist"
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

	// Initialize account store (needed by both API and worker)
	accountStore := account.NewAccountStore(db)

	// Initialize WorkOS organization client
	var orgClient *org.Client
	var orgSync *org.Sync
	if cfg.Auth.WorkOSAPIKey != "" {
		orgClient = org.NewClient(cfg.Auth.WorkOSAPIKey)
		workosClient := auth.NewWorkOSClient(cfg.Auth.WorkOSAPIKey, cfg.Auth.WorkOSClientID, cfg.Auth.RedirectURI, cfg.Auth.FrontendURL)
		orgSync = org.NewSync(orgClient, accountStore, workosClient)
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

	// Track components for graceful shutdown
	var httpSrv *http.Server
	var grpcServer *grpc.Server
	var connectGRPCServer *grpc.Server
	var eventsCancel context.CancelFunc
	var probeHandler *handlers.ProbeHandler
	var adminSrv *admingrpc.Server
	var apiQueue *riverqueue.Queue

	// --- API mode: HTTP server + gRPC admin + gRPC connect ---
	if cfg.RunAPI() {
		httpSrv, grpcServer, connectGRPCServer, probeHandler, adminSrv, apiQueue = runAPI(log, cfg, db, accountStore, orgClient, orgSync, omClient, ent)
	}

	// --- Worker mode: events consumer ---
	if cfg.RunWorker() {
		eventsCancel = runWorker(log, cfg, accountStore, db, omClient, orgClient)
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
	if connectGRPCServer != nil {
		connectGRPCServer.GracefulStop()
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

// runAPI initializes and starts the HTTP API server, gRPC admin server, and connect gRPC server.
func runAPI(
	log *logger.Logger,
	cfg *config.Config,
	db *sql.DB,
	accountStore *account.AccountStore,
	orgClient *org.Client,
	orgSync *org.Sync,
	omClient *openmeter.Client,
	ent *middleware.Entitlements,
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

	// Initialize stores
	agentIndex := agentindex.NewIndexWithDB(db)
	deploymentStore := deploymentstore.NewStore(db)
	waitlistStore := waitlist.NewStore(db)
	heartStore := heartstore.New(db)
	agentMetricsStore := metricsstore.New(db)
	log.Info("Agent index and stores initialized")

	// Initialize Kubernetes client
	var k8sClient k8s.ClusterClient
	clientMode := k8s.ClientMode(cfg.Deployment.K8sClientMode)
	log.Info("Initializing Kubernetes client", "mode", string(clientMode))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	var k8sErr error
	k8sClient, k8sErr = k8s.NewClusterClient(ctx, k8s.ClusterClientConfig{
		Mode:            clientMode,
		ClusterName:     cfg.Deployment.EKSClusterName,
		ClusterEndpoint: cfg.Deployment.K8sMasterURL,
		Region:          cfg.Deployment.AWSRegion,
		KubeconfigPath:  cfg.Deployment.KubeconfigPath,
		KubeContext:     cfg.Deployment.KubeContext,
		Logger:          log,
	})
	cancel()
	if k8sErr != nil {
		log.Warn("Failed to create K8s client", "error", k8sErr)
		log.Warn("Kubernetes features will be unavailable")
		k8sClient = nil
	} else {
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

	// Initialize Loki client (optional — falls back to K8s pod logs if LOKI_URL is unset)
	var lokiClient *loki.Client
	if cfg.LokiURL != "" {
		lokiClient = loki.New(cfg.LokiURL)
		log.Info("Loki log backend configured", "url", cfg.LokiURL)
	} else {
		log.Warn("LOKI_URL not set — deployment logs will be fetched directly from K8s pods")
	}

	// Initialize probe handler
	probeHandler := handlers.NewProbeHandler(log, agentIndex, k8sClient)

	// Create insert-only River queue for API (no workers, no periodic jobs — workers run in runWorker)
	rq, rqErr := riverqueue.NewInsertOnly(context.Background(), cfg.Database.URL, log)
	if rqErr != nil {
		log.Error("Failed to create River queue for API", "error", rqErr)
		os.Exit(1)
	}

	// Register routes
	setupRoutes(router, log, agentIndex, accountStore, deploymentStore, waitlistStore, heartStore, agentMetricsStore, cfg, probeHandler, k8sClient, lokiClient, orgClient, orgSync, omClient, ent, db, rq)

	// Start admin gRPC server
	adminSrv := admingrpc.New(log, deploymentStore, k8sClient, lokiClient, db, cfg.AdminGRPC.OpenMeterURL, cfg.Database.URL, rq, cfg.Deployment.IngressDomain, cfg.Deployment.IngestionIngressDomain)
	grpcServer, grpcErr := startAdminGRPCServer(log, cfg, adminSrv)
	if grpcErr != nil {
		log.Error("Failed to start admin gRPC server", "error", grpcErr)
		os.Exit(1)
	}

	// Start connect gRPC server (QUIC, JWT auth)
	devStore := devicestore.New(db)
	workosClient := auth.NewWorkOSClient(cfg.Auth.WorkOSAPIKey, cfg.Auth.WorkOSClientID, cfg.Auth.RedirectURI, cfg.Auth.FrontendURL)
	jwksURL, _ := workosClient.GetJWKSURL()
	connectJWTValidator := auth.NewJWTValidator(jwksURL, cfg.Auth.JWTIssuer, "")
	connectServer, connectSrv, connectErr := startConnectGRPCServer(log, cfg, devStore, accountStore, connectJWTValidator)
	if connectErr != nil {
		log.Error("Failed to start connect gRPC server", "error", connectErr)
		os.Exit(1)
	}

	// Wire gin router as HTTP handler for admin ProxyHTTP
	adminSrv.SetHTTPHandler(router)

	// Wire WorkOS client ID for admin GetAuthConfig
	adminSrv.SetWorkOSClientID(cfg.Auth.WorkOSClientID)

	// Wire connect server as command dispatcher for admin
	if connectSrv != nil {
		adminSrv.SetCommandDispatcher(connectSrv)
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

	return srv, grpcServer, connectServer, probeHandler, adminSrv, rq
}

// runWorker starts the River queue for all background job processing and returns a cancel func.
func runWorker(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	db *sql.DB,
	omClient *openmeter.Client,
	orgClient *org.Client,
) context.CancelFunc {
	workerCtx, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel is returned to caller

	// Start namespace scanner (reconciles DB ↔ K8s, catches drift)
	var k8sClient k8s.ClusterClient
	clientMode := k8s.ClientMode(cfg.Deployment.K8sClientMode)
	initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
	k8sClient, k8sErr := k8s.NewClusterClient(initCtx, k8s.ClusterClientConfig{
		Mode:            clientMode,
		ClusterName:     cfg.Deployment.EKSClusterName,
		ClusterEndpoint: cfg.Deployment.K8sMasterURL,
		Region:          cfg.Deployment.AWSRegion,
		KubeconfigPath:  cfg.Deployment.KubeconfigPath,
		KubeContext:     cfg.Deployment.KubeContext,
		Logger:          log,
	})
	initCancel()
	if k8sErr != nil {
		log.Warn("Worker: K8s client unavailable, namespace scanner will skip K8s reconciliation", "error", k8sErr)
		k8sClient = nil
	}

	// Initialize Prometheus query client (nil if PROMETHEUS_URL is empty)
	promClient := promquery.NewClient(cfg.PrometheusURL)
	if promClient != nil {
		log.Info("Prometheus query client initialized", "url", cfg.PrometheusURL)
	}

	// Start River queue (handles all periodic workers)
	rq, rqErr := riverqueue.New(workerCtx, cfg.Database.URL, riverqueue.Config{
		DB:           db,
		OMClient:     omClient,
		AccountStore: accountStore,
		K8sClient:    k8sClient,
		ServerConfig: cfg,
		WorkOSAPIKey: cfg.Auth.WorkOSAPIKey,
		OrgClient:    orgClient,
		PromClient:   promClient,
		Logger:       log,
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

// setupRoutes configures all application routes and builds the OpenAPI spec.
func setupRoutes(router *gin.Engine, log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, deploymentStore *deploymentstore.Store, waitlistStore *waitlist.Store, heartStore *heartstore.Store, agentMetricsStore *metricsstore.Store, cfg *config.Config, probeHandler *handlers.ProbeHandler, k8sClient k8s.ClusterClient, lokiClient *loki.Client, orgClient *org.Client, orgSync *org.Sync, omClient *openmeter.Client, ent *middleware.Entitlements, db *sql.DB, queue *riverqueue.Queue) {
	// OpenAPI spec builder — routes registered via api.GET/POST/etc are
	// both added to gin AND documented in the generated spec.
	api := oapispec.New("Astro API", "1.0.0", "Platform for deploying and running AI agents. Provides agent-native infrastructure including models, knowledge bases, tool integrations, and observability.")

	// Serve OpenAPI spec
	router.GET("/openapi.json", api.JSON())

	// Kubernetes-style health probe endpoints (at root, not part of API spec)
	router.GET("/livez", probeHandler.Livez())
	router.GET("/readyz", probeHandler.Readyz())
	router.GET("/healthz", probeHandler.Healthz())

	// JSON Schema for astropods.yml (public, no auth)
	router.GET("/schema/package.json", handlers.AstroAISpecSchema())

	// CLI install script (no files needed — just returns a shell script)
	router.GET("/install", handlers.CLIInstallScript(cfg))

	// CLI binary download — redirects to CDN for backward compat with older CLI versions
	if cfg.Server.DownloadBaseURL != "" {
		router.GET("/download/:name", handlers.CLIDownload(cfg))
		router.HEAD("/download/:name", handlers.CLIDownload(cfg))
	}

	// Setup authentication
	authHandler := handlers.NewAuthHandler(log, cfg, accountStore)
	if orgSync != nil {
		authHandler.SetOrgSync(orgSync)
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

		// Waitlist signup (public, no auth)
		api.POST(v1, "/waitlist", "Join the waitlist", handlers.JoinWaitlist(log, waitlistStore),
			oapispec.Tags("Waitlist"),
			oapispec.Body(&handlers.WaitlistSignupRequest{}),
			oapispec.Response(201, &handlers.WaitlistEntryResponse{}),
			oapispec.Response(409, &handlers.ErrorResponse{}),
		)

		// Agent registry endpoints (public read, with optional auth for visibility)
		api.GET(v1, "/agents", "List public agents", handlers.ListAgents(log, agentIndex, accountStore, heartStore, agentMetricsStore),
			oapispec.Tags("Agents"),
			oapispec.Response(200, &handlers.ListAgentsResponse{}),
		)
		agentDetail := v1.Group("")
		agentDetail.Use(authMw.OptionalAuth())
		{
			api.GET(agentDetail, "/agents/:account", "List agents for account", handlers.ListAccountAgents(log, agentIndex, accountStore, heartStore, agentMetricsStore),
				oapispec.Tags("Agents"),
				oapispec.PathParam("account", "Account name"),
				oapispec.Response(200, &handlers.ListAgentsResponse{}),
				oapispec.Response(404, &handlers.ErrorResponse{}),
			)
			api.GET(agentDetail, "/agents/:account/:name", "Get agent details", handlers.GetAgent(log, agentIndex, accountStore, heartStore, agentMetricsStore),
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
			api.PATCH(protected, "/me", "Update current user profile", handlers.UpdateProfile(log, authHandler.GetWorkOSClient()),
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
			api.POST(protected, "/accounts", "Create an account", handlers.CreateAccount(log, accountStore, orgClient, orgSync, omClient, cfg.OpenMeterDefaultPlan),
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
				api.PUT(accountAdmin, "", "Rename account", handlers.RenameAccount(log, accountStore),
					oapispec.Tags("Accounts"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.RenameAccountRequest{}),
					oapispec.Response(200, &handlers.RenameAccountResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
				)
				api.DELETE(accountAdmin, "", "Delete account", handlers.DeleteAccount(log, accountStore),
					oapispec.Tags("Accounts"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.MessageResponse{}),
					oapispec.Response(501, &handlers.ErrorResponse{}),
				)
				api.GET(accountAdmin, "/usage", "Get account usage", handlers.GetAccountUsage(log, omClient),
					oapispec.Tags("Usage"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.QueryParam("from", "Start of period (RFC3339, defaults to start of current month)", false),
					oapispec.QueryParam("to", "End of period (RFC3339, defaults to now)", false),
					oapispec.Response(200, &handlers.UsageResponse{}),
					oapispec.Response(503, &handlers.ErrorResponse{}),
				)
				api.POST(accountAdmin, "/quota-increase", "Request quota increase", handlers.RequestQuotaIncrease(log, db),
					oapispec.Tags("Usage"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(201, &handlers.QuotaIncreaseResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
				)
				api.GET(accountAdmin, "/quota-increase", "List quota increase requests", handlers.ListQuotaIncreaseRequests(log, db),
					oapispec.Tags("Usage"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.QuotaIncreaseListResponse{}),
				)
			}

			// Member management (requires org:manage permission)
			memberRoutes := protected.Group("/accounts/:account/members")
			memberRoutes.Use(middleware.ResolveAccount(accountStore))
			memberRoutes.Use(middleware.RequireAccountPermission(accountStore, "org:manage"))
			{
				api.GET(memberRoutes, "", "List account members", handlers.ListMembers(log, accountStore),
					oapispec.Tags("Members"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Response(200, &handlers.ListMembersResponse{}),
				)
				api.POST(memberRoutes, "", "Add a member",
					ent.Wrap(handlers.AddMember(log, orgSync, accountStore, omClient, db), "members"),
					oapispec.Tags("Members"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.AddMemberRequest{}),
					oapispec.Response(201, &handlers.AddMemberResponse{}),
				)
				api.PUT(memberRoutes, "/:user_id", "Update member role", handlers.UpdateMemberRole(log, orgSync, accountStore),
					oapispec.Tags("Members"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("user_id", "User ID"),
					oapispec.Body(&handlers.ChangeMemberRoleRequest{}),
					oapispec.Response(200, &handlers.MessageResponse{}),
				)
				api.DELETE(memberRoutes, "/:user_id", "Remove a member", handlers.RemoveMember(log, orgSync, omClient, db),
					oapispec.Tags("Members"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("user_id", "User ID"),
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
				api.POST(invitationRoutes, "", "Send invitations", handlers.CreateInvitations(log, orgSync),
					oapispec.Tags("Invitations"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.Body(&handlers.BulkInvitationRequest{}),
					oapispec.Response(201, &handlers.BulkInvitationResponse{}),
				)
				api.DELETE(invitationRoutes, "/:id", "Revoke an invitation", handlers.RevokeInvitation(log, orgClient),
					oapispec.Tags("Invitations"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("id", "Invitation ID"),
					oapispec.Response(200, &handlers.MessageResponse{}),
				)
			}

			// Deployment template generation
			api.GET(protected, "/agents/:account/:name/deployment-template", "Get deployment template",
				handlers.GetDeploymentTemplate(log, agentIndex, accountStore, cfg),
				oapispec.Tags("Agents"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("name", "Agent name"),
				oapispec.QueryParam("build", "Specific build ID (default: latest)", false),
				oapispec.QueryParam("format", "Response format: json or yaml (default: yaml)", false),
				oapispec.Desc("Returns a deployment spec template for the agent. Defaults to YAML unless ?format=json."),
				oapispec.Response(200, nil),
			)
			api.GET(protected, "/agents/:account/:name/deployment-template/:deploymentID", "Get pre-filled deployment template",
				handlers.GetPrefilledDeploymentTemplate(log, agentIndex, accountStore, cfg, deploymentStore),
				oapispec.Tags("Agents"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("name", "Agent name"),
				oapispec.PathParam("deploymentID", "Existing deployment ID to pre-fill from"),
				oapispec.QueryParam("build", "Specific build ID (default: latest)", false),
				oapispec.QueryParam("format", "Response format: json or yaml (default: yaml)", false),
				oapispec.Desc("Returns a deployment spec template pre-filled with values from an existing deployment."),
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

			// Agent write operations (requires agents:write permission)
			agentWriteRoutes := protected.Group("/agents/:account/:name")
			agentWriteRoutes.Use(middleware.ResolveAccount(accountStore))
			agentWriteRoutes.Use(middleware.RequireAccountPermission(accountStore, "agents:write"))
			{
				api.POST(agentWriteRoutes, "/register", "Register an agent build",
					ent.Wrap(handlers.RegisterAgent(log, agentIndex, omClient, cfg.Server.MinCLIVersion), "agents", "agent_builds"),
					oapispec.Tags("Agents"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Agent name"),
					oapispec.Body(&handlers.RegisterAgentRequest{}),
					oapispec.Response(201, &handlers.RegisterAgentResponse{}),
					oapispec.Response(400, &handlers.ErrorResponse{}),
					oapispec.Response(426, &handlers.ErrorResponse{}),
				)
				api.PUT(agentWriteRoutes, "/visibility", "Set agent visibility", handlers.SetAgentVisibility(log, agentIndex),
					oapispec.Tags("Agents"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Account name"),
					oapispec.PathParam("name", "Agent name"),
					oapispec.Body(&handlers.SetAgentVisibilityRequest{}),
					oapispec.Response(200, &handlers.SetVisibilityResponse{}),
				)
				api.POST(agentWriteRoutes, "/transfer", "Transfer agent to another account", handlers.TransferAgent(log, agentIndex, accountStore),
					oapispec.Tags("Agents"),
					oapispec.BearerAuth(),
					oapispec.PathParam("account", "Source account name"),
					oapispec.PathParam("name", "Agent name"),
					oapispec.Body(&handlers.TransferAgentRequest{}),
					oapispec.Response(200, &handlers.TransferAgentResponse{}),
					oapispec.Response(409, &handlers.ErrorResponse{}),
				)
			}

			// Deployment write (deploy/undeploy/restart/trigger)
			api.POST(protected, "/deploy", "Deploy an agent", handlers.DeployAgent(log, agentIndex, accountStore, cfg, deploymentStore, ent, queue),
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
			api.POST(protected, "/undeploy", "Undeploy an agent", handlers.UndeployAgent(log, agentIndex, accountStore, cfg, deploymentStore, queue),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.Body(&deployment.UndeployRequest{}),
				oapispec.Response(202, &handlers.UndeployResponseAlias{}),
			)
			api.GET(protected, "/deployments/:id/status", "Get deployment status", handlers.GetDeploymentStatus(log, accountStore, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
			)
			api.POST(protected, "/deployments/:id/wakeup", "Wake up a scaled-down deployment", handlers.WakeUpDeployment(log, accountStore, deploymentStore, queue),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(202, nil),
			)
			api.POST(protected, "/deployments/:id/rollback", "Rollback to a previous revision", handlers.RollbackDeployment(log, accountStore, deploymentStore, queue),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment ID"),
				oapispec.Response(202, nil),
			)
			api.POST(protected, "/deployments/:id/pods/:pod/restart", "Restart a pod", handlers.RestartPod(log, accountStore, cfg, k8sClient, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.PathParam("pod", "Pod name"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.RestartPodResponse{}),
			)
			api.POST(protected, "/deployments/:id/ingestion/:ingestion/trigger", "Trigger an ingestion job", handlers.TriggerIngestion(log, agentIndex, accountStore, k8sClient, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.PathParam("ingestion", "Ingestion name"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.TriggerIngestionResponse{}),
			)

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
			api.GET(protected, "/deployments", "List deployments", handlers.ListDeployments(log, accountStore, cfg, k8sClient, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.ListDeploymentsResponse{}),
			)
			api.GET(protected, "/deployments/:id/logs", "Get deployment logs", handlers.GetDeploymentLogs(log, accountStore, cfg, k8sClient, deploymentStore, lokiClient),
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
			api.GET(protected, "/deployments/:id/configmap/:cmname", "Get ConfigMap data", handlers.GetConfigMapData(log, accountStore, cfg, k8sClient, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.PathParam("cmname", "ConfigMap name"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.ConfigMapDataResponse{}),
			)
			api.GET(protected, "/deployments/:id/secret/:secretname/keys", "Get Secret key names", handlers.GetSecretKeys(log, accountStore, cfg, k8sClient, deploymentStore),
				oapispec.Tags("Deployments"),
				oapispec.BearerAuth(),
				oapispec.PathParam("id", "Deployment namespace"),
				oapispec.PathParam("secretname", "Secret name"),
				oapispec.QueryParam("account", "Account name", true),
				oapispec.Response(200, &handlers.SecretKeysResponse{}),
			)
			api.GET(protected, "/agents/:account/:name/observability/metrics", "Get agent metrics", handlers.GetObservabilityMetrics(log, cfg, deploymentStore, accountStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("name", "Agent name"),
				oapispec.QueryParam("start_time", "Start time (RFC3339)", false),
				oapispec.QueryParam("end_time", "End time (RFC3339)", false),
				oapispec.QueryParam("interval", "Bucket interval in minutes (default 60)", false),
				oapispec.Response(200, &handlers.ObservabilityMetricsResponse{}),
			)
			api.GET(protected, "/agents/:account/:name/observability/summary", "Get observability summary", handlers.GetObservabilitySummary(log, cfg, deploymentStore, accountStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("name", "Agent name"),
				oapispec.QueryParam("start_time", "Start time (RFC3339)", false),
				oapispec.QueryParam("end_time", "End time (RFC3339)", false),
				oapispec.Response(200, &handlers.ObservabilitySummaryResponse{}),
			)
			api.GET(protected, "/agents/:account/:name/observability/traces", "Get agent traces", handlers.GetObservabilityTraces(log, cfg, deploymentStore, accountStore),
				oapispec.Tags("Observability"),
				oapispec.BearerAuth(),
				oapispec.PathParam("account", "Account name"),
				oapispec.PathParam("name", "Agent name"),
				oapispec.QueryParam("start_time", "Start time (RFC3339)", false),
				oapispec.QueryParam("end_time", "End time (RFC3339)", false),
				oapispec.QueryParam("limit", "Page size (default 50)", false),
				oapispec.QueryParam("offset", "Pagination offset (default 0)", false),
				oapispec.QueryParam("status", "Filter by status", false),
				oapispec.Response(200, &handlers.ObservabilityTracesResponse{}),
			)
		}

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

	if creds == nil {
		log.Warn("Admin gRPC disabled — mTLS not configured (set ADMIN_GRPC_CERT_FILE, ADMIN_GRPC_KEY_FILE, ADMIN_GRPC_CA_FILE)")
		return nil, nil
	}
	var opts []grpc.ServerOption
	opts = append(opts, grpc.Creds(creds))

	grpcSrv := grpc.NewServer(opts...)
	adminv1.RegisterAdminServiceServer(grpcSrv, adminSrv)

	lis, err := net.Listen("tcp", ":"+port)
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

// startConnectGRPCServer starts the connect gRPC server over QUIC for CLI device connections.
// Returns nil, nil if the port is empty (disabled) or TLS certs are not configured.
func startConnectGRPCServer(
	log *logger.Logger,
	cfg *config.Config,
	devStore *devicestore.Store,
	accountStore *account.AccountStore,
	jwtValidator *auth.JWTValidator,
) (*grpc.Server, *connectgrpc.Server, error) {
	port := cfg.ConnectGRPC.Port
	if port == "" {
		return nil, nil, nil
	}

	// TLS certs provided by platform via fleet-tls K8s secret
	certFile := cfg.ConnectGRPC.CertFile
	keyFile := cfg.ConnectGRPC.KeyFile
	if certFile == "" || keyFile == "" {
		log.Warn("Connect gRPC disabled — TLS not configured (set FLEET_TLS_CERT_PATH, FLEET_TLS_KEY_PATH)")
		return nil, nil, nil
	}

	// Load TLS config for QUIC (QUIC mandates TLS 1.3)
	tlsCert, err := connectgrpc.LoadTLSCert(certFile, keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("connect gRPC TLS: %w", err)
	}

	tlsConf := connectgrpc.NewTLSConfig(tlsCert)

	// Create QUIC listener
	lis, err := connectgrpc.ListenQUIC(":"+port, tlsConf, log)
	if err != nil {
		return nil, nil, fmt.Errorf("connect gRPC QUIC listen: %w", err)
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
		log.Info("Connect gRPC server listening (QUIC/UDP)", "port", port)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Error("Connect gRPC server error", "error", err)
		}
	}()

	return grpcSrv, srv, nil
}
