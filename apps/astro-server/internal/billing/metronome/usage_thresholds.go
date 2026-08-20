package metronome

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/Metronome-Industries/metronome-go/v3"
	"github.com/Metronome-Industries/metronome-go/v3/packages/param"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

func metricEventType(metric billing.UsageMetric) string {
	switch metric {
	case billing.UsageMetricGateway:
		return "ai_gateway_llm_usage"
	default:
		return "deployment_compute_usage"
	}
}

type billableMetricIDs struct {
	mu  sync.Mutex
	ids map[billing.UsageMetric]string
}

// metricID resolves one metric's provider id, listing on the first call. Only
// success is cached, and the lock is never held across the list: caching a
// transient failure would refuse every later cap write until a restart, and
// holding the lock would block unrelated cap operations behind one slow request.
func (p *Provider) metricID(ctx context.Context, metric billing.UsageMetric) (string, error) {
	p.metrics.mu.Lock()
	cached := p.metrics.ids
	p.metrics.mu.Unlock()

	if cached == nil {
		ids := make(map[billing.UsageMetric]string, len(billing.AllUsageMetrics))
		iter := p.mc.V1.BillableMetrics.ListAutoPaging(ctx, metronome.V1BillableMetricListParams{})
		for iter.Next() {
			bm := iter.Current()
			for _, m := range billing.AllUsageMetrics {
				if slices.Contains(bm.EventTypeFilter.InValues, metricEventType(m)) {
					ids[m] = bm.ID
				}
			}
		}
		if err := iter.Err(); err != nil {
			return "", fmt.Errorf("metronome list billable metrics: %w", err)
		}
		cached = ids
		// A partial answer is not cached. A metric that did not exist yet, or was
		// recreated, would otherwise be missing for the life of the process, which
		// is the failure caching a bad result was meant to avoid.
		if len(ids) == len(billing.AllUsageMetrics) {
			p.metrics.mu.Lock()
			p.metrics.ids = ids
			p.metrics.mu.Unlock()
		}
	}
	id, ok := cached[metric]
	if !ok {
		return "", fmt.Errorf("metronome: no billable metric aggregates %q", metricEventType(metric))
	}
	return id, nil
}

// CustomerMetricUsage totals one metric over the customer's current billing
// period, the window a threshold is evaluated in. No draft invoice means no open
// period and so no usage, which reads as zero rather than as an error.
func (p *Provider) CustomerMetricUsage(ctx context.Context, customerID string, metric billing.UsageMetric) (float64, error) {
	metricID, err := p.metricID(ctx, metric)
	if err != nil {
		return 0, err
	}
	draft, err := p.latestInvoice(ctx, customerID, "DRAFT")
	if err != nil || draft == nil {
		return 0, err
	}
	var total float64
	iter := p.mc.V1.Usage.ListAutoPaging(ctx, metronome.V1UsageListParams{
		StartingOn:      draft.StartTimestamp,
		EndingBefore:    draft.EndTimestamp,
		WindowSize:      metronome.V1UsageListParamsWindowSizeNone,
		CustomerIDs:     []string{customerID},
		BillableMetrics: []metronome.V1UsageListParamsBillableMetric{{ID: metricID}},
	})
	for iter.Next() {
		total += iter.Current().Value
	}
	if err := iter.Err(); err != nil {
		return 0, fmt.Errorf("metronome list usage for %s: %w", metric, err)
	}
	return total, nil
}

func (p *Provider) CustomerUsageThresholds(ctx context.Context, customerID string) (map[billing.UsageMetric]billing.UsageThresholds, error) {
	iter := p.mc.V1.Customers.Alerts.ListAutoPaging(ctx, metronome.V1CustomerAlertListParams{
		CustomerID: customerID,
	})
	out := make(map[billing.UsageMetric]billing.UsageThresholds)
	for iter.Next() {
		ca := iter.Current()
		metric, kind, ok := billing.UsageMetricForAlert(ca.Alert.Name)
		if !ok {
			continue
		}
		t := billing.UsageThreshold{
			Amount:  ca.Alert.Threshold,
			InAlarm: string(ca.CustomerStatus) == "in_alarm",
		}
		entry := out[metric]
		if kind == billing.SpendThresholdLimit {
			entry.Limit, entry.HasLimit = t, true
		} else {
			entry.Warning, entry.HasWarning = t, true
		}
		out[metric] = entry
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("metronome list customer alerts: %w", err)
	}
	return out, nil
}

func (p *Provider) existingUsageAlert(ctx context.Context, customerID string, metric billing.UsageMetric, kind billing.SpendThresholdKind) (id string, threshold float64, err error) {
	iter := p.mc.V1.Customers.Alerts.ListAutoPaging(ctx, metronome.V1CustomerAlertListParams{
		CustomerID: customerID,
	})
	want := billing.UsageAlertName(metric, kind)
	for iter.Next() {
		if ca := iter.Current(); ca.Alert.Name == want {
			return ca.Alert.ID, ca.Alert.Threshold, nil
		}
	}
	if err := iter.Err(); err != nil {
		return "", 0, fmt.Errorf("metronome list customer alerts: %w", err)
	}
	return "", 0, nil
}

func (p *Provider) SetCustomerUsageThreshold(ctx context.Context, customerID string, metric billing.UsageMetric, kind billing.SpendThresholdKind, amount float64) error {
	metricID, err := p.metricID(ctx, metric)
	if err != nil {
		return err
	}
	existingID, existing, err := p.existingUsageAlert(ctx, customerID, metric, kind)
	if err != nil {
		return err
	}
	if existingID != "" {
		if existing == amount {
			return nil
		}
		if err := p.archiveAlert(ctx, existingID); err != nil {
			return err
		}
	}

	name := billing.UsageAlertName(metric, kind)
	created, err := p.mc.V1.Alerts.New(ctx, metronome.V1AlertNewParams{
		AlertType:        metronome.V1AlertNewParamsAlertTypeUsageThresholdReached,
		Name:             name,
		Threshold:        amount,
		CustomerID:       param.NewOpt(customerID),
		BillableMetricID: param.NewOpt(metricID),
		UniquenessKey:    param.NewOpt(name + ":" + customerID),
	})
	if err != nil {
		return fmt.Errorf("metronome create usage %s for %s: %w", kind, metric, err)
	}
	if err := p.mc.V1.Customers.Alerts.Reset(ctx, metronome.V1CustomerAlertResetParams{
		AlertID:    created.Data.ID,
		CustomerID: customerID,
	}); err != nil {
		return fmt.Errorf("metronome reset usage %s for %s: %w", kind, metric, err)
	}
	return nil
}

func (p *Provider) ClearCustomerUsageThreshold(ctx context.Context, customerID string, metric billing.UsageMetric, kind billing.SpendThresholdKind) error {
	existingID, _, err := p.existingUsageAlert(ctx, customerID, metric, kind)
	if err != nil || existingID == "" {
		return err
	}
	return p.archiveAlert(ctx, existingID)
}
