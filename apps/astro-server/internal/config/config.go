package config

import (
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the application configuration
type Config struct {
	RunMode    string // SERVER_MODE: "all" (default), "api", or "worker"
	Server     ServerConfig
	Log        LogConfig
	Security   SecurityConfig
	Deployment DeploymentConfig
	Auth       AuthConfig
	Database   DatabaseConfig
	AdminGRPC  AdminGRPCConfig
	Avatar     AvatarConfig
	// BillingProvider selects the metering/billing backend: "noop" (OSS,
	// unmetered) or "metronome" (hosted). Empty resolves to noop via
	// BillingBackend().
	BillingProvider       string // BILLING_PROVIDER
	QuotaEnforce          bool   // QUOTA_ENFORCE — enable DB-quota enforcement (default false)
	FGAShadowEnabled      bool   // FGA_SHADOW_ENABLED — compare deployment authorization with WorkOS without enforcing
	FGAEnforcementEnabled bool   // FGA_ENFORCEMENT_ENABLED — enforce opted-in FGA-ready deployment mutations
	// Metronome hosted billing (BILLING_PROVIDER=metronome).
	MetronomeAPIKey        string // METRONOME_API_KEY — SDK bearer token
	MetronomeWebhookSecret string // METRONOME_WEBHOOK_SECRET — HMAC-SHA256 webhook verification
	// Signup provisioning. Empty package disables it. The package carries the
	// signup credit, so its amount, unit, and expiry are Metronome-side.
	MetronomePackageID string // METRONOME_PACKAGE_ID
	// MetronomeDashboardEnv is the environment segment in Metronome dashboard
	// URLs (app.metronome.com/<env>/customers/...), used only to build admin
	// deep links. Empty for the default environment. The API token is scoped to
	// one environment, so this must name the same one or the links resolve to
	// nothing.
	MetronomeDashboardEnv string // METRONOME_DASHBOARD_ENV
	// Stripe card-vault (payment-method collection only; Metronome charges the
	// saved card). Enabled when StripeSecretKey is set. astro-server never moves
	// money — it only creates SetupIntents and saves cards. Card setup is
	// confirmed synchronously (the server re-reads the SetupIntent from Stripe);
	// the webhook (below) is separate and carries payment-collection lifecycle.
	StripeSecretKey      string // STRIPE_SECRET_KEY — server-side SDK key
	StripePublishableKey string // STRIPE_PUBLISHABLE_KEY — surfaced to the client for Elements
	StripeWebhookSecret  string // STRIPE_WEBHOOK_SECRET — signature verification for payment-collection webhooks
	// Consumption gating (hosted). Status is always computed/written; enforce
	// controls whether it is acted on (402, suspend, notifications) or only
	// logged. Resume is never gated, so turning this off can undo suspensions.
	BillingGateEnforce      bool // BILLING_GATE_ENFORCE — false = observe/log, true = enforce
	BillingDunningGraceDays int  // BILLING_DUNNING_GRACE_DAYS — past_due→suspended window (default 7)
	// QuotaDefaults holds the system-wide default per-account resource limits
	// (blueprints, agent_builds, agent_deployments, members, knowledge_stores,
	// knowledge_endpoints). -1 = unlimited, 0 = disabled. Per-account overrides
	// live in the account_limits table. Overridable via QUOTA_DEFAULTS
	// ("blueprints=10,members=5,..."). See internal/quota.
	QuotaDefaults      map[string]int64
	S3                 S3Config
	GitHub             GitHubConfig
	Slack              SlackConfig
	Notify             NotifyConfig
	OTelIngestEndpoint string // OTEL_INGEST_ENDPOINT — public OTLP ingest URL shown in the ingest-key managed-settings block (e.g. https://otel.astropods.ai)
	RedisURL           string // REDIS_URL — enables K8s state caching when set (e.g. redis://localhost:6379)
}

// Billing backend identifiers.
const (
	BillingBackendNoop      = "noop"
	BillingBackendMetronome = "metronome"
)

// BillingBackend resolves the effective billing backend. An explicit
// BILLING_PROVIDER wins; otherwise it defaults to noop.
func (c *Config) BillingBackend() string {
	if c.BillingProvider != "" {
		return c.BillingProvider
	}
	return BillingBackendNoop
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
	Port     string // ADMIN_GRPC_PORT, default "9091" (optional — gRPC server disabled if empty)
	CertFile string // ADMIN_GRPC_CERT_FILE — file path or inline PEM (optional — no TLS if empty)
	KeyFile  string // ADMIN_GRPC_KEY_FILE  — file path or inline PEM
	CAFile   string // ADMIN_GRPC_CA_FILE   — file path or inline PEM
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

// NotifyConfig holds Novu notification settings. Empty NovuAPIURL selects the
// no-op provider (OSS/unconfigured): emits are logged and dropped.
type NotifyConfig struct {
	// NOVU_API_URL — Novu REST API base (e.g. https://api.novu.astroids.ai).
	// In-cluster/cross-cluster address in prod; the public host works from
	// WAF-allowlisted developer machines for local dev.
	NovuAPIURL string
	// NOVU_SECRET_KEY — Novu environment API key for ApiKey auth. Treat as a
	// secret. This is the dashboard environment key, not the self-hosted
	// store-encryption key of the same name in the novu-app K8s Secret.
	NovuSecretKey string
	// NOVU_TEST_WORKFLOW_ID — overrides the workflow the "Send test" button
	// triggers. Defaults to the system.test workflow id; set this in local dev
	// to an existing workflow (e.g. test-02) when system.test is not authored.
	TestWorkflowID string
	// NOVU_APP_IDENTIFIER — the Novu environment's application identifier, used
	// by the browser Inbox component. Per-environment; empty disables the Inbox.
	AppIdentifier string
	// NOVU_SOCKET_URL — the self-hosted Novu websocket base the Inbox connects
	// to for the live feed (e.g. https://ws.novu.astroids.ai).
	SocketURL string
}

// DeploymentConfig holds deployment-related configuration
type DeploymentConfig struct {
	RegistryURL       string // ECR registry URL (required for eks mode, defaults to "docker.io/library" for local)
	ProxyRegistryHost string // Proxy registry host (e.g., registry.example.com)
	Environment       string // Environment prefix for ECR tenant repos (e.g. "prod", "preview")
	// MessagingImage overrides the messaging sidecar image reference (bare
	// Docker Hub ref, e.g. a pinned "astropods/messaging@sha256:..."). Empty
	// uses the built-in default. Set per-environment to pin prod independently
	// of preview; infra routes the pull through the ECR pull-through cache.
	MessagingImage string // MESSAGING_IMAGE
	// ClusterConfigPath is a mounted JSON file listing every managed cluster's
	// connectivity data (required for eks mode). DefaultClusterID picks out
	// the entry astro-server itself runs on.
	ClusterConfigPath string // CLUSTER_CONFIG_PATH
	DefaultClusterID  string // DEFAULT_CLUSTER_ID
	AWSRegion         string // AWS region (optional, auto-detected from IRSA)
	// K8s client mode: "eks" (default) or "local" (Docker Desktop / kind / minikube)
	K8sClientMode  string // K8S_CLIENT_MODE
	KubeconfigPath string // KUBECONFIG path (local mode, defaults to ~/.kube/config)
	KubeContext    string // KUBE_CONTEXT (local mode, defaults to current-context)
	// IngressDomain, AgentPublicIngressDomain, IngestionIngressDomain,
	// PodSubnetCIDRs, and PodSubnetIPv6CIDRs are only set directly by tests /
	// clustercfg.Resolve's nil-registry fallback — the default cluster's own
	// values come from its clusterConfig entry instead (see clustercfg.Resolve).
	IngressDomain            string
	AgentPublicIngressDomain string
	IngestionIngressDomain   string
	PodSubnetCIDRs           []string
	PodSubnetIPv6CIDRs       []string
	// EKS apiserver ENI subnets (primary VPC private subnets). Service proxy traffic
	// from astro-server enters tenant pods from these CIDRs.
	CPSubnetCIDRs []string // CP_SUBNET_CIDRS
	// KMS envelope encryption for deployment secrets
	KMSKeyARN string // KMS_KEY_ARN — ARN of the KMS key for secret encryption (optional — secrets stripped if unset)
	// AI Gateway (LiteLLM) — per-tenant virtual key issuance and the base URL
	// every caller (astro-server admin, deploy-time tenant pods, local dev
	// containers) uses to reach the gateway. The gateway is publicly
	// reachable over TLS; auth is the gate, not the network. Empty
	// AIGatewayURL disables the feature.
	AIGatewayURL       string // AI_GATEWAY_URL — public gateway base_url tenants use (e.g. https://aig.astropod.ai)
	AIGatewayAdminURL  string // AI_GATEWAY_ADMIN_URL — in-cluster Bifrost governance API (e.g. http://bifrost.bifrost.svc.cluster.local:8080)
	AIGatewayAdminAuth string // AI_GATEWAY_ADMIN_AUTH — full Authorization header (Basic base64(admin:pass)), ESO-delivered
	// Local dev — inject a messaging URL without a real ingress (e.g. http://localhost:8081)
	MessagingURLOverride string // MESSAGING_URL_OVERRIDE
	// TenantRouterInternalURL is only set directly by tests / clustercfg's
	// nil-registry fallback — see ClusterEntry.TenantRouterInternalURL.
	TenantRouterInternalURL string
	// Observability (Langfuse) — direct DB provisioning for per-account projects
	LangfuseDBURL   string // LANGFUSE_DB_URL — Postgres connection string for Langfuse's database
	LangfuseSalt    string // LANGFUSE_SALT — must match Langfuse's SALT env var
	LangfuseOrgID   string // LANGFUSE_ORG_ID — the single org ID in our Langfuse instance
	LangfuseBaseURL string // LANGFUSE_BASE_URL — Langfuse instance URL
	// LangfuseBaseURLExt and LangfuseVPCEIPs are only set directly by tests /
	// clustercfg's nil-registry fallback — see ClusterEntry.LangfuseBaseURLExt.
	LangfuseBaseURLExt string
	LangfuseVPCEIPs    []string
	// PrivateLink automation — managed cluster VPC where VPC endpoints are created at runtime
	PrivateLinkVpcID     string   // PRIVATELINK_VPC_ID — managed cluster VPC ID (empty = PrivateLink disabled)
	PrivateLinkSubnetIDs []string // PRIVATELINK_SUBNET_IDS — comma-separated private subnet IDs
	PrivateLinkSGID      string   // PRIVATELINK_SG_ID — security group for PrivateLink endpoints (broad HTTPS egress; fine-grained access via K8s NetworkPolicy)
	// Template signing — HMAC key for signing deployment templates so the deploy
	// endpoint can verify them without re-generating. Auto-generated at startup if
	// not set. For multi-replica deployments behind a load balancer, set a shared key.
	TemplateSigningKey []byte // TEMPLATE_SIGNING_KEY (hex-encoded; auto-generated if empty)

	// RegistryPullCredential is the cluster pull credential (CPC) the deployer
	// embeds in each tenant namespace's image-pull Secret so tenant pods pull
	// tenant images through astro-registry. Delivered via External Secrets.
	// Empty disables the injection (e.g. local dev). Treat as a secret.
	// See docs/01-spec/registry-pull-through-spec.md.
	RegistryPullCredential string // REGISTRY_PULL_CREDENTIAL
}

// IsLocal reports whether the server is running in the local dev environment
// (ENVIRONMENT=local). Use this instead of comparing Environment to "local" so
// the check lives in one place.
func (d DeploymentConfig) IsLocal() bool {
	return d.Environment == "local"
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
			RegistryURL:            getEnv("REGISTRY_URL", ""),
			ProxyRegistryHost:      getEnv("PROXY_REGISTRY_HOST", ""),
			Environment:            getEnv("ENVIRONMENT", ""),
			MessagingImage:         getEnv("MESSAGING_IMAGE", ""),
			ClusterConfigPath:      getEnv("CLUSTER_CONFIG_PATH", ""),
			DefaultClusterID:       getEnv("DEFAULT_CLUSTER_ID", ""),
			AWSRegion:              getEnv("AWS_REGION", ""),
			K8sClientMode:          getEnv("K8S_CLIENT_MODE", "eks"),
			KubeconfigPath:         getEnv("KUBECONFIG", ""),
			KubeContext:            getEnv("KUBE_CONTEXT", ""),
			CPSubnetCIDRs:          getEnvSlice("CP_SUBNET_CIDRS", nil),
			KMSKeyARN:              getEnv("KMS_KEY_ARN", ""),
			AIGatewayURL:           getEnv("AI_GATEWAY_URL", ""),
			AIGatewayAdminURL:      getEnv("AI_GATEWAY_ADMIN_URL", ""),
			AIGatewayAdminAuth:     getEnv("AI_GATEWAY_ADMIN_AUTH", ""),
			MessagingURLOverride:   getEnv("MESSAGING_URL_OVERRIDE", ""),
			LangfuseDBURL:          getEnv("LANGFUSE_DB_URL", ""),
			LangfuseSalt:           getEnv("LANGFUSE_SALT", ""),
			LangfuseOrgID:          getEnv("LANGFUSE_ORG_ID", "astro"),
			LangfuseBaseURL:        getEnv("LANGFUSE_BASE_URL", ""),
			PrivateLinkVpcID:       getEnv("PRIVATELINK_VPC_ID", ""),
			PrivateLinkSubnetIDs:   getEnvSlice("PRIVATELINK_SUBNET_IDS", nil),
			PrivateLinkSGID:        getEnv("PRIVATELINK_SG_ID", ""),
			TemplateSigningKey:     loadSigningKey(),
			RegistryPullCredential: getEnv("REGISTRY_PULL_CREDENTIAL", ""),
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
			Port:     getEnv("ADMIN_GRPC_PORT", "9091"),
			CertFile: getEnv("ADMIN_GRPC_CERT_FILE", ""),
			KeyFile:  getEnv("ADMIN_GRPC_KEY_FILE", ""),
			CAFile:   getEnv("ADMIN_GRPC_CA_FILE", ""),
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
		Notify: NotifyConfig{
			NovuAPIURL:     getEnv("NOVU_API_URL", ""),
			NovuSecretKey:  getEnv("NOVU_SECRET_KEY", ""),
			TestWorkflowID: getEnv("NOVU_TEST_WORKFLOW_ID", ""),
			AppIdentifier:  getEnv("NOVU_APP_IDENTIFIER", ""),
			SocketURL:      getEnv("NOVU_SOCKET_URL", ""),
		},
		BillingProvider:         getEnv("BILLING_PROVIDER", ""),
		MetronomeAPIKey:         getEnv("METRONOME_API_KEY", ""),
		MetronomeWebhookSecret:  getEnv("METRONOME_WEBHOOK_SECRET", ""),
		MetronomePackageID:      getEnv("METRONOME_PACKAGE_ID", ""),
		MetronomeDashboardEnv:   getEnv("METRONOME_DASHBOARD_ENV", ""),
		StripeSecretKey:         getEnv("STRIPE_SECRET_KEY", ""),
		StripePublishableKey:    getEnv("STRIPE_PUBLISHABLE_KEY", ""),
		StripeWebhookSecret:     getEnv("STRIPE_WEBHOOK_SECRET", ""),
		BillingGateEnforce:      getEnv("BILLING_GATE_ENFORCE", "") == "true",
		BillingDunningGraceDays: getEnvIntDefault("BILLING_DUNNING_GRACE_DAYS", 7),
		QuotaEnforce:            getEnv("QUOTA_ENFORCE", "") == "true",
		FGAShadowEnabled:        getEnv("FGA_SHADOW_ENABLED", "") == "true",
		FGAEnforcementEnabled:   getEnv("FGA_ENFORCEMENT_ENABLED", "") == "true",
		QuotaDefaults:           loadQuotaDefaults(),
		OTelIngestEndpoint:      getEnv("OTEL_INGEST_ENDPOINT", ""),
		RedisURL:                getEnv("REDIS_URL", ""),
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

	if c.BillingProvider != "" {
		switch c.BillingProvider {
		case BillingBackendNoop, BillingBackendMetronome:
		default:
			return fmt.Errorf("invalid BILLING_PROVIDER: %q (must be noop or metronome)", c.BillingProvider)
		}
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

// quotaResourceDefaults is the system-wide default per-account resource limit
// for each quota-managed resource. Over-limit blocking is gated by QUOTA_ENFORCE;
// a per-account account_limits row overrides these.
// Use -1 for unlimited, 0 to disable a feature.
var quotaResourceDefaults = map[string]int64{
	"blueprints":          5,
	"agent_builds":        50,
	"agent_deployments":   10,
	"members":             5,
	"knowledge_stores":    5,
	"knowledge_endpoints": 2,
}

// loadQuotaDefaults builds the default resource→limit map, applying any
// QUOTA_DEFAULTS overrides ("agents=10,members=5"). Unknown keys and malformed
// pairs are ignored.
func loadQuotaDefaults() map[string]int64 {
	defaults := make(map[string]int64, len(quotaResourceDefaults))
	maps.Copy(defaults, quotaResourceDefaults)
	raw := os.Getenv("QUOTA_DEFAULTS")
	if raw == "" {
		return defaults
	}
	for pair := range strings.SplitSeq(raw, ",") {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		if _, ok := defaults[key]; !ok {
			continue
		}
		if v, err := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64); err == nil {
			defaults[key] = v
		}
	}
	return defaults
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvIntDefault parses an int env var, falling back to defaultValue when
// unset or unparseable.
func getEnvIntDefault(key string, defaultValue int) int {
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
