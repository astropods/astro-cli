package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/astropods/astro/apps/astro-registry/handlers"
	"github.com/astropods/astro/apps/astro-registry/internal/account"
	"github.com/astropods/astro/apps/astro-registry/internal/config"
	"github.com/astropods/astro/apps/astro-registry/internal/logger"
	"github.com/astropods/astro/apps/astro-registry/internal/middleware"
	"github.com/astropods/astro/apps/astro-registry/internal/registry"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
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
	log.Info("Starting astro-registry",
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

	// Set trusted proxies
	if len(cfg.Security.TrustedProxies) > 0 {
		if err := router.SetTrustedProxies(cfg.Security.TrustedProxies); err != nil {
			log.Error("Failed to set trusted proxies", "error", err)
		}
	}

	// Initialize database connection
	db, err := sql.Open("postgres", cfg.Database.URL)
	if err != nil {
		log.Error("Failed to open database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close() //nolint:errcheck

	if err := db.Ping(); err != nil {
		log.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	log.Info("Database connected")

	// Initialize membership checker
	mc := account.NewMembershipChecker(db)

	// Initialize ECR auth provider
	ecrAuth := registry.NewECRAuthProvider(cfg.Registry.AWSRegion)
	log.Info("ECR auth provider initialized", "region", cfg.Registry.AWSRegion)

	// Setup authentication middleware
	authMw := middleware.NewAuthMiddleware(log, cfg)
	log.Info("Authentication enabled", "jwks_endpoint", cfg.Auth.JWKSEndpoint)

	// Initialize probe handler for K8s health checks
	probeHandler := handlers.NewProbeHandler(log, ecrAuth)

	// Register routes
	setupRoutes(router, log, cfg, authMw, ecrAuth, probeHandler, mc)

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
func setupRoutes(router *gin.Engine, log *logger.Logger, cfg *config.Config, authMw *middleware.AuthMiddleware, ecrAuth *registry.ECRAuthProvider, probeHandler *handlers.ProbeHandler, mc *account.MembershipChecker) {
	// Kubernetes-style health probe endpoints (at root, no middleware)
	router.GET("/livez", probeHandler.Livez())
	router.GET("/readyz", probeHandler.Readyz())
	router.GET("/healthz", probeHandler.Healthz())

	// Registry proxy configuration
	proxyCfg := handlers.RegistryProxyConfig{
		RegistryURL:       cfg.Registry.URL,
		Environment:       cfg.Registry.Environment,
		AuthProvider:      ecrAuth,
		Logger:            log,
		MembershipChecker: mc,
	}

	// Docker Registry V2 API routes
	v2 := router.Group("/v2")
	v2.Use(authMw.RequireAuth())
	{
		// All registry operations (including version check at root)
		v2.Any("/*path", handlers.RegistryProxy(proxyCfg))
	}

	log.Info("Registry proxy enabled", "registry", cfg.Registry.URL)
}
