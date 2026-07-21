package metering

import (
	"context"
	"database/sql"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
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

// EmitKnowledgeStorage queries all managed knowledge stores for an account and emits
// a knowledge_storage_provisioned event per store with the provisioned GB. Called inline
// after create/delete. This is a metered-consumption signal (knowledge storage), not a
// resource count — counts are served from the quota DB.
func EmitKnowledgeStorage(ctx context.Context, provider billing.BillingProvider, db *sql.DB, log *logger.Logger, accountID string) {
	if provider == nil {
		return
	}
	rows, err := db.QueryContext(ctx,
		"SELECT name, provider, storage FROM knowledge_stores WHERE account_id = $1 AND mode = 'managed' AND status != 'error'",
		accountID,
	)
	if err != nil {
		log.Error("Failed to query knowledge storage for event", "error", err, "account_id", accountID)
		return
	}
	defer rows.Close() //nolint:errcheck

	var events []billing.UsageEvent
	for rows.Next() {
		var name, provName, storage string
		if err := rows.Scan(&name, &provName, &storage); err != nil {
			log.Error("Failed to scan knowledge store row", "error", err, "account_id", accountID)
			continue
		}
		gb := storageToGB(storage)
		if gb <= 0 {
			continue
		}
		events = append(events, usageEvent("knowledge_storage_provisioned", accountID, map[string]any{
			"storage_gb": gb,
			"store_name": name,
			"provider":   provName,
		}))
	}
	if err := rows.Err(); err != nil {
		log.Error("Failed to iterate knowledge store rows", "error", err, "account_id", accountID)
	}
	if len(events) > 0 {
		if err := provider.IngestUsage(ctx, events); err != nil {
			log.Error("Failed to emit knowledge_storage_provisioned events", "error", err, "account_id", accountID)
		} else {
			log.Info("metering: emitted knowledge_storage_provisioned", "account_id", accountID, "events", len(events))
		}
	}
}

// storageToGB parses a K8s quantity string (e.g. "10Gi", "500Mi") to GB.
// Uses the same suffix table as parseMemory in heartbeat.go.
func storageToGB(s string) float64 {
	return parseMemory(s)
}

// knowledgeCU calculates compute units for a knowledge store based on its provider's
// default resource requests. Same formula as deployment compute: CU = max(cpu, mem/2).
func knowledgeCU(provider string) float64 {
	r := knowledgeProviderResourceStrings(provider)
	return math.Max(parseCPU(r.cpu), parseMemory(r.memory)/2)
}
