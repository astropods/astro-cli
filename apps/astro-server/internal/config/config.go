package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the application configuration
type Config struct {
	RunMode              string // SERVER_MODE: "all" (default), "api", or "worker"
	Server               ServerConfig
	Log                  LogConfig
	Security             SecurityConfig
	Deployment           DeploymentConfig
	Auth                 AuthConfig
	Database             DatabaseConfig
	AdminGRPC            AdminGRPCConfig
	FleetGRPC            FleetGRPCConfig
	Avatar               AvatarConfig
	OpenMeterURL         string // OPENMETER_URL — base URL for OpenMeter API
	OpenMeterDefaultPlan string // OPENMETER_DEFAULT_PLAN — plan key to auto-subscribe new accounts (empty = disabled)
	OpenMeterEnforce     bool   // OPENMETER_ENFORCE — enable entitlement enforcement (default false)
	S3                   S3Config
	GitHub               GitHubConfig
	Slack                SlackConfig
	LokiURL              string // LOKI_URL — Loki base URL for log queries (e.g. http://<nlb-dns>:3100); falls back to K8s pod logs if unset
	DeploymentLogBackend string // DEPLOYMENT_LOG_BACKEND — "loki" or "k8s"; defaults to "loki" if LOKI_URL is set, otherwise "k8s"
	PrometheusURL        string // PROMETHEUS_URL — Prometheus base URL for metric queries (e.g. http://prometheus:9090)
	RedisURL             string // REDIS_URL — enables K8s state caching when set (e.g. redis://localhost:6379)
}

// RunAPI returns true if this instance should run the HTTP/gRPC API servers.
func (c *Config) RunAPI() bool {
	return c.RunMode == "all" || c.RunMode == "api"
}

// RunWorker returns true if this instance should run background workers (events consumer).
func (c *Config) RunWorker() bool {
	return c.RunMode == "all" || c.RunMode == "worker"
}

// AdminGRPCConfig holds admin gRPC server configuration.
// Cert/Key/CA values accept file paths or inline PEM (auto-detected by "-----BEGIN" prefix).
type AdminGRPCConfig struct {
	Port         string // ADMIN_GRPC_PORT, default "9091" (optional — gRPC server disabled if empty)
	CertFile     string // ADMIN_GRPC_CERT_FILE — file path or inline PEM (optional — no TLS if empty)
	KeyFile      string // ADMIN_GRPC_KEY_FILE  — file path or inline PEM
	CAFile       string // ADMIN_GRPC_CA_FILE   — file path or inline PEM
	OpenMeterURL string // OPENMETER_URL — base URL for OpenMeter API proxying
}

// FleetGRPCConfig holds Fleet gRPC server configuration (QUIC transport, JWT auth).
// TLS certs are provided by the platform via the fleet-tls K8s secret mounted at /etc/fleet-tls/.
type FleetGRPCConfig struct {
	Port     string // FLEET_GRPC_PORT, default "9092" (UDP/QUIC — disabled if empty)
	CertFile string // FLEET_TLS_CERT_PATH — TLS cert path (default /etc/fleet-tls/tls.crt)
	KeyFile  string // FLEET_TLS_KEY_PATH  — TLS key path (default /etc/fleet-tls/tls.key)
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	URL string // DATABASE_URL connection string (postgres://user:pass@host:port/dbname?sslmode=disable)
}

// AuthConfig holds WorkOS authentication configuration
type AuthConfig struct {
	WorkOSAPIKey   string
	WorkOSClientID string
	RedirectURI    string
	FrontendURL    string
	CookieName     string
	CookiePassword string
	CookieDomain   string
	CookieSecure   bool
	CookieSameSite string // "Strict", "Lax", or "None"
	CookieMaxAge   time.Duration
	SessionMaxAge  time.Duration
	JWTIssuer      string
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	Port            string
	Host            string
	Mode            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	DownloadBaseURL string // Base URL for CLI download CDN, e.g. https://download.astropods.ai (used in /install script)
	MinCLIVersion   string // MIN_CLI_VERSION — minimum CLI version allowed for push (optional, e.g. "0.3.7")
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string
	Format string
}

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	AllowedOrigins    []string
	TrustedProxies    []string
	DeployTokenSecret string // DEPLOY_TOKEN_SECRET — HMAC secret for signing per-deployment tokens injected into messaging containers
}

// AvatarConfig holds avatar/profile-picture configuration.
type AvatarConfig struct {
	S3Bucket  string // ASSETS_BUCKET — S3 bucket for avatar storage (empty = check LocalDir)
	LocalDir  string // ASSETS_LOCAL_DIR — local assets directory path (e.g. "../../assets", for local dev)
	AssetsURL string // ASSETS_CDN_URL — CDN base URL for avatar URLs in API responses
}

// Enabled returns true when avatar storage is configured (either S3 or local).
func (a AvatarConfig) Enabled() bool {
	return a.S3Bucket != "" || a.LocalDir != ""
}

// IsLocal returns true when using filesystem storage instead of S3.
func (a AvatarConfig) IsLocal() bool {
	return a.S3Bucket == "" && a.LocalDir != ""
}

// S3Config holds S3 / S3-compatible storage configuration.
type S3Config struct {
	// S3_ENDPOINT — custom endpoint URL for S3-compatible stores (e.g. http://localhost:9000 for MinIO).
	// Empty in production; the AWS SDK uses the standard S3 endpoint.
	Endpoint string
	// S3_PATH_STYLE — force path-style addressing (required for MinIO and most S3-compatible stores).
	// Automatically set to true when Endpoint is non-empty.
	PathStyle bool
}

// GitHubConfig holds GitHub connection configuration.
type GitHubConfig struct {
	// GITHUB_CALLBACK_URL — base URL for GitHub OAuth callbacks and webhook delivery
	// (e.g. https://astropods.com or an ngrok tunnel in local dev).
	// Falls back to FRONTEND_URL if not set.
	CallbackURL string
	// GITHUB_BUILD_NAMESPACE — K8s namespace for build Jobs (default: as0-builds)
	BuildNamespace string
	// GITHUB_BUILD_SERVICE_ACCOUNT — K8s service account for build Jobs; needs IRSA ECR push permissions in production (default: build-worker)
	BuildServiceAccount string
	// GITHUB_BUILDKIT_CONFIGMAP — ConfigMap in the build namespace containing buildkitd.toml (default: buildkitd-config)
	BuildKitConfigMap string
}

// SlackConfig holds Slack identity-link (raw Slack OAuth) configuration.
//
// We don't use WorkOS Pipes for slack: Pipes hands back a bot token, but
// to discover the linker's *human* slack identity we need a user token
// (slack OAuth's `user_scope`). Pipes' `GetAccessToken` only returns the
// bot token, so we run the OAuth dance ourselves.
type SlackConfig struct {
	// SLACK_CLIENT_ID — slack app credentials from api.slack.com/apps →
	// Basic Information. Required for the OAuth code exchange.
	ClientID string
	// SLACK_CLIENT_SECRET — pair of CLIENT_ID. Used to authenticate the
	// `oauth.v2.access` call from the callback handler. Treat as a secret.
	ClientSecret string
	// SLACK_CALLBACK_URL — base URL the slack OAuth `redirect_uri` is
	// built from (e.g. https://astropods.com or an ngrok tunnel in local
	// dev). The `${base}/api/v1/accounts/:account/slack/callback` URL is
	// built from this value. The slack app must list the same URL under
	// OAuth & Permissions → Redirect URLs. Falls back to FRONTEND_URL.
	CallbackURL string
}

// DeploymentConfig holds deployment-related configuration
type DeploymentConfig struct {
	RegistryURL       string // ECR registry URL (required for eks mode, defaults to "docker.io/library" for local)
	ProxyRegistryHost string // Proxy registry host (e.g., registry.example.com)
	Environment       string // Environment prefix for ECR tenant repos (e.g. "prod", "preview")
	EKSClusterName    string // EKS cluster name (required for eks mode)
	K8sMasterURL      string // K8s API server endpoint (required for eks mode)
	AWSRegion         string // AWS region (optional, auto-detected from IRSA)
	// K8s client mode: "eks" (default) or "local" (Docker Desktop / kind / minikube)
	K8sClientMode  string // K8S_CLIENT_MODE
	KubeconfigPath string // KUBECONFIG path (local mode, defaults to ~/.kube/config)
	KubeContext    string // KUBE_CONTEXT (local mode, defaults to current-context)
	// Ingress configuration for agent workloads (agents.astropods.ai)
	IngressDomain     string // Domain for agent ingress (e.g., agents.astropods.ai)
	ACMCertificateARN string // ACM certificate ARN for HTTPS
	ALBGroupName      string // ALB group name for agent ALB
	// Ingress configuration for ingestion workloads (ingestion.astropods.ai)
	IngestionIngressDomain string // Domain for ingestion webhook ingress (e.g., ingestion.astropods.ai)
	// Knowledge store public host domain (e.g., knowledge.astropods.ai)
	// Public stores get a hostname: {name}.{account}.{KnowledgeDomain}
	KnowledgeDomain       string // KNOWLEDGE_DOMAIN
	IngestionACMCertARN   string // ACM certificate ARN for ingestion wildcard cert
	IngestionALBGroupName string // ALB group name for ingestion ALB (separate from agents)
	// NetworkPolicy isolation: private subnet CIDRs where cluster pods run (comma-separated)
	PodSubnetCIDRs []string // POD_SUBNET_CIDRS
	// KMS envelope encryption for deployment secrets
	KMSKeyARN string // KMS_KEY_ARN — ARN of the KMS key for secret encryption (optional — secrets stripped if unset)
	// Managed provider credentials — injected by the server for managed providers
	ManagedAnthropicAPIKey string // MANAGED_ANTHROPIC_API_KEY — Anthropic API key for the anthropic-managed provider
	// Local dev — inject a messaging URL without a real ingress (e.g. http://localhost:8081)
	MessagingURLOverride string // MESSAGING_URL_OVERRIDE
	// Observability (Langfuse) — direct DB provisioning for per-account projects
	LangfuseDBURL      string   // LANGFUSE_DB_URL — Postgres connection string for Langfuse's database
	LangfuseSalt       string   // LANGFUSE_SALT — must match Langfuse's SALT env var
	LangfuseOrgID      string   // LANGFUSE_ORG_ID — the single org ID in our Langfuse instance
	LangfuseBaseURL    string   // LANGFUSE_BASE_URL — Langfuse instance URL
	LangfuseBaseURLExt string   // LANGFUSE_BASE_URL_EXT — external Langfuse URL for collector (overrides LANGFUSE_BASE_URL)
	LangfuseVPCEIPs    []string // LANGFUSE_VPCE_IPS — VPC endpoint IPs for NetworkPolicy egress rules
	// Messaging OIDC authentication — ALB authenticate-oidc action for messaging ingress
	MessagingOIDCIssuer           string // MESSAGING_OIDC_ISSUER — WorkOS OIDC issuer URL
	MessagingOIDCAuthEndpoint     string // MESSAGING_OIDC_AUTH_ENDPOINT — authorization endpoint
	MessagingOIDCTokenEndpoint    string // MESSAGING_OIDC_TOKEN_ENDPOINT — token endpoint
	MessagingOIDCUserInfoEndpoint string // MESSAGING_OIDC_USERINFO_ENDPOINT — userinfo endpoint
	MessagingOIDCClientID         string // MESSAGING_OIDC_CLIENT_ID — OIDC client ID
	MessagingOIDCClientSecret     string // MESSAGING_OIDC_CLIENT_SECRET — OIDC client secret
	MessagingOIDCSessionTimeout   int    // MESSAGING_OIDC_SESSION_TIMEOUT — session duration in seconds (default 3600)
	// PrivateLink automation — managed cluster VPC where VPC endpoints are created at runtime
	PrivateLinkVpcID     string   // PRIVATELINK_VPC_ID — managed cluster VPC ID (empty = PrivateLink disabled)
	PrivateLinkSubnetIDs []string // PRIVATELINK_SUBNET_IDS — comma-separated private subnet IDs
	PrivateLinkSGID      string   // PRIVATELINK_SG_ID — security group for PrivateLink endpoints (broad HTTPS egress; fine-grained access via K8s NetworkPolicy)
	// Template signing — HMAC key for signing deployment templates so the deploy
	// endpoint can verify them without re-generating. Auto-generated at startup if
	// not set. For multi-replica deployments behind a load balancer, set a shared key.
	TemplateSigningKey []byte // TEMPLATE_SIGNING_KEY (hex-encoded; auto-generated if empty)
}

// Load loads configuration from environment variables with defaults
func Load() (*Config, error) {
	cfg := &Config{
		RunMode: getEnv("SERVER_MODE", "all"),
		Server: ServerConfig{
			Port:            getEnv("PORT", "8080"),
			Host:            getEnv("HOST", "0.0.0.0"),
			Mode:            getEnv("GIN_MODE", "release"),
			ReadTimeout:     getEnvDuration("READ_TIMEOUT", 10*time.Second),
			WriteTimeout:    getEnvDuration("WRITE_TIMEOUT", 10*time.Second),
			ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
			DownloadBaseURL: getEnv("DOWNLOAD_BASE_URL", ""),
			MinCLIVersion:   getEnv("MIN_CLI_VERSION", ""),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "text"),
		},
		Security: SecurityConfig{
			AllowedOrigins:    getEnvSlice("ALLOWED_ORIGINS", []string{"*"}),
			TrustedProxies:    getEnvSlice("TRUSTED_PROXIES", []string{}),
			DeployTokenSecret: getEnv("DEPLOY_TOKEN_SECRET", DevDeployTokenSecret),
		},
		Deployment: DeploymentConfig{
			RegistryURL:                   getEnv("REGISTRY_URL", ""),
			ProxyRegistryHost:             getEnv("PROXY_REGISTRY_HOST", ""),
			Environment:                   getEnv("ENVIRONMENT", ""),
			EKSClusterName:                getEnv("EKS_CLUSTER_NAME", ""),
			K8sMasterURL:                  getEnv("K8S_MASTER_URL", ""),
			AWSRegion:                     getEnv("AWS_REGION", ""),
			K8sClientMode:                 getEnv("K8S_CLIENT_MODE", "eks"),
			KubeconfigPath:                getEnv("KUBECONFIG", ""),
			KubeContext:                   getEnv("KUBE_CONTEXT", ""),
			IngressDomain:                 getEnv("INGRESS_DOMAIN", ""),
			ACMCertificateARN:             getEnv("ACM_CERTIFICATE_ARN", ""),
			ALBGroupName:                  getEnv("ALB_GROUP_NAME", "astro-agents"),
			IngestionIngressDomain:        getEnv("INGESTION_INGRESS_DOMAIN", ""),
			IngestionACMCertARN:           getEnv("INGESTION_ACM_CERTIFICATE_ARN", ""),
			IngestionALBGroupName:         getEnv("INGESTION_ALB_GROUP_NAME", ""),
			KnowledgeDomain:               getEnv("KNOWLEDGE_DOMAIN", ""),
			PodSubnetCIDRs:                getEnvSlice("POD_SUBNET_CIDRS", nil),
			KMSKeyARN:                     getEnv("KMS_KEY_ARN", ""),
			ManagedAnthropicAPIKey:        getEnv("MANAGED_ANTHROPIC_API_KEY", ""),
			MessagingURLOverride:          getEnv("MESSAGING_URL_OVERRIDE", ""),
			LangfuseDBURL:                 getEnv("LANGFUSE_DB_URL", ""),
			LangfuseSalt:                  getEnv("LANGFUSE_SALT", ""),
			LangfuseOrgID:                 getEnv("LANGFUSE_ORG_ID", "astro"),
			LangfuseBaseURL:               getEnv("LANGFUSE_BASE_URL", ""),
			LangfuseBaseURLExt:            getEnv("LANGFUSE_BASE_URL_EXT", ""),
			LangfuseVPCEIPs:               getEnvSlice("LANGFUSE_VPCE_IPS", nil),
			MessagingOIDCIssuer:           getEnv("MESSAGING_OIDC_ISSUER", ""),
			MessagingOIDCAuthEndpoint:     getEnv("MESSAGING_OIDC_AUTH_ENDPOINT", ""),
			MessagingOIDCTokenEndpoint:    getEnv("MESSAGING_OIDC_TOKEN_ENDPOINT", ""),
			MessagingOIDCUserInfoEndpoint: getEnv("MESSAGING_OIDC_USERINFO_ENDPOINT", ""),
			MessagingOIDCClientID:         getEnv("MESSAGING_OIDC_CLIENT_ID", ""),
			MessagingOIDCClientSecret:     getEnv("MESSAGING_OIDC_CLIENT_SECRET", ""),
			MessagingOIDCSessionTimeout:   getEnvInt("MESSAGING_OIDC_SESSION_TIMEOUT", 3600),
			PrivateLinkVpcID:              getEnv("PRIVATELINK_VPC_ID", ""),
			PrivateLinkSubnetIDs:          getEnvSlice("PRIVATELINK_SUBNET_IDS", nil),
			PrivateLinkSGID:               getEnv("PRIVATELINK_SG_ID", ""),
			TemplateSigningKey:            loadSigningKey(),
		},
		Auth: AuthConfig{
			WorkOSAPIKey:   getEnv("WORKOS_API_KEY", ""),
			WorkOSClientID: getEnv("WORKOS_CLIENT_ID", ""),
			RedirectURI:    getEnv("WORKOS_REDIRECT_URI", "http://localhost:8080/auth/callback"),
			FrontendURL:    getEnv("FRONTEND_URL", "http://localhost:5173"),
			CookieName:     getEnv("AUTH_COOKIE_NAME", "astro_session"),
			CookiePassword: getEnv("AUTH_COOKIE_PASSWORD", ""),
			CookieDomain:   getEnv("AUTH_COOKIE_DOMAIN", ""),
			CookieSecure:   getEnv("AUTH_COOKIE_SECURE", "false") == "true",
			CookieSameSite: getEnv("AUTH_COOKIE_SAMESITE", "Lax"),
			CookieMaxAge:   getEnvDuration("AUTH_COOKIE_MAX_AGE", 30*24*time.Hour),
			SessionMaxAge:  getEnvDuration("AUTH_SESSION_MAX_AGE", 30*24*time.Hour),
			JWTIssuer:      getEnv("AUTH_JWT_ISSUER", "https://api.workos.com"),
		},
		Database: DatabaseConfig{
			URL: getEnv("DATABASE_URL", ""),
		},
		AdminGRPC: AdminGRPCConfig{
			Port:         getEnv("ADMIN_GRPC_PORT", "9091"),
			CertFile:     getEnv("ADMIN_GRPC_CERT_FILE", ""),
			KeyFile:      getEnv("ADMIN_GRPC_KEY_FILE", ""),
			CAFile:       getEnv("ADMIN_GRPC_CA_FILE", ""),
			OpenMeterURL: getEnv("OPENMETER_URL", ""),
		},
		FleetGRPC: FleetGRPCConfig{
			Port:     getEnv("FLEET_GRPC_PORT", "9092"),
			CertFile: getEnv("FLEET_TLS_CERT_PATH", ""),
			KeyFile:  getEnv("FLEET_TLS_KEY_PATH", ""),
		},
		Avatar: AvatarConfig{
			S3Bucket:  getEnv("ASSETS_BUCKET", ""),
			LocalDir:  getEnv("ASSETS_LOCAL_DIR", ""),
			AssetsURL: getEnv("ASSETS_CDN_URL", ""),
		},
		S3: S3Config{
			Endpoint:  getEnv("S3_ENDPOINT", ""),
			PathStyle: getEnv("S3_ENDPOINT", "") != "",
		},
		GitHub: GitHubConfig{
			CallbackURL:         getEnv("GITHUB_CALLBACK_URL", ""),
			BuildNamespace:      getEnv("GITHUB_BUILD_NAMESPACE", "as0-builds"),
			BuildServiceAccount: getEnv("GITHUB_BUILD_SERVICE_ACCOUNT", "build-worker"),
			BuildKitConfigMap:   getEnv("GITHUB_BUILDKIT_CONFIGMAP", ""),
		},
		Slack: SlackConfig{
			ClientID:     getEnv("SLACK_CLIENT_ID", ""),
			ClientSecret: getEnv("SLACK_CLIENT_SECRET", ""),
			CallbackURL:  getEnv("SLACK_CALLBACK_URL", ""),
		},
		OpenMeterURL:         getEnv("OPENMETER_URL", ""),
		OpenMeterDefaultPlan: getEnv("OPENMETER_DEFAULT_PLAN", ""),
		OpenMeterEnforce:     getEnv("OPENMETER_ENFORCE", "") == "true",
		LokiURL:              getEnv("LOKI_URL", ""),
		DeploymentLogBackend: getEnv("DEPLOYMENT_LOG_BACKEND", ""),
		PrometheusURL:        getEnv("PROMETHEUS_URL", ""),
		RedisURL:             getEnv("REDIS_URL", ""),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	validRunModes := map[string]bool{"all": true, "api": true, "worker": true}
	if !validRunModes[c.RunMode] {
		return fmt.Errorf("invalid SERVER_MODE: %q (must be all, api, or worker)", c.RunMode)
	}

	if c.Server.Port == "" {
		return fmt.Errorf("server port cannot be empty")
	}

	validModes := map[string]bool{"debug": true, "release": true, "test": true}
	if !validModes[c.Server.Mode] {
		return fmt.Errorf("invalid server mode: %s (must be debug, release, or test)", c.Server.Mode)
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Log.Level] {
		return fmt.Errorf("invalid log level: %s", c.Log.Level)
	}

	// Deployment config only required for API mode
	if c.RunAPI() {
		if c.Deployment.RegistryURL == "" {
			if c.Deployment.K8sClientMode == "local" {
				c.Deployment.RegistryURL = "docker.io/library"
			} else {
				return fmt.Errorf("REGISTRY_URL environment variable is required")
			}
		}

		if c.Deployment.Environment == "" {
			return fmt.Errorf("ENVIRONMENT environment variable is required")
		}

		// Template signature verification gates EnforceEditable bypass — a
		// known key would let any caller forge specs that skip field checks.
		// Allow the compiled-in dev key only against a local cluster.
		if len(c.Deployment.TemplateSigningKey) == 0 {
			if c.Deployment.K8sClientMode == "local" {
				c.Deployment.TemplateSigningKey = localDevSigningKey
			} else {
				return fmt.Errorf("TEMPLATE_SIGNING_KEY environment variable is required (hex-encoded, ≥16 bytes)")
			}
		}

		// The deploy token signs the per-deployment ASTRO_AUTHZ_TOKEN that the
		// messaging container presents back to /deployments/authorize. The
		// dev default literal is in the public source, so a deployment of
		// astro-server that boots with it would let anyone forge a token for
		// any deployment_id and bypass `RequireDeployToken`. Allow the dev
		// default only against a local cluster.
		if c.Security.DeployTokenSecret == "" || c.Security.DeployTokenSecret == DevDeployTokenSecret {
			if c.Deployment.K8sClientMode != "local" {
				return fmt.Errorf("DEPLOY_TOKEN_SECRET environment variable is required in non-local mode (the dev default is published in the public source)")
			}
		}
	}

	if c.Database.URL == "" {
		return fmt.Errorf("DATABASE_URL environment variable is required")
	}

	// Validate auth configuration
	if c.Auth.WorkOSAPIKey == "" {
		return fmt.Errorf("WORKOS_API_KEY environment variable is required")
	}
	if c.Auth.WorkOSClientID == "" {
		return fmt.Errorf("WORKOS_CLIENT_ID environment variable is required")
	}
	if c.Auth.RedirectURI == "" {
		return fmt.Errorf("WORKOS_REDIRECT_URI environment variable is required")
	}
	if c.Auth.CookiePassword == "" {
		return fmt.Errorf("AUTH_COOKIE_PASSWORD environment variable is required (must be at least 32 characters)")
	}
	if len(c.Auth.CookiePassword) < 32 {
		return fmt.Errorf("AUTH_COOKIE_PASSWORD must be at least 32 characters for secure encryption")
	}

	// PrivateLink: if VPC ID is set, subnet IDs and SG ID are also required.
	if d := c.Deployment; d.PrivateLinkVpcID != "" {
		if len(d.PrivateLinkSubnetIDs) == 0 {
			return fmt.Errorf("PRIVATELINK_SUBNET_IDS is required when PRIVATELINK_VPC_ID is set")
		}
		if d.PrivateLinkSGID == "" {
			return fmt.Errorf("PRIVATELINK_SG_ID is required when PRIVATELINK_VPC_ID is set")
		}
	}

	return nil
}

// DevDeployTokenSecret is the HMAC secret used for the deploy token only when
// running against a local cluster. Validate() rejects this default in any
// non-local mode because the literal is in the public source — anyone could
// forge a deploy token and bypass `RequireDeployToken`.
const DevDeployTokenSecret = "astro-dev-secret"

// localDevSigningKey is a static HMAC key used only when running against a
// local cluster (K8sClientMode="local") and TEMPLATE_SIGNING_KEY is unset.
// Validate() rejects this fallback for any non-local mode because anyone with
// repo access could otherwise forge signatures and bypass EnforceEditable.
var localDevSigningKey = []byte{
	0x61, 0x73, 0x74, 0x72, 0x6f, 0x2d, 0x74, 0x6d,
	0x70, 0x6c, 0x2d, 0x73, 0x69, 0x67, 0x6e, 0x2d,
	0x6b, 0x65, 0x79, 0x2d, 0x64, 0x65, 0x66, 0x61,
	0x75, 0x6c, 0x74, 0x2d, 0x76, 0x31, 0x2e, 0x30,
}

// loadSigningKey decodes TEMPLATE_SIGNING_KEY (hex-encoded) from the
// environment. Returns nil when the env var is unset or invalid; Validate()
// then either substitutes localDevSigningKey (local mode) or fails.
func loadSigningKey() []byte {
	raw := os.Getenv("TEMPLATE_SIGNING_KEY")
	if raw == "" {
		return nil
	}
	key, err := hex.DecodeString(raw)
	if err != nil || len(key) == 0 {
		return nil
	}
	return key
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt gets an integer from an environment variable or returns the default
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
	}
	return defaultValue
}

// getEnvDuration gets a duration from environment variable or returns default
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// getEnvSlice gets a comma-separated environment variable as a slice
func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Simple split by comma, can be enhanced based on needs
		var result []string
		for i := 0; i < len(value); i++ {
			start := i
			for i < len(value) && value[i] != ',' {
				i++
			}
			if item := value[start:i]; item != "" {
				result = append(result, item)
			}
		}
		return result
	}
	return defaultValue
}
