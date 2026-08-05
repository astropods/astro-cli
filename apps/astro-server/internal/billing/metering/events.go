package metering

import (
	"time"

	"github.com/google/uuid"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// usageEvent builds a billing.UsageEvent with a fresh idempotency UUID and the
// current timestamp. The active billing.BillingProvider maps it to its own wire
// format on ingest.
func usageEvent(eventType, accountID string, props map[string]any) billing.UsageEvent {
	return billing.UsageEvent{
		TransactionID: uuid.New().String(),
		AccountID:     accountID,
		Type:          eventType,
		Time:          time.Now().UTC(),
		Properties:    props,
	}
}
