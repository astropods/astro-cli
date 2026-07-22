// Package ingest implements the OTLP/HTTP receiver: authenticate the bearer
// ingest key, stamp identity attributes (optionally redacting prompt content),
// and forward each signal to its store — traces to the account's Langfuse
// project, metrics to VictoriaMetrics, and logs transformed into spans and
// forwarded to Langfuse's traces endpoint (see logs.go).
package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-otel/internal/config"
	"github.com/astropods/astro/apps/astro-otel/internal/store"

	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
)

const maxBodyBytes = 16 << 20 // 16 MiB

// Attribute keys stamped by astro-otel.
const (
	attrAccountID = "astro.account_id"
	attrSource    = "astro.source"
	attrTags      = "langfuse.tags"
	sourceValue   = "claude-code"
	sessionIDKey  = "session.id"

	// Claude Code emits identity on these keys; Langfuse reads the langfuse.*
	// keys to populate a trace's userId/sessionId, so we promote across.
	attrUserEmail      = "user.email"
	attrLangfuseUserID = "langfuse.user.id"
	attrLangfuseSessID = "langfuse.session.id"
)

// Handler serves the OTLP endpoints.
type Handler struct {
	store  *store.Store
	cfg    *config.Config
	client *http.Client
	log    *slog.Logger
}

// New creates a Handler.
func New(st *store.Store, cfg *config.Config, log *slog.Logger) *Handler {
	return &Handler{
		store:  st,
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
		log:    log,
	}
}

// Register wires the OTLP and health routes onto a ServeMux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/traces", h.handleTraces)
	mux.HandleFunc("POST /v1/metrics", h.handleMetrics)
	mux.HandleFunc("POST /v1/logs", h.handleLogs)
	mux.HandleFunc("GET /livez", h.health)
	mux.HandleFunc("GET /healthz", h.health)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// authenticate resolves the bearer key to an account. Returns an HTTP status
// to write on failure (0 = success).
func (h *Handler) authenticate(r *http.Request) (accountID string, hash []byte, status int) {
	const prefix = "Bearer "
	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(authz, prefix) {
		return "", nil, http.StatusUnauthorized
	}
	token := strings.TrimSpace(authz[len(prefix):])
	if token == "" {
		return "", nil, http.StatusUnauthorized
	}
	sum := sha256.Sum256([]byte(token))
	id, found, err := h.store.ResolveAccount(r.Context(), sum[:])
	if err != nil {
		h.log.Error("resolve account failed", "error", err)
		return "", nil, http.StatusServiceUnavailable
	}
	if !found {
		return "", nil, http.StatusUnauthorized
	}
	return id, sum[:], 0
}

func (h *Handler) handleTraces(w http.ResponseWriter, r *http.Request) {
	accountID, hash, status := h.authenticate(r)
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
	var req coltracepb.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid OTLP protobuf", http.StatusBadRequest)
		return
	}

	redactOn := h.cfg.RedactAttributes
	for _, rs := range req.GetResourceSpans() {
		if rs.Resource != nil {
			attrs := rs.Resource.Attributes
			if redactOn {
				attrs = redact(attrs)
			}
			rs.Resource.Attributes = stampIdentity(attrs, accountID)
		}
		for _, ss := range rs.GetScopeSpans() {
			for _, span := range ss.GetSpans() {
				attrs := span.Attributes
				if redactOn {
					attrs = redact(attrs)
				}
				span.Attributes = mapLangfuseIdentity(tagClaudeCode(attrs))
			}
		}
	}

	basic, err := h.store.LangfuseBasicAuth(r.Context(), accountID)
	if err != nil {
		h.log.Error("langfuse creds failed", "account_id", accountID, "error", err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	if basic == "" {
		// No Langfuse project for this account — nothing to route to. Ack so the
		// exporter doesn't retry forever; provisioning happens at key creation.
		h.log.Warn("no langfuse project; dropping traces", "account_id", accountID)
		writeProto(w, &coltracepb.ExportTraceServiceResponse{})
		return
	}

	out, err := proto.Marshal(&req)
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
	writeProto(w, &coltracepb.ExportTraceServiceResponse{})
}

func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
	accountID, hash, status := h.authenticate(r)
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	h.touch(hash)

	if h.cfg.VMOTLPEndpoint == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	body, err := readBody(r)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var req colmetricspb.ExportMetricsServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid OTLP protobuf", http.StatusBadRequest)
		return
	}

	for _, rm := range req.GetResourceMetrics() {
		if rm.Resource != nil {
			// session.id would explode label cardinality; drop it defensively
			// (managed settings already set OTEL_METRICS_INCLUDE_SESSION_ID=false).
			rm.Resource.Attributes = stampIdentity(dropAttr(rm.Resource.Attributes, sessionIDKey), accountID)
		}
	}

	out, err := proto.Marshal(&req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	code, err := h.forward(r.Context(), h.cfg.VMOTLPEndpoint, out, nil)
	if err != nil || code < 200 || code >= 300 {
		h.log.Error("victoriametrics forward failed", "account_id", accountID, "status", code, "error", err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	writeProto(w, &colmetricspb.ExportMetricsServiceResponse{})
}

// touch stamps last_used_at without blocking the response.
func (h *Handler) touch(hash []byte) {
	go func() {
		if err := h.store.TouchLastUsed(hash); err != nil {
			h.log.Debug("touch last_used failed", "error", err)
		}
	}()
}

func (h *Handler) forward(ctx context.Context, url string, body []byte, headers map[string]string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, nil
}

func readBody(r *http.Request) ([]byte, error) {
	var reader io.Reader = r.Body
	if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close() //nolint:errcheck
		reader = gz
	}
	return io.ReadAll(io.LimitReader(reader, maxBodyBytes))
}

func writeProto(w http.ResponseWriter, msg proto.Message) {
	b, err := proto.Marshal(msg)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

// --- attribute helpers ---------------------------------------------------

// promptAttrPrefixes are attribute name prefixes that may carry prompt,
// completion, or tool-body content. Only stripped when redaction is enabled
// (OTEL_REDACT_ATTRIBUTES); managed settings keep this content off at the
// source by default, so redaction is opt-in defense in depth.
var promptAttrPrefixes = []string{
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

func shouldRedact(key string) bool {
	for _, p := range promptAttrPrefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// redact returns attrs with prompt/completion/tool-body entries removed.
func redact(attrs []*commonpb.KeyValue) []*commonpb.KeyValue {
	out := make([]*commonpb.KeyValue, 0, len(attrs))
	for _, kv := range attrs {
		if kv != nil && shouldRedact(kv.Key) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// stampIdentity sets astro.account_id and astro.source on a resource.
func stampIdentity(attrs []*commonpb.KeyValue, accountID string) []*commonpb.KeyValue {
	attrs = upsertString(attrs, attrAccountID, accountID)
	attrs = upsertString(attrs, attrSource, sourceValue)
	return attrs
}

// tagClaudeCode sets langfuse.tags to include "claude-code" so the shared
// Langfuse project can distinguish coding-tool spans from agent spans.
func tagClaudeCode(attrs []*commonpb.KeyValue) []*commonpb.KeyValue {
	return upsert(attrs, attrTags, &commonpb.AnyValue{
		Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{
			Values: []*commonpb.AnyValue{
				{Value: &commonpb.AnyValue_StringValue{StringValue: sourceValue}},
			},
		}},
	})
}

// mapLangfuseIdentity promotes Claude Code's identity attributes to the keys
// Langfuse reads for a trace's userId/sessionId: user.email -> langfuse.user.id
// and session.id -> langfuse.session.id. Without it Langfuse falls back to the
// opaque hashed user.id and leaves the session ungrouped.
func mapLangfuseIdentity(attrs []*commonpb.KeyValue) []*commonpb.KeyValue {
	if email := stringAttr(attrs, attrUserEmail); email != "" {
		attrs = upsertString(attrs, attrLangfuseUserID, email)
	}
	if sid := stringAttr(attrs, sessionIDKey); sid != "" {
		attrs = upsertString(attrs, attrLangfuseSessID, sid)
	}
	return attrs
}

// stringAttr returns the string value for key, or "" if absent or non-string.
func stringAttr(attrs []*commonpb.KeyValue, key string) string {
	for _, kv := range attrs {
		if kv != nil && kv.Key == key {
			return kv.GetValue().GetStringValue()
		}
	}
	return ""
}

func upsertString(attrs []*commonpb.KeyValue, key, val string) []*commonpb.KeyValue {
	return upsert(attrs, key, &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: val}})
}

func upsert(attrs []*commonpb.KeyValue, key string, val *commonpb.AnyValue) []*commonpb.KeyValue {
	for _, kv := range attrs {
		if kv != nil && kv.Key == key {
			kv.Value = val
			return attrs
		}
	}
	return append(attrs, &commonpb.KeyValue{Key: key, Value: val})
}

func dropAttr(attrs []*commonpb.KeyValue, key string) []*commonpb.KeyValue {
	out := make([]*commonpb.KeyValue, 0, len(attrs))
	for _, kv := range attrs {
		if kv != nil && kv.Key == key {
			continue
		}
		out = append(out, kv)
	}
	return out
}
