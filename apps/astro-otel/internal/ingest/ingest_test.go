package ingest

import (
	"testing"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
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
