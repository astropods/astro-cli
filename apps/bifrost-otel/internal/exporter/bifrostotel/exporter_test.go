package bifrostotel

import (
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// addAttempt appends an llm.call attempt that carries the request id itself.
func addAttempt(scope ptrace.ScopeSpans, traceID pcommon.TraceID, attempt int64, in, out int64, ok bool) {
	sp := scope.Spans().AppendEmpty()
	sp.SetName("llm.call")
	sp.SetTraceID(traceID)
	sp.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Unix(1_700_000_000, 0)))
	if ok {
		sp.Status().SetCode(ptrace.StatusCodeOk)
	} else {
		sp.Status().SetCode(ptrace.StatusCodeError)
	}
	a := sp.Attributes()
	a.PutInt(attrRetries, attempt)
	a.PutStr(attrCustomerID, "acct_123")
	a.PutStr(attrRequestID, "req_abc")
	a.PutStr(attrRespModel, "claude-sonnet-5")
	a.PutStr(attrProvider, "bedrock")
	a.PutInt(attrInputTokens, in)
	a.PutInt(attrOutputTokens, out)
	a.PutInt(attrTotalTokens, in+out)
}

// A request that failed once then succeeded on retry emits exactly ONE event,
// from the final successful attempt — never the sum of both attempts.
func TestFinalAttemptBilledOnce(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	scope := rs.ScopeSpans().AppendEmpty()
	traceID := pcommon.TraceID([16]byte{1, 2, 3, 4})

	addAttempt(scope, traceID, 0, 10, 5, false)  // failed first attempt
	addAttempt(scope, traceID, 1, 100, 50, true) // successful retry

	// Add a non-billable root span (no usage) — must be ignored.
	root := scope.Spans().AppendEmpty()
	root.SetName("http.request")
	root.SetTraceID(traceID)

	e := &bifrostExporter{cfg: &Config{EventType: "ai_gateway_llm_usage"}, log: zap.NewNop()}
	events := e.eventsFromTraces(td)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.CustomerID != "acct_123" {
		t.Errorf("customer_id = %q, want acct_123", ev.CustomerID)
	}
	if ev.TransactionID != "req_abc" {
		t.Errorf("transaction_id = %q, want req_abc", ev.TransactionID)
	}
	if got := ev.Properties["input_tokens"].(int64); got != 100 {
		t.Errorf("input_tokens = %d, want 100 (final attempt, not summed)", got)
	}
	if got := ev.Properties["output_tokens"].(int64); got != 50 {
		t.Errorf("output_tokens = %d, want 50", got)
	}
}

// Two distinct requests in one batch produce two events.
func TestDistinctTracesBilledSeparately(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	scope := rs.ScopeSpans().AppendEmpty()
	addAttempt(scope, pcommon.TraceID([16]byte{1}), 0, 10, 5, true)
	addAttempt(scope, pcommon.TraceID([16]byte{2}), 0, 20, 8, true)

	e := &bifrostExporter{cfg: &Config{EventType: "ai_gateway_llm_usage"}, log: zap.NewNop()}
	if got := len(e.eventsFromTraces(td)); got != 2 {
		t.Fatalf("expected 2 events, got %d", got)
	}
}

// addRequest appends the shape Bifrost emits: the root holds the request id,
// the llm.call child holds the usage and no request id of its own.
func addRequest(scope ptrace.ScopeSpans, traceID pcommon.TraceID, root, child pcommon.SpanID, reqID string, attempt, in, out int64, ok bool) {
	r := scope.Spans().AppendEmpty()
	r.SetName("http.request")
	r.SetTraceID(traceID)
	r.SetSpanID(root)
	r.Attributes().PutStr(attrRequestID, reqID)

	c := scope.Spans().AppendEmpty()
	c.SetName("llm.call")
	c.SetTraceID(traceID)
	c.SetSpanID(child)
	c.SetParentSpanID(root)
	c.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Unix(1_700_000_000, 0)))
	if ok {
		c.Status().SetCode(ptrace.StatusCodeOk)
	} else {
		c.Status().SetCode(ptrace.StatusCodeError)
	}
	a := c.Attributes()
	a.PutInt(attrRetries, attempt)
	a.PutStr(attrCustomerID, "acct_123")
	a.PutStr(attrRespModel, "claude-sonnet-5")
	a.PutInt(attrInputTokens, in)
	a.PutInt(attrOutputTokens, out)
	a.PutInt(attrTotalTokens, in+out)
}

// Two requests sharing a trace id must still bill twice. Charging such a
// session once is the whole undercount.
func TestSharedTraceBillsEveryRequest(t *testing.T) {
	td := ptrace.NewTraces()
	scope := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	traceID := pcommon.TraceID([16]byte{7})

	addRequest(scope, traceID, pcommon.SpanID([8]byte{1}), pcommon.SpanID([8]byte{2}), "req_1", 0, 10, 5, true)
	addRequest(scope, traceID, pcommon.SpanID([8]byte{3}), pcommon.SpanID([8]byte{4}), "req_2", 0, 20, 8, true)

	e := &bifrostExporter{cfg: &Config{EventType: "ai_gateway_llm_usage"}, log: zap.NewNop()}
	events := e.eventsFromTraces(td)

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	got := map[string]int64{}
	for _, ev := range events {
		got[ev.TransactionID] = ev.Properties["input_tokens"].(int64)
	}
	if got["req_1"] != 10 || got["req_2"] != 20 {
		t.Errorf("events keyed by request id = %v, want req_1:10 req_2:20", got)
	}
}

// Retries share a request id, so they still collapse when only the root names it.
func TestRetriesUnderOneRequestBilledOnce(t *testing.T) {
	td := ptrace.NewTraces()
	scope := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	traceID := pcommon.TraceID([16]byte{8})
	root := pcommon.SpanID([8]byte{1})

	addRequest(scope, traceID, root, pcommon.SpanID([8]byte{2}), "req_1", 0, 10, 5, false)
	addRequest(scope, traceID, root, pcommon.SpanID([8]byte{3}), "req_1", 1, 100, 50, true)

	e := &bifrostExporter{cfg: &Config{EventType: "ai_gateway_llm_usage"}, log: zap.NewNop()}
	events := e.eventsFromTraces(td)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].TransactionID != "req_1" {
		t.Errorf("transaction_id = %q, want req_1", events[0].TransactionID)
	}
	if got := events[0].Properties["input_tokens"].(int64); got != 100 {
		t.Errorf("input_tokens = %d, want 100 (final attempt)", got)
	}
}

// With no request id the trace is the only scope left. Billing per span instead
// would charge every retry.
func TestNoRequestIDFallsBackToTrace(t *testing.T) {
	td := ptrace.NewTraces()
	scope := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	traceID := pcommon.TraceID([16]byte{9, 9})
	for _, in := range []int64{10, 20} {
		sp := scope.Spans().AppendEmpty()
		sp.SetName("llm.call")
		sp.SetTraceID(traceID)
		sp.Attributes().PutStr(attrCustomerID, "acct_123")
		sp.Attributes().PutInt(attrInputTokens, in)
	}

	e := &bifrostExporter{cfg: &Config{EventType: "ai_gateway_llm_usage"}, log: zap.NewNop()}
	events := e.eventsFromTraces(td)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].TransactionID != traceID.String() {
		t.Errorf("transaction_id = %q, want the trace id %q", events[0].TransactionID, traceID.String())
	}
}

// A span without a resolved customer id is skipped (nothing to attribute).
func TestMissingCustomerSkipped(t *testing.T) {
	td := ptrace.NewTraces()
	scope := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	sp := scope.Spans().AppendEmpty()
	sp.SetName("llm.call")
	sp.SetTraceID(pcommon.TraceID([16]byte{9}))
	sp.Attributes().PutInt(attrInputTokens, 10)
	sp.Attributes().PutInt(attrOutputTokens, 5)

	e := &bifrostExporter{cfg: &Config{EventType: "ai_gateway_llm_usage"}, log: zap.NewNop()}
	if got := len(e.eventsFromTraces(td)); got != 0 {
		t.Fatalf("expected 0 events, got %d", got)
	}
}
