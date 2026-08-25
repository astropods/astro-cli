package bifrostotel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// ingestBatchLimit is Metronome's documented max events per /v1/ingest request.
const ingestBatchLimit = 100

// Attribute keys Bifrost writes on GenAI spans (canonical bifrost.* preferred;
// gen_ai.* kept as legacy fallbacks).
const (
	attrCustomerID    = "bifrost.customer.id"
	attrCustomerIDAlt = "gen_ai.customer_id"
	attrRequestID     = "bifrost.request.id"
	attrRequestIDAlt  = "gen_ai.request_id"
	attrVirtualKeyID  = "bifrost.virtual_key.id"
	attrProvider      = "bifrost.provider.name"
	attrRetries       = "bifrost.retries"

	attrReqModel  = "gen_ai.request.model"
	attrRespModel = "gen_ai.response.model"

	// OTel semantic-convention attribute names, not secrets. gosec matches on
	// "token" in the identifier.
	attrInputTokens     = "gen_ai.usage.input_tokens"            //nolint:gosec
	attrOutputTokens    = "gen_ai.usage.output_tokens"           //nolint:gosec
	attrTotalTokens     = "gen_ai.usage.total_tokens"            //nolint:gosec
	attrCacheReadTokens = "gen_ai.usage.cache_read.input_tokens" //nolint:gosec
	attrReasoningTokens = "gen_ai.usage.reasoning.output_tokens" //nolint:gosec
	attrCost            = "gen_ai.usage.cost"
)

// event is one Metronome usage event.
type event struct {
	TransactionID string         `json:"transaction_id"`
	CustomerID    string         `json:"customer_id"`
	EventType     string         `json:"event_type"`
	Timestamp     string         `json:"timestamp"`
	Properties    map[string]any `json:"properties"`
}

type bifrostExporter struct {
	cfg    *Config
	log    *zap.Logger
	client *http.Client
	url    string
}

func newExporter(cfg *Config, set exporter.Settings) *bifrostExporter {
	return &bifrostExporter{
		cfg: cfg,
		log: set.Logger,
		url: cfg.Endpoint + "/v1/ingest",
	}
}

func (e *bifrostExporter) start(_ context.Context, host component.Host) error {
	e.client = &http.Client{}
	return nil
}

// pushTraces maps the batch to Metronome events and posts them. A returned
// error is retried by the exporterhelper queue; a permanent error is dropped.
func (e *bifrostExporter) pushTraces(ctx context.Context, traces ptrace.Traces) error {
	events := e.eventsFromTraces(traces)
	if len(events) == 0 {
		return nil
	}
	for start := 0; start < len(events); start += ingestBatchLimit {
		end := min(start+ingestBatchLimit, len(events))
		if err := e.ingest(ctx, events[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// maxAncestorHops caps the walk to the root. The chain nests one level per
// plugin hook, so seven plugins put the root 16 hops up.
const maxAncestorHops = 128

// requestKey groups the attempts of one request. An empty id means none was
// resolved and the trace is the only scope left.
type requestKey struct {
	id    string
	trace pcommon.TraceID
}

type spanEntry struct {
	attrs         pcommon.Map
	resourceAttrs pcommon.Map
	parent        pcommon.SpanID
}

// forEachSpan visits every span in the payload with the resource attributes it
// inherits.
func forEachSpan(traces ptrace.Traces, visit func(span ptrace.Span, resourceAttrs pcommon.Map)) {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)
		resourceAttrs := resourceSpan.Resource().Attributes()
		scopeSpans := resourceSpan.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			spans := scopeSpans.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				visit(spans.At(k), resourceAttrs)
			}
		}
	}
}

// eventsFromTraces maps the final successful attempt of each request to a
// Metronome event. Grouping by trace instead would bill a whole session once:
// an x-bf-session-id or an inbound traceparent puts every request of that
// session under one trace id.
func (e *bifrostExporter) eventsFromTraces(traces ptrace.Traces) []event {
	type candidate struct {
		span          ptrace.Span
		resourceAttrs pcommon.Map
		attempt       int64
		succeeded     bool
	}
	index := indexSpans(traces)
	best := map[requestKey]candidate{}

	forEachSpan(traces, func(span ptrace.Span, resourceAttrs pcommon.Map) {
		if !isBillable(span) {
			return
		}
		attempt := getInt(span.Attributes(), resourceAttrs, attrRetries)
		succeeded := span.Status().Code() != ptrace.StatusCodeError
		key := requestKey{id: resolveRequestID(span, resourceAttrs, index), trace: span.TraceID()}
		current, exists := best[key]
		if !exists || attempt > current.attempt || (attempt == current.attempt && succeeded && !current.succeeded) {
			best[key] = candidate{span: span, resourceAttrs: resourceAttrs, attempt: attempt, succeeded: succeeded}
		}
	})

	events := make([]event, 0, len(best))
	var unresolved int
	for key, chosen := range best {
		attrs := chosen.span.Attributes()
		resourceAttrs := chosen.resourceAttrs
		customerID := firstStr(attrs, resourceAttrs, attrCustomerID, attrCustomerIDAlt)
		if customerID == "" {
			e.log.Warn("bifrostotel: span missing customer id, skipping", zap.String("trace_id", chosen.span.TraceID().String()))
			continue
		}
		txnID := key.id
		if txnID == "" {
			txnID = key.trace.String()
			unresolved++
		}

		props := map[string]any{
			"model":              firstStr(attrs, resourceAttrs, attrRespModel, attrReqModel),
			"provider":           firstStr(attrs, resourceAttrs, attrProvider),
			"input_tokens":       getInt(attrs, resourceAttrs, attrInputTokens),
			"output_tokens":      getInt(attrs, resourceAttrs, attrOutputTokens),
			"total_tokens":       getInt(attrs, resourceAttrs, attrTotalTokens),
			"cached_read_tokens": getInt(attrs, resourceAttrs, attrCacheReadTokens),
			"reasoning_tokens":   getInt(attrs, resourceAttrs, attrReasoningTokens),
			"virtual_key_id":     firstStr(attrs, resourceAttrs, attrVirtualKeyID),
		}
		if cost, ok := getFloat(attrs, resourceAttrs, attrCost); ok {
			props["cost_usd"] = cost
		}

		endedAt := chosen.span.EndTimestamp().AsTime()
		if endedAt.IsZero() {
			endedAt = time.Now()
		}
		events = append(events, event{
			TransactionID: txnID,
			CustomerID:    customerID,
			EventType:     e.cfg.EventType,
			Timestamp:     endedAt.UTC().Format(time.RFC3339),
			Properties:    props,
		})
	}
	if unresolved > 0 {
		e.log.Warn("bifrostotel: billed by trace, no request id resolved",
			zap.Int("events", unresolved), zap.Int("total", len(events)))
	}
	return events
}

// indexSpans keys every span by its own id so resolveRequestID can climb.
func indexSpans(traces ptrace.Traces) map[pcommon.SpanID]spanEntry {
	index := map[pcommon.SpanID]spanEntry{}
	forEachSpan(traces, func(span ptrace.Span, resourceAttrs pcommon.Map) {
		index[span.SpanID()] = spanEntry{attrs: span.Attributes(), resourceAttrs: resourceAttrs, parent: span.ParentSpanID()}
	})
	return index
}

// resolveRequestID finds the request id. Bifrost writes it on the request's
// root span, not on the billable llm.call descendant, so it is usually
// inherited from an ancestor.
func resolveRequestID(span ptrace.Span, resourceAttrs pcommon.Map, index map[pcommon.SpanID]spanEntry) string {
	if id := firstStr(span.Attributes(), resourceAttrs, attrRequestID, attrRequestIDAlt); id != "" {
		return id
	}
	parent := span.ParentSpanID()
	for hops := 0; hops < maxAncestorHops && !parent.IsEmpty(); hops++ {
		entry, ok := index[parent]
		if !ok {
			return ""
		}
		if id := firstStr(entry.attrs, entry.resourceAttrs, attrRequestID, attrRequestIDAlt); id != "" {
			return id
		}
		parent = entry.parent
	}
	return ""
}

// ingest posts one batch. 429/5xx are retryable; other 4xx are permanent.
func (e *bifrostExporter) ingest(ctx context.Context, batch []event) error {
	body, err := json.Marshal(batch)
	if err != nil {
		return consumererror.NewPermanent(fmt.Errorf("bifrostotel: marshal: %w", err))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(body))
	if err != nil {
		return consumererror.NewPermanent(fmt.Errorf("bifrostotel: new request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("bifrostotel: post: %w", err) // network error → retry
	}
	defer resp.Body.Close() //nolint:errcheck

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return fmt.Errorf("bifrostotel: metronome status %d", resp.StatusCode) // retry
	default:
		return consumererror.NewPermanent(fmt.Errorf("bifrostotel: metronome status %d", resp.StatusCode))
	}
}

// isBillable reports whether a span is an LLM-call span carrying usage (excludes
// root/http, plugin, and MCP spans, which have no token usage).
func isBillable(span ptrace.Span) bool {
	if _, ok := span.Attributes().Get(attrInputTokens); ok {
		return true
	}
	if _, ok := span.Attributes().Get(attrTotalTokens); ok {
		return true
	}
	return false
}

func firstStr(attrs, res pcommon.Map, keys ...string) string {
	for _, k := range keys {
		if v, ok := attrs.Get(k); ok {
			return v.AsString()
		}
		if v, ok := res.Get(k); ok {
			return v.AsString()
		}
	}
	return ""
}

func getInt(attrs, res pcommon.Map, key string) int64 {
	if v, ok := attrs.Get(key); ok {
		return v.Int()
	}
	if v, ok := res.Get(key); ok {
		return v.Int()
	}
	return 0
}

func getFloat(attrs, res pcommon.Map, key string) (float64, bool) {
	if v, ok := attrs.Get(key); ok {
		return v.Double(), true
	}
	if v, ok := res.Get(key); ok {
		return v.Double(), true
	}
	return 0, false
}
