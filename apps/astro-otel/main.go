// Command astro-otel is the OTLP ingest service for local AI coding tools.
// It authenticates a per-account ingest key against astro-server's database,
// then forwards traces to the account's Langfuse project and metrics to
// VictoriaMetrics. It runs alongside astro-server and shares its Postgres.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	_ "github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-otel/internal/config"
	"github.com/astropods/astro/apps/astro-otel/internal/envelope"
	"github.com/astropods/astro/apps/astro-otel/internal/ingest"
	"github.com/astropods/astro/apps/astro-otel/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "error", err)
		os.Exit(1)
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Error("open db", "error", err)
		os.Exit(1)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Error("ping db", "error", err)
		os.Exit(1)
	}

	// KMS is used to decrypt per-account Langfuse secret keys. If AWS config
	// isn't available (e.g. local dev without credentials), continue without
	// it — only plaintext-stored creds resolve, which is the dev convention.
	var kmsClient envelope.KMSClient
	if awsCfg, err := awsconfig.LoadDefaultConfig(context.Background()); err != nil {
		log.Warn("AWS config unavailable; KMS-encrypted Langfuse creds will not decrypt", "error", err)
	} else {
		kmsClient = kms.NewFromConfig(awsCfg)
	}

	st := store.New(db, kmsClient, cfg.TokenCacheTTL)
	h := ingest.New(st, cfg, log)

	mux := http.NewServeMux()
	h.Register(mux)

	srv := &http.Server{
		Addr:              cfg.Host + ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("astro-otel listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "error", err)
	}
	_ = db.Close()
}
