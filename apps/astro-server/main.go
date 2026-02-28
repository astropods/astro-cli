package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	adminv1 "github.com/postman/astro/packages/astro-proto/admin/v1"
	"google.golang.org/grpc"

	"github.com/postman/astro/apps/astro-server/handlers"
	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/admingrpc"
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/config"
	"github.com/postman/astro/apps/astro-server/internal/deploymentstore"
	"github.com/postman/astro/apps/astro-server/internal/k8s"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"github.com/postman/astro/apps/astro-server/internal/middleware"
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
		"port", cfg.Server.Port,
	)

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

	// Initialize stores with shared DB
	agentIndex := agentindex.NewIndexWithDB(db)
	accountStore := account.NewAccountStore(db)
	deploymentStore := deploymentstore.NewStore(db)
	log.Info("Agent index and account store initialized")

	// Initialize Kubernetes client (EKS for production, local for development)
	var k8sClient k8s.ClusterClient
	clientMode := k8s.ClientMode(cfg.Deployment.K8sClientMode)
	log.Info("Initializing Kubernetes client",
		"mode", string(clientMode),
	)
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
		// Test connectivity and get server version
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

	// Initialize probe handler for K8s health checks
	probeHandler := handlers.NewProbeHandler(log, agentIndex, k8sClient)

	// Register routes
	setupRoutes(router, log, agentIndex, accountStore, deploymentStore, cfg, probeHandler, k8sClient)

	// Start admin gRPC server
	grpcServer, grpcErr := startAdminGRPCServer(log, cfg, deploymentStore, k8sClient, db)
	if grpcErr != nil {
		log.Error("Failed to start admin gRPC server", "error", grpcErr)
		os.Exit(1)
	}

	// Create HTTP server with timeouts
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	// Start HTTP server in a goroutine
	go func() {
		log.Info("Server listening", "address", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Mark as not ready to stop receiving new traffic
	probeHandler.SetReady(false)

	// Graceful shutdown of gRPC server
	if grpcServer != nil {
		grpcServer.GracefulStop()
	}

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	// Attempt graceful shutdown of HTTP server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	log.Info("Server stopped gracefully")
}

// setupRoutes configures all application routes
func setupRoutes(router *gin.Engine, log *logger.Logger, agentIndex *agentindex.Index, accountStore *account.AccountStore, deploymentStore *deploymentstore.Store, cfg *config.Config, probeHandler *handlers.ProbeHandler, k8sClient k8s.ClusterClient) {
	// Kubernetes-style health probe endpoints (at root, no middleware)
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

	// Auth routes (no auth required)
	auth := router.Group("/auth")
	{
		auth.GET("/login", authHandler.Login())
		auth.GET("/callback", authHandler.Callback())
		auth.GET("/logout", authHandler.Logout())
		auth.GET("/me", authHandler.Me())
		auth.POST("/refresh", authHandler.Refresh())
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
		v1.GET("/health", handlers.HealthCheck(log))

		// Readiness check endpoint (public)
		v1.GET("/ready", handlers.ReadinessCheck(log))

		// Agent registry endpoints (public read, with optional auth for visibility)
		v1.GET("/agents", handlers.ListAgents(log, agentIndex, accountStore))
		agentDetail := v1.Group("")
		agentDetail.Use(authMw.OptionalAuth())
		{
			agentDetail.GET("/agents/:account/:name", handlers.GetAgent(log, agentIndex, accountStore))
		}
		// Account endpoints (public read)
		v1.GET("/accounts/:account", handlers.GetAccount(log, accountStore, authHandler.GetWorkOSClient()))
		v1.GET("/accounts/check/:name", handlers.CheckAccountName(log, accountStore))

		// Protected endpoints (require authentication)
		protected := v1.Group("")
		protected.Use(authMw.RequireAuth())
		{
			// Profile
			protected.GET("/me", handlers.GetProfile(log, accountStore, agentIndex))

			// Account management
			protected.POST("/accounts", handlers.CreateAccount(log, accountStore))

			// Account-scoped routes with role check
			accountRoutes := protected.Group("/accounts/:account")
			accountRoutes.Use(middleware.ResolveAccount(accountStore))
			accountRoutes.Use(middleware.RequireAccountRole(accountStore, "owner"))
			{
				accountRoutes.PUT("", handlers.RenameAccount(log, accountStore))
			}

			// Agent config (protected read, resolves latest build)
			protected.GET("/agents/:account/:name/config", handlers.GetAgentConfig(log, agentIndex, accountStore))

			// Deployment template generation
			protected.GET("/agents/:account/:name/deployment-template", handlers.GetDeploymentTemplate(log, agentIndex, accountStore, cfg))

			// Agent registration and publishing (account-scoped, requires write access)
			protected.POST("/agents/:account/:name/register", handlers.RegisterAgent(log, agentIndex, accountStore))
			protected.POST("/agents/:account/:name/publish", handlers.PublishAgent(log, agentIndex, accountStore))

			// Deploy/undeploy
			protected.POST("/deploy", handlers.DeployAgent(log, agentIndex, accountStore, cfg, k8sClient, deploymentStore))
			protected.POST("/deploy/validate", handlers.ValidateDeployment(log, agentIndex, accountStore, cfg))
			protected.POST("/undeploy", handlers.UndeployAgent(log, agentIndex, accountStore, cfg, k8sClient, deploymentStore))

			// Deployment spec retrieval
			protected.GET("/agents/:account/:name/deployment", handlers.GetActiveDeploymentSpec(log, accountStore, deploymentStore))
			protected.GET("/agents/:account/:name/deployment/history", handlers.GetDeploymentHistory(log, accountStore, deploymentStore))
			protected.GET("/deployments", handlers.ListDeployments(log, accountStore, cfg, k8sClient))
			protected.GET("/deployments/:namespace/logs", handlers.GetDeploymentLogs(log, accountStore, cfg, k8sClient))
			protected.POST("/deployments/:namespace/pods/:pod/restart", handlers.RestartPod(log, accountStore, cfg, k8sClient))
			protected.GET("/deployments/:namespace/configmap/:cmname", handlers.GetConfigMapData(log, accountStore, cfg, k8sClient))
			protected.GET("/deployments/:namespace/secret/:secretname/keys", handlers.GetSecretKeys(log, accountStore, cfg, k8sClient))
			protected.POST("/deployments/:namespace/ingestion/:ingestion/trigger", handlers.TriggerIngestion(log, agentIndex, accountStore, k8sClient))

			// Observability endpoints
			protected.GET("/agents/:account/:name/observability/metrics", handlers.GetObservabilityMetrics(log, cfg, deploymentStore, accountStore))
			protected.GET("/agents/:account/:name/observability/summary", handlers.GetObservabilitySummary(log, cfg, deploymentStore, accountStore))
			protected.GET("/agents/:account/:name/observability/traces", handlers.GetObservabilityTraces(log, cfg, deploymentStore, accountStore))
		}

	}
}

// startAdminGRPCServer starts the admin gRPC server and returns it for graceful shutdown.
// Returns nil, nil if the port is empty (disabled).
func startAdminGRPCServer(
	log *logger.Logger,
	cfg *config.Config,
	deployStore *deploymentstore.Store,
	k8sClient k8s.ClusterClient,
	db *sql.DB,
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
	adminv1.RegisterAdminServiceServer(grpcSrv, admingrpc.New(
		log,
		deployStore,
		k8sClient,
		db,
		cfg.Deployment.AWSRegion,
		cfg.Deployment.Environment,
	))

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
