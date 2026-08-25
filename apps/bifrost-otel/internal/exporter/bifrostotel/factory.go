// Package bifrostotel implements an OpenTelemetry Collector traces exporter that
// turns Bifrost AI-gateway GenAI spans into Metronome usage events and posts them
// to Metronome's /v1/ingest API. It bills the final successful attempt per trace
// (never the sum of retry/fallback attempts) and relies on Metronome's 34-day
// transaction_id dedupe so the framework's at-least-once queue/retry is safe.
package bifrostotel

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	typeStr   = "bifrostotel"
	stability = component.StabilityLevelDevelopment
)

// NewFactory returns the factory for the bifrostotel exporter.
func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		exporter.WithTraces(createTraces, stability),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Endpoint:      "https://api.metronome.com",
		EventType:     "ai_gateway_llm_usage",
		TimeoutConfig: exporterhelper.NewDefaultTimeoutConfig(),
		QueueConfig:   configoptional.Default(exporterhelper.NewDefaultQueueConfig()),
		BackOffConfig: configretry.NewDefaultBackOffConfig(),
	}
}

func createTraces(
	ctx context.Context,
	set exporter.Settings,
	cfg component.Config,
) (exporter.Traces, error) {
	c := cfg.(*Config)
	e := newExporter(c, set)
	return exporterhelper.NewTraces(
		ctx, set, cfg, e.pushTraces,
		exporterhelper.WithStart(e.start),
		exporterhelper.WithTimeout(c.TimeoutConfig),
		exporterhelper.WithQueue(c.QueueConfig),
		exporterhelper.WithRetry(c.BackOffConfig),
	)
}
