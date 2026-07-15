// Package config loads astro-otel's runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds astro-otel's runtime configuration.
type Config struct {
	// Port is the HTTP listen port for the OTLP receiver. Default 4318 (the
	// OTLP/HTTP convention), matching what developer machines are pointed at.
	Port string
	// Host is the bind address. Default 0.0.0.0.
	Host string

	// DatabaseURL is astro-server's Postgres — read for otel_ingest_tokens
	// (auth) and account_langfuse (per-account trace credentials). Shared DB,
	// read-only access on astro-otel's part.
	DatabaseURL string

	// LangfuseOTLPEndpoint is the base OTLP URL of Langfuse (e.g.
	// http://<langfuse-vpce>:3000/api/public/otel). Traces are POSTed to
	// <base>/v1/traces with per-account Basic auth.
	LangfuseOTLPEndpoint string

	// VMOTLPEndpoint is VictoriaMetrics' native OTLP push URL (e.g.
	// http://victoria-metrics-server.monitoring.svc.cluster.local:8428/opentelemetry/api/v1/push).
	VMOTLPEndpoint string

	// TokenCacheTTL bounds how long a validated key→account (and its resolved
	// Langfuse credential) is cached before re-reading the DB. Also the upper
	// bound on how long a revoked key keeps working.
	TokenCacheTTL time.Duration

	// RedactAttributes, when true, strips prompt/completion/tool-body
	// attributes before forwarding (defense in depth). Off by default —
	// managed settings already keep that content off at the source, so this is
	// an opt-in belt-and-suspenders control, not the primary guarantee.
	RedactAttributes bool
}

// Load reads configuration from the environment.
func Load() (*Config, error) {
	cfg := &Config{
		Port:                 getEnv("PORT", "4318"),
		Host:                 getEnv("HOST", "0.0.0.0"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		LangfuseOTLPEndpoint: os.Getenv("LANGFUSE_OTLP_ENDPOINT"),
		VMOTLPEndpoint:       os.Getenv("VM_OTLP_ENDPOINT"),
		TokenCacheTTL:        getDuration("TOKEN_CACHE_TTL", 60*time.Second),
		RedactAttributes:     getBool("OTEL_REDACT_ATTRIBUTES", false),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.LangfuseOTLPEndpoint == "" && cfg.VMOTLPEndpoint == "" {
		return nil, fmt.Errorf("at least one of LANGFUSE_OTLP_ENDPOINT or VM_OTLP_ENDPOINT must be set")
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func getBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
