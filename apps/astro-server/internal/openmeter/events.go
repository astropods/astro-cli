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
