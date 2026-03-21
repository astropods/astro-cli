package astro

import (
	"context"
	"encoding/json"
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

// promptAttributePrefixes lists attribute name prefixes that may contain
// sensitive prompt/completion content and should be redacted when enabled.
var promptAttributePrefixes = []string{
	"gen_ai.prompt",
	"gen_ai.completion",
	"gen_ai.input",
	"gen_ai.output",
	"llm.prompts",
	"llm.completions",
	"langfuse.observation.input",
	"langfuse.observation.output",
	"langfuse.trace.input",
	"langfuse.trace.output",
}

// promptAttributeSuffixes lists attribute name suffixes that, when combined
// with the mastra.* prefix, indicate sensitive content (e.g. mastra.agent_run.input).
var promptAttributeSuffixes = []string{
	".input",
	".output",
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

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				attrs := span.Attributes()
				p.setLangfuseTags(attrs)
				mapMastraAttributes(attrs)
				mapMastraUserSession(attrs)
				isRoot := span.ParentSpanID().IsEmpty()
				if isRoot {
					mapMastraTraceIO(attrs)
				}
			}
		}

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
}

// setLangfuseTags sets langfuse.trace.tags on span attributes so Langfuse
// indexes them as filterable tags. Includes the deployment tag and merges in
// any agent-defined tags from mastra.tags (a JSON-stringified string array
// set by @mastra/otel-exporter on root spans).
func (p *astroProcessor) setLangfuseTags(attrs pcommon.Map) {
	tags := attrs.PutEmptySlice("langfuse.trace.tags")
	tags.AppendEmpty().SetStr("deployment:" + p.cfg.DeploymentID)

	if mastraTags, ok := attrs.Get("mastra.tags"); ok {
		var parsed []string
		if err := json.Unmarshal([]byte(mastraTags.Str()), &parsed); err == nil {
			for _, t := range parsed {
				tags.AppendEmpty().SetStr(t)
			}
		}
	}
}

// Mastra's OTel exporter (@mastra/otel-exporter) stores input/output in
// "mastra.{span_type}.input" / "mastra.{span_type}.output" for non-generation
// span types (agent_run, workflow_run, workflow_step, generic, etc.).
// Langfuse's OTEL ingestion only recognises langfuse.observation.input/output,
// gen_ai.*, input.value/output.value, or mlflow.* — so the Mastra attributes
// are silently dropped into metadata.attributes.
//
// MODEL_GENERATION spans already use gen_ai.input.messages / gen_ai.output.messages
// which Langfuse recognises, so they don't need mapping.
var mastraInputOutputSuffixes = []struct {
	suffix string
	dst    string
}{
	{".input", "langfuse.observation.input"},
	{".output", "langfuse.observation.output"},
}

const mastraAttrPrefix = "mastra."

// mapMastraAttributes copies Mastra-specific input/output attributes to
// Langfuse-recognized attribute names when the destination is not already set.
// It matches any "mastra.*.input" / "mastra.*.output" attribute, covering all
// Mastra span types (agent_run, workflow_run, workflow_step, generic, etc.).
func mapMastraAttributes(attrs pcommon.Map) {
	// Check if langfuse destinations are already set — skip early if so.
	inputSet := false
	outputSet := false
	for _, m := range mastraInputOutputSuffixes {
		if _, exists := attrs.Get(m.dst); exists {
			if m.dst == "langfuse.observation.input" {
				inputSet = true
			} else {
				outputSet = true
			}
		}
	}
	if inputSet && outputSet {
		return
	}

	attrs.Range(func(k string, v pcommon.Value) bool {
		if !strings.HasPrefix(k, mastraAttrPrefix) {
			return true
		}
		for _, m := range mastraInputOutputSuffixes {
			if strings.HasSuffix(k, m.suffix) {
				alreadySet := (m.dst == "langfuse.observation.input" && inputSet) ||
					(m.dst == "langfuse.observation.output" && outputSet)
				if !alreadySet {
					v.CopyTo(attrs.PutEmpty(m.dst))
					if m.dst == "langfuse.observation.input" {
						inputSet = true
					} else {
						outputSet = true
					}
				}
				break
			}
		}
		return true
	})
}

// mastraUserSessionMappings maps Mastra metadata attributes to Langfuse
// trace-level user/session fields so Langfuse's filtering UI works.
// Mastra's OTel exporter sends resourceId for the user identity and threadId
// for the conversation session. It may also send userId/sessionId if explicitly
// set in agent metadata. We check both, preferring the explicit names.
var mastraUserSessionMappings = []struct {
	src string
	dst string
}{
	{"mastra.metadata.userId", "langfuse.user.id"},
	{"mastra.metadata.resourceId", "langfuse.user.id"},
	{"mastra.metadata.sessionId", "langfuse.session.id"},
	{"mastra.metadata.threadId", "langfuse.session.id"},
	{"gen_ai.conversation.id", "langfuse.session.id"},
}

// mapMastraUserSession maps Mastra user/session metadata to Langfuse attributes.
// Checks userId before resourceId and sessionId before threadId so explicit
// values take priority. Won't overwrite if the destination is already set.
func mapMastraUserSession(attrs pcommon.Map) {
	for _, m := range mastraUserSessionMappings {
		if src, ok := attrs.Get(m.src); ok {
			if _, exists := attrs.Get(m.dst); !exists {
				src.CopyTo(attrs.PutEmpty(m.dst))
			}
		}
	}
}

// mapMastraTraceIO sets langfuse.trace.input/output from the observation-level
// input/output on root spans. This ensures the Langfuse trace record shows
// the agent's top-level input/output, not just the observation.
// Must be called after mapMastraAttributes so langfuse.observation.* is set.
func mapMastraTraceIO(attrs pcommon.Map) {
	if src, ok := attrs.Get("langfuse.observation.input"); ok {
		if _, exists := attrs.Get("langfuse.trace.input"); !exists {
			src.CopyTo(attrs.PutEmpty("langfuse.trace.input"))
		}
	}
	if src, ok := attrs.Get("langfuse.observation.output"); ok {
		if _, exists := attrs.Get("langfuse.trace.output"); !exists {
			src.CopyTo(attrs.PutEmpty("langfuse.trace.output"))
		}
	}
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
		if isSensitiveAttribute(k) {
			attrs.PutStr(k, "[REDACTED]")
			attrs.PutBool(attrRedacted, true)
		}
		return true
	})
}

// isSensitiveAttribute returns true if the attribute key matches known
// prompt/completion patterns (by prefix or by mastra.*.input/output suffix).
func isSensitiveAttribute(k string) bool {
	for _, prefix := range promptAttributePrefixes {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	if strings.HasPrefix(k, mastraAttrPrefix) {
		for _, s := range promptAttributeSuffixes {
			if strings.HasSuffix(k, s) {
				return true
			}
		}
	}
	return false
}
