package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/postman/astro/apps/astro-server/handlers"
	"github.com/postman/astro/apps/astro-server/internal/account"
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
	defer db.Close()

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

	// Create HTTP server with timeouts
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine
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

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	// Attempt graceful shutdown
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

	// CLI install script (no files needed — just returns a shell script)
	router.GET("/install", handlers.CLIInstallScript(cfg))

	// CLI binary download (HEAD for version check, GET for download)
	if cfg.Server.CLIDir != "" {
		router.GET("/download/:name", handlers.CLIDownload(cfg))
		router.HEAD("/download/:name", handlers.CLIDownload(cfg))
	}

	// Serve static frontend assets if configured
	if cfg.Server.StaticDir != "" {
		setupStaticFiles(router, log, cfg.Server.StaticDir)
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
		v1.GET("/accounts/:account", handlers.GetAccount(log, accountStore))
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
		}

		// Admin endpoints (require basic auth)
		admin := v1.Group("/admin")
		if cfg.Admin.Enabled {
			admin.Use(middleware.BasicAuth(middleware.BasicAuthConfig{
				Username: cfg.Admin.Username,
				Password: cfg.Admin.Password,
				Realm:    "Astro Admin",
			}))
			log.Info("Admin API enabled with basic auth")
		}
		{
			admin.GET("/cluster/status", handlers.ClusterStatus(log, k8sClient))
			admin.GET("/images", handlers.ListImages(log, cfg.Deployment.AWSRegion, cfg.Deployment.Environment))
		}
	}
}

// setupStaticFiles configures static file serving for the SPA frontend
func setupStaticFiles(router *gin.Engine, log *logger.Logger, staticDir string) {
	// Verify static directory exists
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		log.Warn("Static directory does not exist, skipping static file serving", "path", staticDir)
		return
	}

	log.Info("Serving static files", "path", staticDir)

	// Serve static assets (js, css, images, etc.)
	router.Static("/assets", filepath.Join(staticDir, "assets"))

	// Serve other static files at root (favicon, robots.txt, etc.)
	router.StaticFile("/favicon.ico", filepath.Join(staticDir, "favicon.ico"))
	router.StaticFile("/robots.txt", filepath.Join(staticDir, "robots.txt"))

	// SPA fallback: serve index.html for all non-API, non-auth, non-static routes
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Don't serve index.html for API or auth routes
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/auth/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		// Serve index.html for SPA routing
		c.File(filepath.Join(staticDir, "index.html"))
	})
}
