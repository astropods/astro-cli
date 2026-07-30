package ingest

import (
	"bytes"
	"testing"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func logRecord(body string, traceID, spanID []byte, attrs ...*commonpb.KeyValue) *logspb.LogRecord {
	return &logspb.LogRecord{
		TimeUnixNano: 123,
		Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: body}},
		TraceId:      traceID,
		SpanId:       spanID,
		Attributes:   attrs,
	}
}

func TestSynthesizeAssistantResponse(t *testing.T) {
	trace := []byte("0123456789abcdef")
	parent := []byte("01234567")
	rec := logRecord("claude_code.assistant_response", trace, parent,
		strAttr("response", "hi there"),
		strAttr("model", "claude-haiku-4-5"),
		strAttr("user.email", "dev@example.com"),
		strAttr("session.id", "sess-1"),
	)

	span := synthesizeSpan(rec, false)
	if span == nil {
		t.Fatal("expected a span")
	}
	if !bytes.Equal(span.TraceId, trace) {
		t.Fatalf("traceId not reused: %x", span.TraceId)
	}
	if !bytes.Equal(span.ParentSpanId, parent) {
		t.Fatalf("parentSpanId should be the interaction spanId: %x", span.ParentSpanId)
	}
	if bytes.Equal(span.SpanId, parent) || len(span.SpanId) != 8 {
		t.Fatalf("spanId should be freshly generated (8 bytes, != parent): %x", span.SpanId)
	}
	if span.Name != "assistant_response" {
		t.Fatalf("name: %q", span.Name)
	}
	if v := find(span.Attributes, attrObsType); v == nil || v.Value.GetStringValue() != obsTypeGeneration {
		t.Fatalf("observation type not generation: %+v", v)
	}
	if v := find(span.Attributes, attrObsOutput); v == nil || v.Value.GetStringValue() != "hi there" {
		t.Fatalf("observation output missing: %+v", v)
	}
	if v := find(span.Attributes, attrTraceOutput); v == nil || v.Value.GetStringValue() != "hi there" {
		t.Fatalf("trace output missing: %+v", v)
	}
	if v := find(span.Attributes, attrObsModel); v == nil || v.Value.GetStringValue() != "claude-haiku-4-5" {
		t.Fatalf("model missing: %+v", v)
	}
	if v := find(span.Attributes, attrLangfuseUserID); v == nil || v.Value.GetStringValue() != "dev@example.com" {
		t.Fatalf("langfuse.user.id not mapped: %+v", v)
	}
	if v := find(span.Attributes, attrTags); v == nil {
		t.Fatal("claude-code tag missing")
	}
}

func TestSynthesizeUserPrompt(t *testing.T) {
	span := synthesizeSpan(logRecord("claude_code.user_prompt", []byte("0123456789abcdef"), []byte("01234567"),
		strAttr("prompt", "do the thing")), false)
	if span == nil {
		t.Fatal("expected a span")
	}
	if v := find(span.Attributes, attrObsInput); v == nil || v.Value.GetStringValue() != "do the thing" {
		t.Fatalf("observation input missing: %+v", v)
	}
	if v := find(span.Attributes, attrTraceInput); v == nil || v.Value.GetStringValue() != "do the thing" {
		t.Fatalf("trace input missing: %+v", v)
	}
}

func TestSynthesizeSkipsEmptyContent(t *testing.T) {
	// Redacted at the source: event present, no content attribute.
	if s := synthesizeSpan(logRecord("claude_code.assistant_response", []byte("0123456789abcdef"), nil), false); s != nil {
		t.Fatal("expected nil span for content-less record")
	}
	// Uninteresting event type.
	if s := synthesizeSpan(logRecord("claude_code.api_request", []byte("0123456789abcdef"), nil,
		strAttr("prompt.id", "x")), false); s != nil {
		t.Fatal("expected nil span for non-content event")
	}
}

func TestSynthesizeRedactStripsContent(t *testing.T) {
	span := synthesizeSpan(logRecord("claude_code.assistant_response", []byte("0123456789abcdef"), []byte("01234567"),
		strAttr("response", "secret")), true)
	if span == nil {
		t.Fatal("span still emitted when redacting")
	}
	if find(span.Attributes, attrObsOutput) != nil || find(span.Attributes, attrTraceOutput) != nil {
		t.Fatal("redaction should strip response content from the span")
	}
}

func TestTransformLogsToTraces(t *testing.T) {
	trace := []byte("0123456789abcdef")
	in := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{
					logRecord("claude_code.user_prompt", trace, []byte("01234567"), strAttr("prompt", "q")),
					logRecord("claude_code.assistant_response", trace, []byte("01234567"), strAttr("response", "a")),
					logRecord("claude_code.api_request", trace, []byte("01234567"), strAttr("prompt.id", "x")),
				},
			}},
		}},
	}
	out := transformLogsToTraces(in, "acct-1", false, nil)
	if out == nil {
		t.Fatal("expected a traces request")
	}
	rs := out.GetResourceSpans()
	if len(rs) != 1 {
		t.Fatalf("expected 1 resourceSpans, got %d", len(rs))
	}
	if kv := find(rs[0].GetResource().GetAttributes(), attrAccountID); kv == nil || kv.Value.GetStringValue() != "acct-1" {
		t.Fatalf("account id not stamped on resource: %+v", rs[0].GetResource())
	}
	spans := rs[0].GetScopeSpans()[0].GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 content spans (prompt+response), got %d", len(spans))
	}
}

func TestTransformLogsToTracesEmpty(t *testing.T) {
	in := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{
					logRecord("claude_code.api_request", []byte("0123456789abcdef"), nil, strAttr("prompt.id", "x")),
				},
			}},
		}},
	}
	if out := transformLogsToTraces(in, "acct-1", false, nil); out != nil {
		t.Fatal("expected nil when no content-bearing records")
	}
}

func TestTransformLogsToTracesExcludesEmail(t *testing.T) {
	trace := []byte("0123456789abcdef")
	rl := func(email string) *logspb.ResourceLogs {
		return &logspb.ResourceLogs{
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{
					logRecord("claude_code.user_prompt", trace, []byte("01234567"),
						strAttr("prompt", "q"), strAttr("user.email", email)),
				},
			}},
		}
	}
	in := &collogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{
		rl("Excluded@Example.com"), // matches case-insensitively
		rl("kept@example.com"),
	}}
	excluded := map[string]struct{}{"excluded@example.com": {}}

	out := transformLogsToTraces(in, "acct-1", false, excluded)
	if out == nil {
		t.Fatal("expected a traces request for the non-excluded record")
	}
	spans := out.GetResourceSpans()[0].GetScopeSpans()[0].GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected only the non-excluded record to survive, got %d spans", len(spans))
	}
	if v := find(spans[0].Attributes, attrLangfuseUserID); v == nil || v.Value.GetStringValue() != "kept@example.com" {
		t.Fatalf("surviving span is not the kept user: %+v", v)
	}
}

func TestTransformLogsToTracesExcludesViaResourceEmail(t *testing.T) {
	trace := []byte("0123456789abcdef")
	in := &collogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{
		Resource:  &resourcepb.Resource{Attributes: []*commonpb.KeyValue{strAttr("user.email", "excluded@example.com")}},
		ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{logRecord("claude_code.user_prompt", trace, []byte("01234567"), strAttr("prompt", "q"))}}},
	}}}
	excluded := map[string]struct{}{"excluded@example.com": {}}

	if out := transformLogsToTraces(in, "acct-1", false, excluded); out != nil {
		t.Fatal("expected nil: the only record's resource email is excluded")
	}
}
