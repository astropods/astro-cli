package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/postman/astro/apps/astro-server/handlers"
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/config"
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
	indexPath := filepath.Join(cfg.Deployment.ArtifactDir, "agent-index.db")
	agentIndex, err := agentindex.NewIndex(indexPath)
	if err != nil {
		log.Error("Failed to create agent index", "error", err)
		os.Exit(1)
	}
	log.Info("Agent index initialized", "path", indexPath)

	// Register routes
	setupRoutes(router, log, agentIndex, cfg)

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

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	log.Info("Server stopped gracefully")
}

// setupRoutes configures all application routes
func setupRoutes(router *gin.Engine, log *logger.Logger, agentIndex *agentindex.Index, cfg *config.Config) {
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
			protected.GET("/agents/:name/:version/credentials", handlers.GetAgentCredentials(log, agentIndex))
			protected.POST("/agents/register", handlers.RegisterAgent(log, agentIndex))
			protected.POST("/deploy", handlers.DeployAgent(log, agentIndex, cfg))
			protected.POST("/undeploy", handlers.UndeployAgent(log, agentIndex, cfg))
		}
	}
}
