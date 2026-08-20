package metronome

import (
	"context"
	"net/http"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

const bothMetrics = `{"data":[` +
	`{"id":"bm_compute","name":"Compute Units","event_type_filter":{"in_values":["deployment_compute_usage"]}},` +
	`{"id":"bm_gateway","name":"LLM Usage","event_type_filter":{"in_values":["ai_gateway_llm_usage"]}}` +
	`],"next_page":null}`

const computeOnly = `{"data":[` +
	`{"id":"bm_compute","name":"Compute Units","event_type_filter":{"in_values":["deployment_compute_usage"]}}` +
	`],"next_page":null}`

// A metric absent from the first successful list must not be absent for the
// life of the process. Caching a partial answer is the same failure as caching
// an error: every later cap write for that metric fails until a restart.
func TestMetricID_DoesNotCacheAPartialList(t *testing.T) {
	body := computeOnly
	calls := 0
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	if _, err := p.metricID(context.Background(), billing.UsageMetricGateway); err == nil {
		t.Fatal("want an error while the gateway metric is missing")
	}
	body = bothMetrics
	id, err := p.metricID(context.Background(), billing.UsageMetricGateway)
	if err != nil {
		t.Fatalf("metricID after the metric appeared: %v", err)
	}
	if id != "bm_gateway" {
		t.Errorf("id = %q, want bm_gateway", id)
	}
	if calls != 2 {
		t.Errorf("listed %d times, want 2: the partial answer was cached", calls)
	}
}

// A complete answer is cached, or every cap write pays a list.
func TestMetricID_CachesACompleteList(t *testing.T) {
	calls := 0
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(bothMetrics))
	})
	for _, m := range billing.AllUsageMetrics {
		if _, err := p.metricID(context.Background(), m); err != nil {
			t.Fatalf("metricID(%s): %v", m, err)
		}
	}
	if calls != 1 {
		t.Errorf("listed %d times, want 1", calls)
	}
}
