package ingest

import (
	"encoding/json"
	"testing"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func strAttr(key, val string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{
		Value: &commonpb.AnyValue_StringValue{StringValue: val},
	}}
}

func find(attrs []*commonpb.KeyValue, key string) *commonpb.KeyValue {
	for _, kv := range attrs {
		if kv.Key == key {
			return kv
		}
	}
	return nil
}

func TestRedact(t *testing.T) {
	in := []*commonpb.KeyValue{
		strAttr("gen_ai.prompt.0.content", "secret"),
		strAttr("gen_ai.completion.0.content", "secret"),
		strAttr("langfuse.trace.input", "secret"),
		strAttr("gen_ai.request.model", "keep-me"),
		strAttr("service.name", "keep-me"),
	}
	out := redact(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 attrs after redaction, got %d", len(out))
	}
	if find(out, "gen_ai.request.model") == nil || find(out, "service.name") == nil {
		t.Fatal("redaction dropped a non-sensitive attribute")
	}
	if find(out, "gen_ai.prompt.0.content") != nil {
		t.Fatal("redaction kept a prompt attribute")
	}
}

func TestStampIdentity(t *testing.T) {
	out := stampIdentity(nil, "acct-123")
	if kv := find(out, attrAccountID); kv == nil || kv.Value.GetStringValue() != "acct-123" {
		t.Fatalf("account id not stamped: %+v", out)
	}
	if kv := find(out, attrSource); kv == nil || kv.Value.GetStringValue() != sourceValue {
		t.Fatalf("source not stamped: %+v", out)
	}

	// Upsert replaces rather than duplicates.
	out = stampIdentity(out, "acct-456")
	n := 0
	for _, kv := range out {
		if kv.Key == attrAccountID {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected 1 account_id attr after upsert, got %d", n)
	}
	if find(out, attrAccountID).Value.GetStringValue() != "acct-456" {
		t.Fatal("upsert did not replace account id")
	}
}

func TestTagClaudeCode(t *testing.T) {
	out := tagClaudeCode(nil)
	kv := find(out, attrTags)
	if kv == nil {
		t.Fatal("langfuse.tags not set")
	}
	arr := kv.Value.GetArrayValue()
	if arr == nil || len(arr.Values) != 1 || arr.Values[0].GetStringValue() != sourceValue {
		t.Fatalf("unexpected tags value: %+v", kv.Value)
	}
}

func TestMapLangfuseIdentity(t *testing.T) {
	out := mapLangfuseIdentity([]*commonpb.KeyValue{
		strAttr("user.email", "dev@example.com"),
		strAttr("user.id", "hashed"),
		strAttr("session.id", "sess-1"),
	})
	if kv := find(out, attrLangfuseUserID); kv == nil || kv.Value.GetStringValue() != "dev@example.com" {
		t.Fatalf("langfuse.user.id not mapped from user.email: %+v", out)
	}
	if kv := find(out, attrLangfuseSessID); kv == nil || kv.Value.GetStringValue() != "sess-1" {
		t.Fatalf("langfuse.session.id not mapped from session.id: %+v", out)
	}

	// Re-mapping upserts rather than duplicating.
	out = mapLangfuseIdentity(out)
	n := 0
	for _, kv := range out {
		if kv.Key == attrLangfuseUserID {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected 1 langfuse.user.id after re-map, got %d", n)
	}
}

func TestMapLangfuseIdentityAbsent(t *testing.T) {
	out := mapLangfuseIdentity([]*commonpb.KeyValue{strAttr("service.name", "svc")})
	if find(out, attrLangfuseUserID) != nil || find(out, attrLangfuseSessID) != nil {
		t.Fatalf("added langfuse identity keys with no source attrs: %+v", out)
	}
}

func TestStripDatapointAttrCoversEveryDatapointKind(t *testing.T) {
	withAttrs := func() []*commonpb.KeyValue {
		return []*commonpb.KeyValue{strAttr(sessionIDKey, "sess-1"), strAttr("model", "sonnet")}
	}
	metrics := []*metricspb.Metric{
		{Name: "gauge", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
			DataPoints: []*metricspb.NumberDataPoint{{Attributes: withAttrs()}}}}},
		{Name: "sum", Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
			DataPoints: []*metricspb.NumberDataPoint{{Attributes: withAttrs()}}}}},
		{Name: "histogram", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{
			DataPoints: []*metricspb.HistogramDataPoint{{Attributes: withAttrs()}}}}},
		{Name: "exphistogram", Data: &metricspb.Metric_ExponentialHistogram{
			ExponentialHistogram: &metricspb.ExponentialHistogram{
				DataPoints: []*metricspb.ExponentialHistogramDataPoint{{Attributes: withAttrs()}}}}},
		{Name: "summary", Data: &metricspb.Metric_Summary{Summary: &metricspb.Summary{
			DataPoints: []*metricspb.SummaryDataPoint{{Attributes: withAttrs()}}}}},
	}
	req := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource:     &resourcepb.Resource{Attributes: withAttrs()},
			ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: metrics}},
		}},
	}

	stripDatapointAttr(req, sessionIDKey)

	got := map[string][]*commonpb.KeyValue{}
	for _, m := range req.ResourceMetrics[0].ScopeMetrics[0].Metrics {
		switch d := m.GetData().(type) {
		case *metricspb.Metric_Gauge:
			got[m.Name] = d.Gauge.DataPoints[0].Attributes
		case *metricspb.Metric_Sum:
			got[m.Name] = d.Sum.DataPoints[0].Attributes
		case *metricspb.Metric_Histogram:
			got[m.Name] = d.Histogram.DataPoints[0].Attributes
		case *metricspb.Metric_ExponentialHistogram:
			got[m.Name] = d.ExponentialHistogram.DataPoints[0].Attributes
		case *metricspb.Metric_Summary:
			got[m.Name] = d.Summary.DataPoints[0].Attributes
		}
	}
	if len(got) != len(metrics) {
		t.Fatalf("expected %d datapoint kinds, read back %d", len(metrics), len(got))
	}
	for name, attrs := range got {
		if find(attrs, sessionIDKey) != nil {
			t.Errorf("%s: session.id survived on the datapoint", name)
		}
		if find(attrs, "model") == nil {
			t.Errorf("%s: dropped an unrelated attribute", name)
		}
	}
}

func TestDropAttr(t *testing.T) {
	in := []*commonpb.KeyValue{
		strAttr("session.id", "abc"),
		strAttr("service.name", "svc"),
	}
	out := dropAttr(in, sessionIDKey)
	if len(out) != 1 || find(out, sessionIDKey) != nil {
		t.Fatalf("session.id not dropped: %+v", out)
	}
}

func TestEmailExcluded(t *testing.T) {
	set := map[string]struct{}{"dev@example.com": {}}
	if !emailExcluded([]*commonpb.KeyValue{strAttr("user.email", "Dev@Example.com")}, set) {
		t.Fatal("expected case-insensitive match")
	}
	if emailExcluded([]*commonpb.KeyValue{strAttr("user.email", "other@example.com")}, set) {
		t.Fatal("non-listed email should not be excluded")
	}
	if emailExcluded([]*commonpb.KeyValue{strAttr("service.name", "svc")}, set) {
		t.Fatal("no user.email should not be excluded")
	}
	if emailExcluded([]*commonpb.KeyValue{strAttr("user.email", "dev@example.com")}, nil) {
		t.Fatal("empty exclusion set should exclude nobody")
	}
}

// spanWithEmail builds a one-span ResourceSpans; resourceEmail/spanEmail are
// stamped where non-empty.
func resourceSpans(resourceEmail string, spanEmails ...string) *tracepb.ResourceSpans {
	var res *resourcepb.Resource
	if resourceEmail != "" {
		res = &resourcepb.Resource{Attributes: []*commonpb.KeyValue{strAttr("user.email", resourceEmail)}}
	}
	spans := make([]*tracepb.Span, 0, len(spanEmails))
	for _, e := range spanEmails {
		var attrs []*commonpb.KeyValue
		if e != "" {
			attrs = []*commonpb.KeyValue{strAttr("user.email", e)}
		}
		spans = append(spans, &tracepb.Span{Attributes: attrs})
	}
	return &tracepb.ResourceSpans{Resource: res, ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}}}
}

func spanCount(req *coltracepb.ExportTraceServiceRequest) int {
	n := 0
	for _, rs := range req.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			n += len(ss.GetSpans())
		}
	}
	return n
}

func TestDropExcludedSpans(t *testing.T) {
	excluded := map[string]struct{}{"excluded@example.com": {}}

	t.Run("empty set keeps everything", func(t *testing.T) {
		req := &coltracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{resourceSpans("", "a@x.com")}}
		if !dropExcludedSpans(req, nil) || spanCount(req) != 1 {
			t.Fatalf("nil set should keep all spans, got %d", spanCount(req))
		}
	})

	t.Run("drops excluded span keeps others", func(t *testing.T) {
		req := &coltracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{
			resourceSpans("", "excluded@example.com", "kept@example.com"),
		}}
		if !dropExcludedSpans(req, excluded) {
			t.Fatal("expected survivors")
		}
		if spanCount(req) != 1 {
			t.Fatalf("expected 1 surviving span, got %d", spanCount(req))
		}
	})

	t.Run("resource-level email drops all its spans", func(t *testing.T) {
		req := &coltracepb.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{
			resourceSpans("excluded@example.com", "", ""),
		}}
		if dropExcludedSpans(req, excluded) {
			t.Fatal("expected nothing to survive when the resource email is excluded")
		}
		if spanCount(req) != 0 {
			t.Fatalf("expected 0 surviving spans, got %d", spanCount(req))
		}
	})
}

func intAttrKV(k string, v int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{
		Value: &commonpb.AnyValue_IntValue{IntValue: v}}}
}

// Claude Code emits bare token names that Langfuse does not recognise.
func TestMapLangfuseUsageTranslatesClaudeCodeTokens(t *testing.T) {
	out := mapLangfuseUsage([]*commonpb.KeyValue{
		intAttrKV("input_tokens", 2),
		intAttrKV("output_tokens", 73),
		intAttrKV("cache_read_tokens", 18611),
		intAttrKV("cache_creation_tokens", 8820),
		strAttr("gen_ai.request.model", "claude-opus-5[1m]"),
	})

	kv := find(out, "langfuse.observation.usage_details")
	if kv == nil {
		t.Fatal("usage_details not set")
	}
	var got map[string]int64
	if err := json.Unmarshal([]byte(kv.GetValue().GetStringValue()), &got); err != nil {
		t.Fatalf("usage_details is not JSON: %v", err)
	}
	want := map[string]int64{
		"input":                       2,
		"output":                      73,
		"cache_read_input_tokens":     18611,
		"cache_creation_input_tokens": 8820,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %d, want %d", k, got[k], v)
		}
	}
	// Langfuse reads the model from the original attributes.
	if find(out, "gen_ai.request.model") == nil {
		t.Error("model attribute was dropped")
	}
}

// Cache traffic is the bulk of Claude Code's token volume.
func TestMapLangfuseUsageIncludesCacheKeys(t *testing.T) {
	out := mapLangfuseUsage([]*commonpb.KeyValue{
		intAttrKV("input_tokens", 1),
		intAttrKV("cache_read_tokens", 50000),
	})
	var got map[string]int64
	_ = json.Unmarshal([]byte(find(out, "langfuse.observation.usage_details").GetValue().GetStringValue()), &got)
	if got["cache_read_input_tokens"] != 50000 {
		t.Errorf("cache_read_input_tokens = %d, want 50000", got["cache_read_input_tokens"])
	}
}

// Non-LLM spans must not gain an empty usage object.
func TestMapLangfuseUsageLeavesNonLLMSpansAlone(t *testing.T) {
	in := []*commonpb.KeyValue{strAttr("span.type", "interaction")}
	out := mapLangfuseUsage(in)
	if find(out, "langfuse.observation.usage_details") != nil {
		t.Error("usage_details set on a span with no token attributes")
	}
	if len(out) != len(in) {
		t.Errorf("attribute count changed: %d -> %d", len(in), len(out))
	}
}

// An upstream encoding change should degrade to working, not drop usage.
func TestIntAttrAcceptsStringEncodedCounts(t *testing.T) {
	out := mapLangfuseUsage([]*commonpb.KeyValue{strAttr("output_tokens", "42")})
	var got map[string]int64
	_ = json.Unmarshal([]byte(find(out, "langfuse.observation.usage_details").GetValue().GetStringValue()), &got)
	if got["output"] != 42 {
		t.Errorf("output = %d, want 42", got["output"])
	}
}
