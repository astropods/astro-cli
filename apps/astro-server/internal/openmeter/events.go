package openmeter

import (
	"context"
	"database/sql"

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
