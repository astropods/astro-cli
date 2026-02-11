package astro

import "go.opentelemetry.io/collector/component"

// Config holds the configuration for the astro processor.
type Config struct {
	// AgentName is the name of the Astro agent (from ASTRO_AGENT_NAME).
	AgentName string `mapstructure:"agent_name"`

	// AgentVersion is the version of the Astro agent (from ASTRO_AGENT_VERSION).
	AgentVersion string `mapstructure:"agent_version"`

	// DeploymentID is the unique deployment identifier (from ASTRO_DEPLOYMENT_ID).
	DeploymentID string `mapstructure:"deployment_id"`

	// RedactPrompts controls whether prompt content in span attributes is redacted.
	RedactPrompts bool `mapstructure:"redact_prompts"`
}

var _ component.Config = (*Config)(nil)

// Validate checks if the configuration is valid.
func (cfg *Config) Validate() error {
	return nil
}
