package bifrostotel

import (
	"errors"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// Config configures the bifrostotel exporter, which maps Bifrost GenAI trace
// spans to Metronome usage events and posts them to Metronome's ingest API.
type Config struct {
	// APIKey is the Metronome bearer token (METRONOME_API_KEY).
	APIKey string `mapstructure:"api_key"`
	// Endpoint is the Metronome API base URL; /v1/ingest is appended.
	Endpoint string `mapstructure:"endpoint"`
	// EventType is the Metronome event_type stamped on every event.
	EventType string `mapstructure:"event_type"`

	// Standard exporterhelper blocks: timeout, persistent queue, and retry are
	// handled by the collector framework, not hand-rolled here.
	TimeoutConfig exporterhelper.TimeoutConfig                             `mapstructure:",squash"`
	QueueConfig   configoptional.Optional[exporterhelper.QueueBatchConfig] `mapstructure:"sending_queue"`
	BackOffConfig configretry.BackOffConfig                                `mapstructure:"retry_on_failure"`
}

var _ component.Config = (*Config)(nil)

// Validate checks required fields.
func (cfg *Config) Validate() error {
	if cfg.APIKey == "" {
		return errors.New("bifrostotel: api_key is required")
	}
	if cfg.Endpoint == "" {
		return errors.New("bifrostotel: endpoint is required")
	}
	if cfg.EventType == "" {
		return errors.New("bifrostotel: event_type is required")
	}
	return nil
}
