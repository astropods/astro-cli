package spec

import "strings"

// PortDef defines a named port.
type PortDef struct {
	Name string
	Port int
}

// Provider holds provider-specific configuration.
type Provider struct {
	Image       string    // default container image (e.g., "qdrant/qdrant:latest")
	DefaultPort int       // primary port (6333 for qdrant, 6379 for redis, etc.)
	ExtraPorts  []PortDef // additional named ports (e.g., gRPC 6334 for qdrant)
	MountPath   string    // volume mount path for persistent data
	EnvPrefix   string    // env var prefix ("QDRANT", "REDIS", etc.)
	URLScheme   string    // connection URL scheme ("http", "redis")
	HealthCheck []string  // exec health check command; nil → use HealthPath instead
	HealthPath  string    // HTTP health check path (e.g., "/healthz")
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
}

var defaultProvider = Provider{
	DefaultPort: 6333,
	MountPath:   "/data",
}

// GetProvider returns configuration for a given provider name.
// Unknown providers get a sensible fallback.
func GetProvider(provider string) Provider {
	if p, ok := providerRegistry[strings.ToLower(provider)]; ok {
		return p
	}
	return defaultProvider
}
