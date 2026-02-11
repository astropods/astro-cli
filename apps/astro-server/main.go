package main

import (
	"context"
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
	"github.com/postman/astro/apps/astro-server/handlers"
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/config"
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

	// Initialize agent index for tracking published agents
	agentIndex, err := agentindex.NewIndex(cfg.Database.URL)
	if err != nil {
		log.Error("Failed to create agent index", "error", err)
		os.Exit(1)
	}
	log.Info("Agent index initialized")

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
	setupRoutes(router, log, agentIndex, cfg, probeHandler, k8sClient)

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
func setupRoutes(router *gin.Engine, log *logger.Logger, agentIndex *agentindex.Index, cfg *config.Config, probeHandler *handlers.ProbeHandler, k8sClient k8s.ClusterClient) {
	// Kubernetes-style health probe endpoints (at root, no middleware)
	router.GET("/livez", probeHandler.Livez())
	router.GET("/readyz", probeHandler.Readyz())
	router.GET("/healthz", probeHandler.Healthz())

	// CLI binary download (how-to lives on SPA /dev)
	if cfg.Server.CLIDir != "" {
		router.GET("/download/:name", handlers.CLIDownload(cfg))
	}

	// Serve static frontend assets if configured
	if cfg.Server.StaticDir != "" {
		setupStaticFiles(router, log, cfg.Server.StaticDir)
	}

	// Setup authentication if enabled
	var authMw *middleware.AuthMiddleware
	if cfg.Auth.Enabled {
		authHandler := handlers.NewAuthHandler(log, cfg)

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
		authMw = middleware.NewAuthMiddleware(
			log,
			cfg,
			authHandler.GetSessionManager(),
			authHandler.GetJWTValidator(),
		)

		log.Info("Authentication enabled", "provider", "WorkOS")
	}

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Health check endpoint (public)
		v1.GET("/health", handlers.HealthCheck(log))

		// Readiness check endpoint (public)
		v1.GET("/ready", handlers.ReadinessCheck(log))

		// Agent registry endpoints (public read, protected write)
		v1.GET("/agents", handlers.ListAgents(log, agentIndex))
		v1.GET("/agents/:name", handlers.GetAgent(log, agentIndex))
		v1.GET("/agents/:name/:version", handlers.GetAgentVersion(log, agentIndex))

		// Protected endpoints (require authentication when enabled)
		protected := v1.Group("")
		if authMw != nil {
			protected.Use(authMw.RequireAuth())
		}
		{
			protected.GET("/agents/:name/:version/config", handlers.GetAgentConfig(log, agentIndex))
			protected.POST("/agents/register", handlers.RegisterAgent(log, agentIndex))
			protected.POST("/deploy", handlers.DeployAgent(log, agentIndex, cfg, k8sClient))
			protected.POST("/undeploy", handlers.UndeployAgent(log, agentIndex, cfg, k8sClient))
			protected.GET("/deployments", handlers.ListDeployments(log, cfg, k8sClient))
			protected.GET("/deployments/:name/:version/logs", handlers.GetDeploymentLogs(log, cfg, k8sClient))
			protected.POST("/deployments/:name/:version/ingestion/:ingestion/trigger", handlers.TriggerIngestion(log, agentIndex, k8sClient))
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
			admin.GET("/images", handlers.ListImages(log, cfg.Deployment.AWSRegion))
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
