package astro

import (
	"context"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// Attribute keys added by the astro processor.
const (
	attrAgentName    = "astro.agent.name"
	attrAgentVersion = "astro.agent.version"
	attrDeploymentID = "astro.deployment.id"
	attrRedacted     = "astro.redacted"
)

// Span attribute prefixes that may contain sensitive prompt content.
var promptAttributePrefixes = []string{
	"gen_ai.prompt",
	"gen_ai.completion",
	"llm.prompts",
	"llm.completions",
}

type astroProcessor struct {
	logger      *zap.Logger
	cfg         *Config
	nextTraces  consumer.Traces
	nextMetrics consumer.Metrics
	nextLogs    consumer.Logs
}

func newProcessor(
	logger *zap.Logger,
	cfg *Config,
	nextTraces consumer.Traces,
	nextMetrics consumer.Metrics,
	nextLogs consumer.Logs,
) *astroProcessor {
	return &astroProcessor{
		logger:      logger,
		cfg:         cfg,
		nextTraces:  nextTraces,
		nextMetrics: nextMetrics,
		nextLogs:    nextLogs,
	}
}

// Start implements component.Component.
func (p *astroProcessor) Start(_ context.Context, _ component.Host) error {
	p.logger.Info("astro processor started",
		zap.String("agent_name", p.cfg.AgentName),
		zap.String("agent_version", p.cfg.AgentVersion),
		zap.Bool("redact_prompts", p.cfg.RedactPrompts),
	)
	return nil
}

// Shutdown implements component.Component.
func (p *astroProcessor) Shutdown(_ context.Context) error {
	return nil
}

// Capabilities implements processor.Traces/Metrics/Logs.
func (p *astroProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

// ConsumeTraces processes trace data by enriching resource attributes.
func (p *astroProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	if p.nextTraces == nil {
		return nil
	}
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)
		p.enrichResourceAttributes(rs.Resource().Attributes())

		if p.cfg.RedactPrompts {
			p.redactTraceSpans(rs)
		}
	}
	return p.nextTraces.ConsumeTraces(ctx, td)
}

// ConsumeMetrics processes metric data by enriching resource attributes.
func (p *astroProcessor) ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error {
	if p.nextMetrics == nil {
		return nil
	}
	for i := 0; i < md.ResourceMetrics().Len(); i++ {
		rm := md.ResourceMetrics().At(i)
		p.enrichResourceAttributes(rm.Resource().Attributes())
	}
	return p.nextMetrics.ConsumeMetrics(ctx, md)
}

// ConsumeLogs processes log data by enriching resource attributes.
func (p *astroProcessor) ConsumeLogs(ctx context.Context, ld plog.Logs) error {
	if p.nextLogs == nil {
		return nil
	}
	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		rl := ld.ResourceLogs().At(i)
		p.enrichResourceAttributes(rl.Resource().Attributes())
	}
	return p.nextLogs.ConsumeLogs(ctx, ld)
}

// enrichResourceAttributes adds Astro platform metadata to resource attributes.
func (p *astroProcessor) enrichResourceAttributes(attrs pcommon.Map) {
	attrs.PutStr(attrAgentName, p.cfg.AgentName)
	attrs.PutStr(attrAgentVersion, p.cfg.AgentVersion)
	attrs.PutStr(attrDeploymentID, p.cfg.DeploymentID)

	// Set langfuse.tags so Langfuse indexes them as filterable tags.
	tags := attrs.PutEmptySlice("langfuse.tags")
	tags.AppendEmpty().SetStr("deployment:" + p.cfg.DeploymentID)
}

// redactTraceSpans redacts sensitive prompt/completion content from span attributes.
func (p *astroProcessor) redactTraceSpans(rs ptrace.ResourceSpans) {
	for i := 0; i < rs.ScopeSpans().Len(); i++ {
		ss := rs.ScopeSpans().At(i)
		for j := 0; j < ss.Spans().Len(); j++ {
			span := ss.Spans().At(j)
			p.redactSpanAttributes(span.Attributes())
		}
	}
}

// redactSpanAttributes replaces sensitive attribute values with a redaction marker.
func (p *astroProcessor) redactSpanAttributes(attrs pcommon.Map) {
	attrs.Range(func(k string, _ pcommon.Value) bool {
		for _, prefix := range promptAttributePrefixes {
			if strings.HasPrefix(k, prefix) {
				attrs.PutStr(k, "[REDACTED]")
				attrs.PutBool(attrRedacted, true)
				break
			}
		}
		return true
	})
}
