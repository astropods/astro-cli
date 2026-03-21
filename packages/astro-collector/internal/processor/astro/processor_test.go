package astro

import (
	"context"
	"testing"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// captureTraces is a consumer.Traces that captures the last batch for assertions.
type captureTraces struct {
	last ptrace.Traces
}

func (c *captureTraces) ConsumeTraces(_ context.Context, td ptrace.Traces) error {
	c.last = td
	return nil
}
func (c *captureTraces) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// newTestProcessor creates a processor with sensible defaults and the given config overrides.
func newTestProcessor(cfg *Config, next consumer.Traces) *astroProcessor {
	return newProcessor(zap.NewNop(), cfg, next, nil, nil)
}

// buildSingleSpanTrace creates a Traces payload with one resource span containing one span.
// If parentID is non-empty the span is given a parent (making it a child span).
func buildSingleSpanTrace(spanName string, parentID string) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName(spanName)
	span.SetTraceID(pcommon.TraceID([16]byte{1}))
	span.SetSpanID(pcommon.SpanID([8]byte{1}))
	if parentID != "" {
		span.SetParentSpanID(pcommon.SpanID([8]byte{2}))
	}
	return td
}

func TestEnrichResourceAttributes(t *testing.T) {
	cfg := &Config{
		AgentName:    "my-agent",
		AgentVersion: "v1.2.3",
		DeploymentID: "deploy-abc",
	}
	capture := &captureTraces{}
	p := newTestProcessor(cfg, capture)

	td := buildSingleSpanTrace("test-span", "")
	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces failed: %v", err)
	}

	attrs := capture.last.ResourceSpans().At(0).Resource().Attributes()

	tests := []struct {
		key  string
		want string
	}{
		{attrAgentName, "my-agent"},
		{attrAgentVersion, "v1.2.3"},
		{attrDeploymentID, "deploy-abc"},
	}
	for _, tt := range tests {
		v, ok := attrs.Get(tt.key)
		if !ok {
			t.Errorf("expected attribute %q to be set", tt.key)
			continue
		}
		if v.Str() != tt.want {
			t.Errorf("%s: expected %q, got %q", tt.key, tt.want, v.Str())
		}
	}

	// Verify langfuse.trace.tags are set on span attributes for Langfuse filtering
	spanAttrs := capture.last.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	tagsVal, ok := spanAttrs.Get("langfuse.trace.tags")
	if !ok {
		t.Fatal("expected langfuse.trace.tags to be set on span")
	}
	tagsSlice := tagsVal.Slice()
	wantTags := []string{"deployment:deploy-abc"}
	if tagsSlice.Len() != len(wantTags) {
		t.Fatalf("expected %d tags, got %d", len(wantTags), tagsSlice.Len())
	}
	for i, want := range wantTags {
		if tagsSlice.At(i).Str() != want {
			t.Errorf("tag[%d]: expected %q, got %q", i, want, tagsSlice.At(i).Str())
		}
	}
}

func TestEnrichResourceAttributes_SpansUntouched(t *testing.T) {
	cfg := &Config{
		AgentName:    "my-agent",
		AgentVersion: "v1.0",
		DeploymentID: "dep-123",
	}
	capture := &captureTraces{}
	p := newTestProcessor(cfg, capture)

	td := buildSingleSpanTrace("llm-call", "")
	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces failed: %v", err)
	}

	span := capture.last.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)

	// Span name should be unchanged — no span-level enrichment
	if span.Name() != "llm-call" {
		t.Errorf("span name should be unchanged, got %q", span.Name())
	}

	// No extra span attributes injected
	if _, ok := span.Attributes().Get("external_id"); ok {
		t.Error("span should not have external_id")
	}
	if _, ok := span.Attributes().Get("tags"); ok {
		t.Error("span should not have tags")
	}
}

func TestRedactPrompts(t *testing.T) {
	cfg := &Config{
		AgentName:     "agent",
		AgentVersion:  "v1",
		DeploymentID:  "dep-1",
		RedactPrompts: true,
	}
	capture := &captureTraces{}
	p := newTestProcessor(cfg, capture)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("llm")
	span.SetTraceID(pcommon.TraceID([16]byte{1}))
	span.SetSpanID(pcommon.SpanID([8]byte{1}))
	span.Attributes().PutStr("gen_ai.prompt.0", "secret prompt")
	span.Attributes().PutStr("gen_ai.completion.0", "secret completion")
	span.Attributes().PutStr("safe_attr", "not redacted")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces failed: %v", err)
	}

	outSpan := capture.last.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	attrs := outSpan.Attributes()

	prompt, _ := attrs.Get("gen_ai.prompt.0")
	if prompt.Str() != "[REDACTED]" {
		t.Errorf("gen_ai.prompt.0 should be redacted, got %q", prompt.Str())
	}

	completion, _ := attrs.Get("gen_ai.completion.0")
	if completion.Str() != "[REDACTED]" {
		t.Errorf("gen_ai.completion.0 should be redacted, got %q", completion.Str())
	}

	safe, _ := attrs.Get("safe_attr")
	if safe.Str() != "not redacted" {
		t.Errorf("safe_attr should not be redacted, got %q", safe.Str())
	}

	redacted, ok := attrs.Get(attrRedacted)
	if !ok || !redacted.Bool() {
		t.Error("expected astro.redacted=true")
	}
}

func TestRedactPrompts_Disabled(t *testing.T) {
	cfg := &Config{
		AgentName:     "agent",
		AgentVersion:  "v1",
		DeploymentID:  "dep-1",
		RedactPrompts: false,
	}
	capture := &captureTraces{}
	p := newTestProcessor(cfg, capture)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("llm")
	span.SetTraceID(pcommon.TraceID([16]byte{1}))
	span.SetSpanID(pcommon.SpanID([8]byte{1}))
	span.Attributes().PutStr("gen_ai.prompt.0", "visible prompt")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces failed: %v", err)
	}

	outSpan := capture.last.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	prompt, _ := outSpan.Attributes().Get("gen_ai.prompt.0")
	if prompt.Str() != "visible prompt" {
		t.Errorf("prompt should not be redacted when RedactPrompts=false, got %q", prompt.Str())
	}
}

func TestRedactPrompts_AllPrefixes(t *testing.T) {
	cfg := &Config{
		AgentName:     "agent",
		AgentVersion:  "v1",
		DeploymentID:  "dep-1",
		RedactPrompts: true,
	}
	capture := &captureTraces{}
	p := newTestProcessor(cfg, capture)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("llm")
	span.SetTraceID(pcommon.TraceID([16]byte{1}))
	span.SetSpanID(pcommon.SpanID([8]byte{1}))
	span.Attributes().PutStr("gen_ai.prompt.0", "p1")
	span.Attributes().PutStr("gen_ai.completion.0", "c1")
	span.Attributes().PutStr("llm.prompts.0", "p2")
	span.Attributes().PutStr("llm.completions.0", "c2")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces failed: %v", err)
	}

	outSpan := capture.last.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	for _, key := range []string{"gen_ai.prompt.0", "gen_ai.completion.0", "llm.prompts.0", "llm.completions.0"} {
		v, ok := outSpan.Attributes().Get(key)
		if !ok {
			t.Errorf("expected attribute %q", key)
			continue
		}
		if v.Str() != "[REDACTED]" {
			t.Errorf("%s should be redacted, got %q", key, v.Str())
		}
	}
}

func TestMapMastraAttributes(t *testing.T) {
	// Test that all Mastra span types get mapped to langfuse.observation.*
	for _, spanType := range []string{"agent_run", "workflow_run", "workflow_step", "generic"} {
		t.Run(spanType, func(t *testing.T) {
			cfg := &Config{
				AgentName:    "agent",
				AgentVersion: "v1",
				DeploymentID: "dep-1",
			}
			capture := &captureTraces{}
			p := newTestProcessor(cfg, capture)

			td := ptrace.NewTraces()
			rs := td.ResourceSpans().AppendEmpty()
			ss := rs.ScopeSpans().AppendEmpty()
			span := ss.Spans().AppendEmpty()
			span.SetName(spanType)
			span.SetTraceID(pcommon.TraceID([16]byte{1}))
			span.SetSpanID(pcommon.SpanID([8]byte{1}))
			span.Attributes().PutStr("mastra."+spanType+".input", "the input")
			span.Attributes().PutStr("mastra."+spanType+".output", `{"text":"hello"}`)

			if err := p.ConsumeTraces(context.Background(), td); err != nil {
				t.Fatalf("ConsumeTraces failed: %v", err)
			}

			attrs := capture.last.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()

			// Original attributes preserved
			v, ok := attrs.Get("mastra." + spanType + ".input")
			if !ok || v.Str() != "the input" {
				t.Errorf("expected original mastra.%s.input preserved", spanType)
			}

			// Mapped to langfuse attributes
			v, ok = attrs.Get("langfuse.observation.input")
			if !ok {
				t.Fatal("expected langfuse.observation.input to be set")
			}
			if v.Str() != "the input" {
				t.Errorf("langfuse.observation.input: expected %q, got %q", "the input", v.Str())
			}

			v, ok = attrs.Get("langfuse.observation.output")
			if !ok {
				t.Fatal("expected langfuse.observation.output to be set")
			}
			if v.Str() != `{"text":"hello"}` {
				t.Errorf("langfuse.observation.output: expected %q, got %q", `{"text":"hello"}`, v.Str())
			}
		})
	}
}

func TestMapMastraAttributes_NoOverwrite(t *testing.T) {
	cfg := &Config{
		AgentName:    "agent",
		AgentVersion: "v1",
		DeploymentID: "dep-1",
	}
	capture := &captureTraces{}
	p := newTestProcessor(cfg, capture)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("agent_run")
	span.SetTraceID(pcommon.TraceID([16]byte{1}))
	span.SetSpanID(pcommon.SpanID([8]byte{1}))
	span.Attributes().PutStr("mastra.agent_run.input", "mastra input")
	span.Attributes().PutStr("langfuse.observation.input", "already set")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces failed: %v", err)
	}

	attrs := capture.last.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	v, _ := attrs.Get("langfuse.observation.input")
	if v.Str() != "already set" {
		t.Errorf("should not overwrite existing langfuse attribute, got %q", v.Str())
	}
}

func TestMapMastraAttributes_NoMastraAttrs(t *testing.T) {
	cfg := &Config{
		AgentName:    "agent",
		AgentVersion: "v1",
		DeploymentID: "dep-1",
	}
	capture := &captureTraces{}
	p := newTestProcessor(cfg, capture)

	td := buildSingleSpanTrace("non-mastra-span", "")
	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces failed: %v", err)
	}

	attrs := capture.last.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	if _, ok := attrs.Get("langfuse.observation.input"); ok {
		t.Error("should not set langfuse.observation.input when no mastra attributes present")
	}
	if _, ok := attrs.Get("langfuse.observation.output"); ok {
		t.Error("should not set langfuse.observation.output when no mastra attributes present")
	}
}

func TestRedactPrompts_MastraAttributes(t *testing.T) {
	cfg := &Config{
		AgentName:     "agent",
		AgentVersion:  "v1",
		DeploymentID:  "dep-1",
		RedactPrompts: true,
	}
	capture := &captureTraces{}
	p := newTestProcessor(cfg, capture)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("agent")
	span.SetTraceID(pcommon.TraceID([16]byte{1}))
	span.SetSpanID(pcommon.SpanID([8]byte{1}))
	span.Attributes().PutStr("mastra.agent_run.input", "secret input")
	span.Attributes().PutStr("mastra.agent_run.output", "secret output")
	span.Attributes().PutStr("mastra.workflow_run.input", "wf input")
	span.Attributes().PutStr("mastra.span.type", "agent_run") // not input/output, should NOT be redacted
	span.Attributes().PutStr("safe_attr", "visible")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces failed: %v", err)
	}

	attrs := capture.last.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()

	for _, key := range []string{"mastra.agent_run.input", "mastra.agent_run.output", "mastra.workflow_run.input"} {
		v, ok := attrs.Get(key)
		if !ok {
			t.Errorf("expected attribute %q", key)
			continue
		}
		if v.Str() != "[REDACTED]" {
			t.Errorf("%s should be redacted, got %q", key, v.Str())
		}
	}

	// langfuse.observation.input/output should also be redacted (set by mapMastraAttributes, then redacted)
	for _, key := range []string{"langfuse.observation.input", "langfuse.observation.output"} {
		v, ok := attrs.Get(key)
		if !ok {
			t.Errorf("expected attribute %q", key)
			continue
		}
		if v.Str() != "[REDACTED]" {
			t.Errorf("%s should be redacted, got %q", key, v.Str())
		}
	}

	// Non-input/output mastra attributes should NOT be redacted
	v, _ := attrs.Get("mastra.span.type")
	if v.Str() != "agent_run" {
		t.Errorf("mastra.span.type should not be redacted, got %q", v.Str())
	}

	v, _ = attrs.Get("safe_attr")
	if v.Str() != "visible" {
		t.Errorf("safe_attr should not be redacted, got %q", v.Str())
	}
}

func TestConsumeTraces_NilNext(t *testing.T) {
	cfg := &Config{AgentName: "agent", AgentVersion: "v1", DeploymentID: "dep-1"}
	p := newTestProcessor(cfg, nil) // nil nextTraces

	td := buildSingleSpanTrace("test", "")
	err := p.ConsumeTraces(context.Background(), td)
	if err != nil {
		t.Errorf("expected nil error with nil nextTraces, got %v", err)
	}
}

func TestEnrichResourceAttributes_MultipleResourceSpans(t *testing.T) {
	cfg := &Config{
		AgentName:    "agent",
		AgentVersion: "v2",
		DeploymentID: "dep-multi",
	}
	capture := &captureTraces{}
	p := newTestProcessor(cfg, capture)

	td := ptrace.NewTraces()
	// Add two resource spans
	rs1 := td.ResourceSpans().AppendEmpty()
	rs1.ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetName("span1")
	rs2 := td.ResourceSpans().AppendEmpty()
	rs2.ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetName("span2")

	if err := p.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("ConsumeTraces failed: %v", err)
	}

	// Both resource spans should have enriched attributes
	for i := 0; i < capture.last.ResourceSpans().Len(); i++ {
		attrs := capture.last.ResourceSpans().At(i).Resource().Attributes()
		v, ok := attrs.Get(attrAgentName)
		if !ok || v.Str() != "agent" {
			t.Errorf("resource span %d: expected agent name 'agent', got %q", i, v.Str())
		}
		v, ok = attrs.Get(attrDeploymentID)
		if !ok || v.Str() != "dep-multi" {
			t.Errorf("resource span %d: expected deployment ID 'dep-multi', got %q", i, v.Str())
		}
	}
}
