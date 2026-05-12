package spec

import "strings"

// CredentialSuffix describes one credential a cloud provider requires.
type CredentialSuffix struct {
	Suffix      string
	Description string
	Optional    bool
}

// PortDef defines a named port.
type PortDef struct {
	Name string
	Port int
}

// BindCredentialDef maps a reference attribute name (used in ${knowledge.*.credentials.<attr>})
// to the exact key stored in knowledge_store_credentials.
type BindCredentialDef struct {
	Attr       string // reference attribute (e.g. "user", "password")
	StorageKey string // exact key in credentials store (e.g. "POSTGRES_USER")
}

// Toleration mirrors corev1.Toleration for use outside k8s packages.
type Toleration struct {
	Key      string
	Operator string // "Exists" or "Equal"
	Value    string
	Effect   string // "NoSchedule", "NoExecute", "PreferNoSchedule"
}

// BuiltinProvider is the single canonical type for every platform-known provider.
// All providers — cloud and self-hosted, across all sections — are declared once
// in the builtinProviders slice below. Everything else is derived from it.
type BuiltinProvider struct {
	Name    string // lowercase provider name (e.g. "ollama", "anthropic")
	Section string // "models", "knowledge", or "integrations"
	Cloud   bool   // true → credentials only, no container deployed
	Managed bool   // true → server injects credentials from its own environment (user never provides them)

	// Cloud provider fields
	Credentials []CredentialSuffix

	// Self-hosted provider fields
	Image          string
	DefaultPort    int
	ExtraPorts     []PortDef
	MountPath      string
	EnvPrefix      string
	URLScheme      string
	HealthCheck    []string // exec health check; nil → use HealthPath
	HealthPath     string   // HTTP health check path
	DefaultEnv     map[string]string
	GPU            bool
	NodeSelector   map[string]string
	Tolerations    []Toleration
	WritableRootFS bool     // true → skip readOnlyRootFilesystem (e.g. qdrant writes outside its data mount)
	ExtraEmptyDirs []string // extra paths that need writable emptyDir mounts (e.g. "/qdrant/snapshots")
	FsGroup        int64    // non-zero → pod runs as this uid/gid (overrides hardened default of 1000)
	InitSQL        string   // optional SQL run via /docker-entrypoint-initdb.d/ on first boot (postgres-compatible images only)

	// BindCredentials defines the credential schema for knowledge store bindings.
	// Each entry maps a reference attribute (e.g. "user") to its storage key (e.g. "USERNAME").
	// Used by ${knowledge.*.credentials.<attr>} reference validation and deploy-time resolution.
	BindCredentials []BindCredentialDef
}

// builtinProviders is the single authoritative list of all platform-known providers.
// To add a provider, add one entry here — no other file needs to change.
var builtinProviders = []BuiltinProvider{
	// ── Models: self-hosted ──────────────────────────────────────────────────
	{
		Name: "ollama", Section: "models",
		Image: "ollama/ollama:latest", DefaultPort: 11434,
		MountPath: "/root/.ollama", EnvPrefix: "OLLAMA",
		HealthPath:   "/api/tags",
		DefaultEnv:   map[string]string{"OLLAMA_HOST": "0.0.0.0", "OLLAMA_KEEP_ALIVE": "-1"},
		GPU:          true,
		NodeSelector: map[string]string{"workload-type": "gpu"},
		Tolerations:  []Toleration{{Key: "nvidia.com/gpu", Operator: "Exists", Effect: "NoSchedule"}},
	},

	// ── Models: cloud ────────────────────────────────────────────────────────
	{
		Name: "anthropic", Section: "models", Cloud: true,
		Credentials: []CredentialSuffix{{Suffix: "API_KEY", Description: "Anthropic API key for Claude models"}},
	},
	{
		Name: "anthropic-managed", Section: "models", Cloud: true, Managed: true,
		Credentials: []CredentialSuffix{{Suffix: "API_KEY", Description: "Anthropic API key (provided by platform)"}},
	},
	{
		Name: "openai", Section: "models", Cloud: true,
		Credentials: []CredentialSuffix{{Suffix: "API_KEY", Description: "OpenAI API key for GPT models"}},
	},
	{
		Name: "google", Section: "models", Cloud: true,
		Credentials: []CredentialSuffix{{Suffix: "API_KEY", Description: "Google API key for Gemini models"}},
	},
	{
		Name: "gemini", Section: "models", Cloud: true,
		Credentials: []CredentialSuffix{{Suffix: "API_KEY", Description: "Google API key for Gemini models (alias for google)"}},
	},
	{
		Name: "cohere", Section: "models", Cloud: true,
		Credentials: []CredentialSuffix{{Suffix: "API_KEY", Description: "Cohere API key for language models"}},
	},

	// ── Knowledge: self-hosted ───────────────────────────────────────────────
	{
		Name: "qdrant", Section: "knowledge",
		Image: "qdrant/qdrant:latest", DefaultPort: 6333,
		ExtraPorts: []PortDef{{Name: "grpc", Port: 6334}},
		MountPath:  "/qdrant/storage", EnvPrefix: "QDRANT", URLScheme: "http",
		HealthPath:     "/healthz",
		WritableRootFS: true,
		ExtraEmptyDirs: []string{"/qdrant/snapshots"},
		BindCredentials: []BindCredentialDef{
			{Attr: "api_key", StorageKey: "QDRANT__SERVICE__API_KEY"},
		},
	},
	{
		Name: "redis", Section: "knowledge",
		Image: "redis:7-alpine", DefaultPort: 6379,
		MountPath: "/data", EnvPrefix: "REDIS", URLScheme: "redis",
		HealthCheck: []string{"redis-cli", "ping"},
		BindCredentials: []BindCredentialDef{
			{Attr: "password", StorageKey: "REDIS_PASSWORD"},
		},
	},
	{
		Name: "postgres", Section: "knowledge",
		Image: "pgvector/pgvector:pg17", DefaultPort: 5432,
		MountPath: "/var/lib/postgresql/data", EnvPrefix: "POSTGRES",
		HealthCheck:    []string{"sh", "-c", `pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"`},
		DefaultEnv:     map[string]string{"PGDATA": "/var/lib/postgresql/data/pgdata"},
		FsGroup:        999,                             // postgres uid/gid — entrypoint skips chown when running as non-root
		ExtraEmptyDirs: []string{"/var/run/postgresql"}, // socket dir must be writable; emptyDir avoids chmod under dropped caps
		InitSQL:        "CREATE EXTENSION IF NOT EXISTS vector;",
		BindCredentials: []BindCredentialDef{
			{Attr: "user", StorageKey: "POSTGRES_USER"},
			{Attr: "password", StorageKey: "POSTGRES_PASSWORD"},
			{Attr: "database", StorageKey: "POSTGRES_DB"},
		},
	},
	{
		Name: "mysql", Section: "knowledge",
		Image: "mysql:8.0", DefaultPort: 3306,
		MountPath: "/var/lib/mysql", EnvPrefix: "MYSQL",
		HealthCheck: []string{"sh", "-c", `mysqladmin ping -h 127.0.0.1 -u "$MYSQL_USER" -p"$MYSQL_PASSWORD"`},
		FsGroup:     999, // mysql uid/gid in the official image
		BindCredentials: []BindCredentialDef{
			{Attr: "user", StorageKey: "MYSQL_USER"},
			{Attr: "password", StorageKey: "MYSQL_PASSWORD"},
			{Attr: "database", StorageKey: "MYSQL_DATABASE"},
		},
	},
	{
		Name: "neo4j", Section: "knowledge",
		Image: "neo4j:5-community", DefaultPort: 7474,
		ExtraPorts: []PortDef{{Name: "bolt", Port: 7687}},
		MountPath:  "/data", EnvPrefix: "NEO4J", URLScheme: "bolt",
		HealthPath: "/",
		DefaultEnv: map[string]string{"NEO4J_AUTH": "none"},
		BindCredentials: []BindCredentialDef{
			{Attr: "auth", StorageKey: "NEO4J_AUTH"},
		},
	},

	// ── Knowledge: cloud ─────────────────────────────────────────────────────
	{
		Name: "pinecone", Section: "knowledge", Cloud: true,
		Credentials: []CredentialSuffix{{Suffix: "API_KEY", Description: "Pinecone API key for vector database"}},
	},

	// ── Integrations: cloud ─────────────────────────────────────────────────────────
	{
		Name: "github", Section: "integrations", Cloud: true,
		Credentials: []CredentialSuffix{{Suffix: "TOKEN", Description: "GitHub token for API access"}},
	},
	{
		Name: "gitlab", Section: "integrations", Cloud: true,
		Credentials: []CredentialSuffix{{Suffix: "TOKEN", Description: "GitLab token for API access"}},
	},
}

// ── Lookup indexes (built once at init) ──────────────────────────────────────

var builtinIndex map[string]BuiltinProvider // "section:name" → provider

func init() {
	builtinIndex = make(map[string]BuiltinProvider, len(builtinProviders))
	for _, p := range builtinProviders {
		builtinIndex[p.Section+":"+p.Name] = p
	}
}

// LookupBuiltin returns the BuiltinProvider for the given section and name.
// The second return value is false if the provider is not in the registry.
func LookupBuiltin(section, name string) (BuiltinProvider, bool) {
	p, ok := builtinIndex[section+":"+strings.ToLower(name)]
	return p, ok
}

// ── Derived helpers (maintain backward-compatible API) ───────────────────────

// Provider holds self-hosted container configuration. Returned by GetProvider
// and GetModelProvider for backward compatibility with existing callers.
type Provider = BuiltinProvider

func IsCloudModelProvider(name string) bool {
	p, ok := LookupBuiltin("models", name)
	return ok && p.Cloud
}

func IsManagedProvider(section, name string) bool {
	p, ok := LookupBuiltin(section, name)
	return ok && p.Managed
}

func IsCloudKnowledgeProvider(name string) bool {
	p, ok := LookupBuiltin("knowledge", name)
	return ok && p.Cloud
}

func IsCloudIntegrationProvider(name string) bool {
	p, ok := LookupBuiltin("integrations", name)
	return ok && p.Cloud
}

func GetCloudModelCredentials(name string) ([]CredentialSuffix, bool) {
	p, ok := LookupBuiltin("models", name)
	if !ok || !p.Cloud {
		return nil, false
	}
	return p.Credentials, true
}

func GetCloudKnowledgeCredentials(name string) ([]CredentialSuffix, bool) {
	p, ok := LookupBuiltin("knowledge", name)
	if !ok || !p.Cloud {
		return nil, false
	}
	return p.Credentials, true
}

func GetCloudIntegrationCredentials(name string) ([]CredentialSuffix, bool) {
	p, ok := LookupBuiltin("integrations", name)
	if !ok || !p.Cloud {
		return nil, false
	}
	return p.Credentials, true
}

// GetProvider returns self-hosted configuration for a knowledge provider.
// Unknown or cloud-only providers return a zero BuiltinProvider.
func GetProvider(name string) BuiltinProvider {
	p, ok := LookupBuiltin("knowledge", name)
	if !ok || p.Cloud {
		return BuiltinProvider{}
	}
	return p
}

// CredentialKeys returns the credential attribute names valid for
// ${knowledge.*.credentials.<attr>} references for the given provider.
// Derived from the provider registry's BindCredentials field.
func CredentialKeys(provider string) []string {
	p, ok := LookupBuiltin("knowledge", provider)
	if !ok {
		return nil
	}
	keys := make([]string, len(p.BindCredentials))
	for i, c := range p.BindCredentials {
		keys[i] = c.Attr
	}
	return keys
}

// CredentialStorageKeyMap returns a map of storage key → reference attribute for a provider.
// Used at deploy time to map decrypted credential keys to reference attributes.
func CredentialStorageKeyMap(provider string) map[string]string {
	p, ok := LookupBuiltin("knowledge", provider)
	if !ok {
		return nil
	}
	m := make(map[string]string, len(p.BindCredentials))
	for _, c := range p.BindCredentials {
		m[c.StorageKey] = c.Attr
	}
	return m
}

// ProviderEndpoints returns the default endpoints for a self-hosted knowledge provider,
// derived from the provider registry. Used by reference validation for bound entries
// (whose Endpoints map is empty).
func ProviderEndpoints(provider string) map[string]Endpoint {
	p := GetProvider(provider)
	if p.Name == "" {
		return nil
	}
	eps := map[string]Endpoint{
		"http": {Port: p.DefaultPort, Protocol: "http"},
	}
	for _, ep := range p.ExtraPorts {
		eps[ep.Name] = Endpoint{Port: ep.Port, Protocol: ep.Name}
	}
	return eps
}

// GetModelProvider returns self-hosted configuration for a model provider.
// Unknown or cloud-only providers return a zero BuiltinProvider.
func GetModelProvider(name string) BuiltinProvider {
	p, ok := LookupBuiltin("models", name)
	if !ok || p.Cloud {
		return BuiltinProvider{}
	}
	return p
}
