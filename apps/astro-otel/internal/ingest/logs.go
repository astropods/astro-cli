// Logs leg of the receiver. Claude Code emits the assistant response (and the
// prompt/tool I/O) only on the OTLP logs signal, but Langfuse ingests only
// traces. This handler transforms the content-bearing log records into spans
// and forwards them to Langfuse's /v1/traces, so the response lands as an
// observation in the same trace the prompt already belongs to.
//
// The transform is stateless: every Claude Code log record carries the parent
// interaction's traceId + spanId inline, so each synthesized span drops into
// the correct trace without joining across records.

package ingest

import (
	"crypto/rand"
	"net/http"
	"strings"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// Langfuse-native span attributes the ingestion processor reads first (see
// packages/shared/src/server/otel/attributes.ts). Setting these maps content
// to an observation's input/output/type without mimicking any framework.
const (
	attrObsType     = "langfuse.observation.type"
	attrObsInput    = "langfuse.observation.input"
	attrObsOutput   = "langfuse.observation.output"
	attrObsModel    = "langfuse.observation.model.name"
	attrTraceInput  = "langfuse.trace.input"
	attrTraceOutput = "langfuse.trace.output"

	obsTypeGeneration = "generation"
	obsTypeSpan       = "span"

	// Content-bearing keys on Claude Code log records.
	claudeEventPrefix = "claude_code."
	recAttrEventName  = "event.name"
	recAttrPrompt     = "prompt"
	recAttrResponse   = "response"
	recAttrModel      = "model"
	recAttrToolInput  = "tool_input"

	scopeName = "astro-otel/devtool-transform"
)

func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	accountID, hash, excluded, status := h.authenticate(r)
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	h.touch(hash)

	body, err := readBody(r)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var req collogspb.ExportLogsServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid OTLP protobuf", http.StatusBadRequest)
		return
	}

	traces := transformLogsToTraces(&req, accountID, h.cfg.RedactAttributes, excluded)
	if traces == nil {
		// Nothing content-bearing (excluded user, content redacted at the
		// source, or off). Ack so the exporter doesn't retry.
		writeProto(w, &collogspb.ExportLogsServiceResponse{})
		return
	}

	basic, err := h.store.LangfuseBasicAuth(r.Context(), accountID)
	if err != nil {
		h.log.Error("langfuse creds failed", "account_id", accountID, "error", err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	if basic == "" {
		h.log.Warn("no langfuse project; dropping logs", "account_id", accountID)
		writeProto(w, &collogspb.ExportLogsServiceResponse{})
		return
	}

	out, err := proto.Marshal(traces)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	url := strings.TrimRight(h.cfg.LangfuseOTLPEndpoint, "/") + "/v1/traces"
	code, err := h.forward(r.Context(), url, out, map[string]string{"Authorization": "Basic " + basic})
	if err != nil || code < 200 || code >= 300 {
		h.log.Error("langfuse forward failed", "account_id", accountID, "status", code, "error", err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	writeProto(w, &collogspb.ExportLogsServiceResponse{})
}

// transformLogsToTraces reshapes an OTLP logs export into a traces export
// carrying one synthesized span per content-bearing Claude Code record.
// Returns nil when no record yields content (so the caller can ack and skip
// the forward).
func transformLogsToTraces(in *collogspb.ExportLogsServiceRequest, accountID string, redactOn bool, excluded map[string]struct{}) *coltracepb.ExportTraceServiceRequest {
	var spans []*tracepb.Span
	var resourceAttrs []*commonpb.KeyValue
	for _, rl := range in.GetResourceLogs() {
		if resourceAttrs == nil && rl.GetResource() != nil {
			resourceAttrs = rl.GetResource().GetAttributes()
		}
		// Log records are pure content (prompt/response/tool bodies), so for an
		// excluded user there is nothing to keep — drop them outright. Email may
		// sit on the record or the enclosing resource.
		resExcluded := rl.GetResource() != nil && emailExcluded(rl.GetResource().GetAttributes(), excluded)
		for _, sl := range rl.GetScopeLogs() {
			for _, rec := range sl.GetLogRecords() {
				if resExcluded || emailExcluded(rec.GetAttributes(), excluded) {
					continue
				}
				if span := synthesizeSpan(rec, redactOn); span != nil {
					spans = append(spans, span)
				}
			}
		}
	}
	if len(spans) == 0 {
		return nil
	}
	res := &resourcepb.Resource{Attributes: stampIdentity(cloneAttrs(resourceAttrs), accountID)}
	return &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: res,
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{Name: scopeName},
				Spans: spans,
			}},
		}},
	}
}

// synthesizeSpan converts a single Claude Code log record into a Langfuse-ready
// span, or nil if the record carries no collectable content. The span reuses
// the record's traceId and parents to the record's spanId (the interaction
// span), so it nests under the prompt's trace.
func synthesizeSpan(rec *logspb.LogRecord, redactOn bool) *tracepb.Span {
	if rec == nil {
		return nil
	}
	var attrs []*commonpb.KeyValue
	switch eventName(rec) {
	case "user_prompt":
		prompt := stringAttr(rec.GetAttributes(), recAttrPrompt)
		if prompt == "" {
			return nil
		}
		attrs = upsertString(attrs, attrObsType, obsTypeSpan)
		attrs = upsertString(attrs, attrObsInput, prompt)
		attrs = upsertString(attrs, attrTraceInput, prompt)
	case "assistant_response":
		resp := stringAttr(rec.GetAttributes(), recAttrResponse)
		if resp == "" {
			return nil
		}
		attrs = upsertString(attrs, attrObsType, obsTypeGeneration)
		attrs = upsertString(attrs, attrObsOutput, resp)
		attrs = upsertString(attrs, attrTraceOutput, resp)
		if model := stringAttr(rec.GetAttributes(), recAttrModel); model != "" {
			attrs = upsertString(attrs, attrObsModel, model)
		}
	case "tool_result":
		toolInput := stringAttr(rec.GetAttributes(), recAttrToolInput)
		if toolInput == "" {
			return nil
		}
		attrs = upsertString(attrs, attrObsType, obsTypeSpan)
		attrs = upsertString(attrs, attrObsInput, toolInput)
	default:
		return nil
	}

	// Carry identity from the record so mapLangfuseIdentity can promote it.
	if email := stringAttr(rec.GetAttributes(), attrUserEmail); email != "" {
		attrs = upsertString(attrs, attrUserEmail, email)
	}
	if sid := stringAttr(rec.GetAttributes(), sessionIDKey); sid != "" {
		attrs = upsertString(attrs, sessionIDKey, sid)
	}
	if redactOn {
		attrs = redact(attrs)
	}
	attrs = mapLangfuseIdentity(tagClaudeCode(attrs))

	spanID := randID(8)
	if spanID == nil {
		return nil
	}
	traceID := rec.GetTraceId()
	if len(traceID) == 0 {
		if traceID = randID(16); traceID == nil {
			return nil
		}
	}
	ts := rec.GetTimeUnixNano()
	if ts == 0 {
		ts = rec.GetObservedTimeUnixNano()
	}
	return &tracepb.Span{
		TraceId:           traceID,
		SpanId:            spanID,
		ParentSpanId:      rec.GetSpanId(), // interaction span; empty => root
		Name:              eventName(rec),
		Kind:              tracepb.Span_SPAN_KIND_INTERNAL,
		StartTimeUnixNano: ts,
		EndTimeUnixNano:   ts,
		Attributes:        attrs,
	}
}

// eventName resolves the Claude Code event, e.g. "assistant_response". The
// event is the log body ("claude_code.assistant_response"); the event.name
// attribute is a fallback.
func eventName(rec *logspb.LogRecord) string {
	if b := rec.GetBody().GetStringValue(); b != "" {
		return strings.TrimPrefix(b, claudeEventPrefix)
	}
	return stringAttr(rec.GetAttributes(), recAttrEventName)
}

func cloneAttrs(attrs []*commonpb.KeyValue) []*commonpb.KeyValue {
	if attrs == nil {
		return nil
	}
	out := make([]*commonpb.KeyValue, len(attrs))
	copy(out, attrs)
	return out
}

// randID returns n cryptographically random bytes, or nil on failure.
func randID(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil
	}
	return b
}
