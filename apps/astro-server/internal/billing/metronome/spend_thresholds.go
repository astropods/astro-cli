package metronome

import (
	"context"
	"fmt"

	"github.com/Metronome-Industries/metronome-go/v3"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// Alert names for the two thresholds a customer sets. Metronome is the only
// store for them, so the name is what tells one apart from the other and both
// apart from the org-wide backstop, which is named by hand. The SDK's alert type
// carries no customer_id, so a prefix is the only discriminator available.
const (
	alertNameSpendWarning = "astro:spend_warning"
	alertNameSpendLimit   = "astro:spend_limit"
)

// CustomerSpendThresholds returns the customer's own warning and limit. An
// absent threshold reports HasWarning or HasLimit false rather than zero, which
// is a threshold a customer could legitimately set.
func (p *Provider) CustomerSpendThresholds(ctx context.Context, customerID string) (billing.SpendThresholds, error) {
	var out billing.SpendThresholds
	page, err := p.mc.V1.Customers.Alerts.List(ctx, metronome.V1CustomerAlertListParams{
		CustomerID: customerID,
	})
	if err != nil {
		return out, fmt.Errorf("metronome list customer alerts: %w", err)
	}
	for _, ca := range page.Data {
		t := billing.SpendThreshold{
			Amount:  ca.Alert.Threshold,
			InAlarm: string(ca.CustomerStatus) == "in_alarm",
		}
		switch ca.Alert.Name {
		case alertNameSpendWarning:
			out.Warning, out.HasWarning = t, true
		case alertNameSpendLimit:
			out.Limit, out.HasLimit = t, true
		}
	}
	return out, nil
}
