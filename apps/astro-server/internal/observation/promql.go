package observation

import (
	"context"

	"github.com/astropods/astro/apps/astro-server/internal/promquery"
)

// promInstant is the slice of *promquery.Client the PromQL engine needs.
type promInstant interface {
	Query(ctx context.Context, promql string) ([]promquery.Sample, error)
}

// PromQLEngine adapts a Prometheus-compatible instant-query client (VictoriaMetrics)
// to the engine-neutral Querier. It is the engine for EnginePromQL conditions.
type PromQLEngine struct{ client promInstant }

// NewPromQLEngine wraps an instant-query client as a Querier. Returns nil if the
// client is nil so callers can leave the engine unregistered when Prometheus is
// unconfigured.
func NewPromQLEngine(client promInstant) *PromQLEngine {
	if client == nil {
		return nil
	}
	return &PromQLEngine{client: client}
}

func (e *PromQLEngine) Query(ctx context.Context, query string) ([]Series, error) {
	samples, err := e.client.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]Series, 0, len(samples))
	for _, s := range samples {
		out = append(out, Series{Labels: s.Labels, Value: s.Value})
	}
	return out, nil
}
