package metering

import (
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// usageEventAt stamps the event with the end of the span it covers rather than
// the moment it was sent. Metronome files usage into a billing period by event
// time, so a catch-up emit would otherwise bill last night's usage to today.
func usageEventAt(txID string, at time.Time, eventType, accountID string, props map[string]any) billing.UsageEvent {
	return billing.UsageEvent{
		TransactionID: txID,
		AccountID:     accountID,
		Type:          eventType,
		Time:          at.UTC(),
		Properties:    props,
	}
}
