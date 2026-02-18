package spec

import (
	"strings"
)

// CredentialSuffix describes one credential a cloud provider requires.
type CredentialSuffix struct {
	Suffix      string
	Description string
	Optional    bool
}

// Cloud provider registries — these providers have no container, only credentials.

var cloudModelProviders = map[string][]CredentialSuffix{
	"anthropic": {{Suffix: "API_KEY", Description: "Anthropic API key for Claude models"}},
	"openai":    {{Suffix: "API_KEY", Description: "OpenAI API key for GPT models"}},
	"google":    {{Suffix: "API_KEY", Description: "Google API key for Gemini models"}},
	"gemini":    {{Suffix: "API_KEY", Description: "Google API key for Gemini models"}},
	"cohere":    {{Suffix: "API_KEY", Description: "Cohere API key for language models"}},
}

var cloudKnowledgeProviders = map[string][]CredentialSuffix{
	"pinecone": {{Suffix: "API_KEY", Description: "Pinecone API key for vector database"}},
}

var cloudToolProviders = map[string][]CredentialSuffix{
	"github": {{Suffix: "TOKEN", Description: "GitHub token for API access"}},
	"gitlab": {{Suffix: "TOKEN", Description: "GitLab token for API access"}},
}

// IsCloudModelProvider returns true if the provider is a cloud model provider (no container).
func IsCloudModelProvider(name string) bool {
	_, ok := cloudModelProviders[strings.ToLower(name)]
	return ok
}

// IsCloudKnowledgeProvider returns true if the provider is a cloud knowledge provider (no container).
func IsCloudKnowledgeProvider(name string) bool {
	_, ok := cloudKnowledgeProviders[strings.ToLower(name)]
	return ok
}

// IsCloudToolProvider returns true if the provider is a cloud tool provider (no container).
func IsCloudToolProvider(name string) bool {
	_, ok := cloudToolProviders[strings.ToLower(name)]
	return ok
}

// GetCloudModelCredentials returns credential suffixes for a cloud model provider.
func GetCloudModelCredentials(name string) ([]CredentialSuffix, bool) {
	cs, ok := cloudModelProviders[strings.ToLower(name)]
	return cs, ok
}

// GetCloudKnowledgeCredentials returns credential suffixes for a cloud knowledge provider.
func GetCloudKnowledgeCredentials(name string) ([]CredentialSuffix, bool) {
	cs, ok := cloudKnowledgeProviders[strings.ToLower(name)]
	return cs, ok
}

// GetCloudToolCredentials returns credential suffixes for a cloud tool provider.
func GetCloudToolCredentials(name string) ([]CredentialSuffix, bool) {
	cs, ok := cloudToolProviders[strings.ToLower(name)]
	return cs, ok
}


// PortDef defines a named port.
type PortDef struct {
	Name string
	Port int
}

// Provider holds provider-specific configuration.
type Provider struct {
	Image        string            // default container image (e.g., "qdrant/qdrant:latest")
	DefaultPort  int               // primary port (6333 for qdrant, 6379 for redis, etc.)
	ExtraPorts   []PortDef         // additional named ports (e.g., gRPC 6334 for qdrant)
	MountPath    string            // volume mount path for persistent data
	EnvPrefix    string            // env var prefix ("QDRANT", "REDIS", etc.)
	URLScheme    string            // connection URL scheme ("http", "redis")
	HealthCheck  []string          // exec health check command; nil → use HealthPath instead
	HealthPath   string            // HTTP health check path (e.g., "/healthz")
	DefaultEnv   map[string]string // default environment variables for the container
	GPU          bool              // whether the provider requires GPU resources
	NodeSelector map[string]string // node selector labels for scheduling
	Tolerations  []Toleration      // tolerations for GPU/specialized node taints
}

// Toleration mirrors corev1.Toleration for use outside k8s packages.
type Toleration struct {
	Key      string
	Operator string // "Exists" or "Equal"
	Value    string
	Effect   string // "NoSchedule", "NoExecute", "PreferNoSchedule"
}

var providerRegistry = map[string]Provider{
	"qdrant": {
		Image:       "qdrant/qdrant:latest",
		DefaultPort: 6333,
		ExtraPorts:  []PortDef{{Name: "grpc", Port: 6334}},
		MountPath:   "/qdrant/storage",
		EnvPrefix:   "QDRANT",
		URLScheme:   "http",
		HealthPath:  "/healthz",
	},
	"redis": {
		Image:       "redis:7-alpine",
		DefaultPort: 6379,
		MountPath:   "/data",
		EnvPrefix:   "REDIS",
		URLScheme:   "redis",
		HealthCheck: []string{"redis-cli", "ping"},
	},
	"postgres": {
		Image:       "postgres:15-alpine",
		DefaultPort: 5432,
		MountPath:   "/var/lib/postgresql/data",
		EnvPrefix:   "POSTGRES",
		HealthCheck: []string{"pg_isready", "-U", "postgres"},
	},
	"neo4j": {
		Image:       "neo4j:5-community",
		DefaultPort: 7474,
		ExtraPorts:  []PortDef{{Name: "bolt", Port: 7687}},
		MountPath:   "/data",
		EnvPrefix:   "NEO4J",
		URLScheme:   "bolt",
		HealthPath:  "/",
		DefaultEnv:  map[string]string{"NEO4J_AUTH": "none"},
	},
}

var defaultProvider = Provider{
	DefaultPort: 6333,
	MountPath:   "/data",
}

// GetProvider returns configuration for a given knowledge provider name.
// Unknown providers get a sensible fallback.
func GetProvider(provider string) Provider {
	if p, ok := providerRegistry[strings.ToLower(provider)]; ok {
		return p
	}
	return defaultProvider
}

// Model provider registry (separate from knowledge providers).
var modelProviderRegistry = map[string]Provider{
	"ollama": {
		Image:       "ollama/ollama:latest",
		DefaultPort: 11434,
		MountPath:   "/root/.ollama",
		EnvPrefix:   "OLLAMA",
		HealthPath:  "/api/tags",
		DefaultEnv:  map[string]string{"OLLAMA_HOST": "0.0.0.0", "OLLAMA_KEEP_ALIVE": "-1"},
		GPU:         true,
		NodeSelector: map[string]string{"workload-type": "gpu"},
		Tolerations: []Toleration{
			{Key: "nvidia.com/gpu", Operator: "Exists", Effect: "NoSchedule"},
		},
	},
}

var defaultModelProvider = Provider{
	DefaultPort: 8080,
}

// GetModelProvider returns configuration for a given model provider name.
// Unknown providers get a sensible fallback.
func GetModelProvider(provider string) Provider {
	if p, ok := modelProviderRegistry[strings.ToLower(provider)]; ok {
		return p
	}
	return defaultModelProvider
}
