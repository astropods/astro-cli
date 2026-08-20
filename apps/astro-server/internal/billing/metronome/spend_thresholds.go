package metronome

import (
	"context"
	"fmt"

	"github.com/Metronome-Industries/metronome-go/v3"
	"github.com/Metronome-Industries/metronome-go/v3/packages/param"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// The names come from internal/billing so the webhook path recognises the same
// strings. The SDK's alert type carries no customer_id, so the name is the only
// discriminator a read has.
const (
	alertNameSpendWarning = billing.SpendWarningAlertName
	alertNameSpendLimit   = billing.SpendLimitAlertName
)

// CustomerSpendThresholds returns the customer's own warning and limit. An
// absent threshold reports HasWarning or HasLimit false rather than zero, which
// is a threshold a customer could legitimately set.
func (p *Provider) CustomerSpendThresholds(ctx context.Context, customerID string) (billing.SpendThresholds, error) {
	var out billing.SpendThresholds
	iter := p.mc.V1.Customers.Alerts.ListAutoPaging(ctx, metronome.V1CustomerAlertListParams{
		CustomerID: customerID,
	})
	for iter.Next() {
		ca := iter.Current()
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
	if err := iter.Err(); err != nil {
		return billing.SpendThresholds{}, fmt.Errorf("metronome list customer alerts: %w", err)
	}
	return out, nil
}

// alertName maps a kind to the name reads discriminate on. The list endpoint
// returns no customer_id and the SDK exposes no uniqueness key, so the name is
// the only handle a read has.
func alertName(kind billing.SpendThresholdKind) string {
	if kind == billing.SpendThresholdLimit {
		return alertNameSpendLimit
	}
	return alertNameSpendWarning
}

// alertUniquenessKey scopes the write-side handle to one customer, so a repeat
// create 409s instead of stacking a second alert on the same account.
func alertUniquenessKey(kind billing.SpendThresholdKind, customerID string) string {
	return alertName(kind) + ":" + customerID
}

// existingAlert returns the customer's alert of this kind, or "" when it has
// none. The threshold comes back too, so a caller can skip a write that would
// set the number already in force.
func (p *Provider) existingAlert(ctx context.Context, customerID string, kind billing.SpendThresholdKind) (id string, threshold float64, err error) {
	iter := p.mc.V1.Customers.Alerts.ListAutoPaging(ctx, metronome.V1CustomerAlertListParams{
		CustomerID: customerID,
	})
	want := alertName(kind)
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

// SetCustomerSpendThreshold puts the customer's own warning or limit at amount.
//
// Metronome has no edit for a threshold notification, so a change is an archive
// followed by a create. The archive must release the uniqueness key or the
// replacement collides with the alert it is replacing. A reset follows, because
// evaluation is otherwise deferred: a customer who raises a limit above current
// spend would stay suspended until Metronome next evaluated on its own.
func (p *Provider) SetCustomerSpendThreshold(ctx context.Context, customerID string, kind billing.SpendThresholdKind, amount float64) error {
	existingID, existing, err := p.existingAlert(ctx, customerID, kind)
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

	created, err := p.mc.V1.Alerts.New(ctx, metronome.V1AlertNewParams{
		AlertType:     metronome.V1AlertNewParamsAlertTypeSpendThresholdReached,
		Name:          alertName(kind),
		Threshold:     amount,
		CustomerID:    param.NewOpt(customerID),
		CreditTypeID:  param.NewOpt(usdCentsCreditTypeID),
		UniquenessKey: param.NewOpt(alertUniquenessKey(kind, customerID)),
	})
	if err != nil {
		return fmt.Errorf("metronome create spend %s: %w", kind, err)
	}
	if err := p.mc.V1.Customers.Alerts.Reset(ctx, metronome.V1CustomerAlertResetParams{
		AlertID:    created.Data.ID,
		CustomerID: customerID,
	}); err != nil {
		return fmt.Errorf("metronome reset spend %s: %w", kind, err)
	}
	return nil
}

// ClearCustomerSpendThreshold removes the customer's own warning or limit.
// Removing one the customer does not have is not an error.
func (p *Provider) ClearCustomerSpendThreshold(ctx context.Context, customerID string, kind billing.SpendThresholdKind) error {
	existingID, _, err := p.existingAlert(ctx, customerID, kind)
	if err != nil || existingID == "" {
		return err
	}
	return p.archiveAlert(ctx, existingID)
}

// archiveAlert removes one alert and frees its uniqueness key, without which the
// next create for the same customer and kind collides forever.
func (p *Provider) archiveAlert(ctx context.Context, alertID string) error {
	if _, err := p.mc.V1.Alerts.Archive(ctx, metronome.V1AlertArchiveParams{
		ID:                   alertID,
		ReleaseUniquenessKey: param.NewOpt(true),
	}); err != nil {
		return fmt.Errorf("metronome archive alert %s: %w", alertID, err)
	}
	return nil
}
