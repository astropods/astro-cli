package openmeter

import (
	"context"
	"database/sql"
	"math"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// EmitAgentBuild emits an agent_build event when a new build is registered.
func EmitAgentBuild(ctx context.Context, client *Client, log *logger.Logger, accountID, agentName string) {
	if client == nil {
		return
	}
	ev := NewCloudEvent("agent_build", accountID, map[string]string{
		"agent_name": agentName,
	})
	if err := client.IngestEvents(ctx, []CloudEvent{ev}); err != nil {
		log.Error("Failed to emit agent_build event", "error", err, "account_id", accountID, "agent_name", agentName)
	}
}

// EmitActiveAgents queries the current registered agent count for an account and emits
// an active_agents event. Called inline after agent registration so entitlement checks
// on the next request see the updated count without waiting for the heartbeat.
func EmitActiveAgents(ctx context.Context, client *Client, db *sql.DB, log *logger.Logger, accountID string) {
	if client == nil {
		return
	}
	var count int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM agents WHERE account_id = $1 AND archived_at IS NULL",
		accountID,
	).Scan(&count); err != nil {
		log.Error("Failed to query agent count for event", "error", err, "account_id", accountID)
		return
	}
	ev := NewCloudEvent("active_agents", accountID, map[string]any{
		"count": count,
	})
	if err := client.IngestEvents(ctx, []CloudEvent{ev}); err != nil {
		log.Error("Failed to emit active_agents event", "error", err, "account_id", accountID)
	}
}

// EmitActiveDeployments queries the current deployment count (pending + active) for an
// account and emits an active_deployments event. Called inline after deploy/undeploy so
// entitlement checks on the next request see the updated count without waiting for the
// heartbeat (which only runs every 5 minutes and only counts active deployments).
func EmitActiveDeployments(ctx context.Context, client *Client, db *sql.DB, log *logger.Logger, accountID string) {
	if client == nil {
		return
	}
	var count int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM deployments WHERE account_id = $1 AND status IN ('pending', 'active', 'scaled_down')",
		accountID,
	).Scan(&count); err != nil {
		log.Error("Failed to query deployment count for event", "error", err, "account_id", accountID)
		return
	}
	ev := NewCloudEvent("active_deployments", accountID, map[string]any{
		"count": count,
	})
	if err := client.IngestEvents(ctx, []CloudEvent{ev}); err != nil {
		log.Error("Failed to emit active_deployments event", "error", err, "account_id", accountID)
	}
}

// EmitActiveMembers queries the current member count for an account and emits
// an active_members event. Called inline after member add/remove for immediate
// entitlement updates (the heartbeat also reconciles every 5 minutes).
func EmitActiveMembers(ctx context.Context, client *Client, db *sql.DB, log *logger.Logger, accountID string) {
	if client == nil {
		return
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM account_members WHERE account_id = $1", accountID).Scan(&count); err != nil {
		log.Error("Failed to query member count for event", "error", err, "account_id", accountID)
		return
	}
	ev := NewCloudEvent("active_members", accountID, map[string]any{
		"count": count,
	})
	if err := client.IngestEvents(ctx, []CloudEvent{ev}); err != nil {
		log.Error("Failed to emit active_members event", "error", err, "account_id", accountID)
	}
}

// EmitActiveKnowledgeStores queries the current knowledge store count (managed + external,
// excluding errored) for an account and emits an active_knowledge_stores event. Called
// inline after create/connect/delete for immediate entitlement updates.
func EmitActiveKnowledgeStores(ctx context.Context, client *Client, db *sql.DB, log *logger.Logger, accountID string) {
	if client == nil {
		return
	}
	var count int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM knowledge_stores WHERE account_id = $1 AND status != 'error'",
		accountID,
	).Scan(&count); err != nil {
		log.Error("Failed to query knowledge store count for event", "error", err, "account_id", accountID)
		return
	}
	ev := NewCloudEvent("active_knowledge_stores", accountID, map[string]any{
		"count": count,
	})
	if err := client.IngestEvents(ctx, []CloudEvent{ev}); err != nil {
		log.Error("Failed to emit active_knowledge_stores event", "error", err, "account_id", accountID)
	}
}

// EmitKnowledgeStorage queries all managed knowledge stores for an account and emits
// a knowledge_storage_provisioned event per store with the provisioned GB. Called inline
// after create/delete for immediate entitlement updates.
func EmitKnowledgeStorage(ctx context.Context, client *Client, db *sql.DB, log *logger.Logger, accountID string) {
	if client == nil {
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

	var events []CloudEvent
	for rows.Next() {
		var name, provider, storage string
		if err := rows.Scan(&name, &provider, &storage); err != nil {
			log.Error("Failed to scan knowledge store row", "error", err, "account_id", accountID)
			continue
		}
		gb := storageToGB(storage)
		if gb <= 0 {
			continue
		}
		events = append(events, NewCloudEvent("knowledge_storage_provisioned", accountID, map[string]any{
			"storage_gb": gb,
			"store_name": name,
			"provider":   provider,
		}))
	}
	if err := rows.Err(); err != nil {
		log.Error("Failed to iterate knowledge store rows", "error", err, "account_id", accountID)
	}
	if len(events) > 0 {
		if err := client.IngestEvents(ctx, events); err != nil {
			log.Error("Failed to emit knowledge_storage_provisioned events", "error", err, "account_id", accountID)
		}
	}
}

// EmitActiveKnowledgeEndpoints queries the current PrivateLink endpoint count for an
// account and emits an active_knowledge_endpoints event. Called inline after connecting
// an external store with PrivateLink.
func EmitActiveKnowledgeEndpoints(ctx context.Context, client *Client, db *sql.DB, log *logger.Logger, accountID string) {
	if client == nil {
		return
	}
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_store_endpoints kse
		 JOIN knowledge_stores ks ON ks.id = kse.knowledge_store_id
		 WHERE ks.account_id = $1 AND kse.status != 'error'`,
		accountID,
	).Scan(&count); err != nil {
		log.Error("Failed to query knowledge endpoint count for event", "error", err, "account_id", accountID)
		return
	}
	ev := NewCloudEvent("active_knowledge_endpoints", accountID, map[string]any{
		"count": count,
	})
	if err := client.IngestEvents(ctx, []CloudEvent{ev}); err != nil {
		log.Error("Failed to emit active_knowledge_endpoints event", "error", err, "account_id", accountID)
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
	type resources struct{ cpu, memory string }
	defaults := map[string]resources{
		"postgres": {"250m", "256Mi"},
		"redis":    {"50m", "64Mi"},
		"qdrant":   {"250m", "512Mi"},
		"neo4j":    {"500m", "512Mi"},
	}
	r, ok := defaults[provider]
	if !ok {
		r = resources{"100m", "128Mi"}
	}
	return math.Max(parseCPU(r.cpu), parseMemory(r.memory)/2)
}
