package openmeter

import (
	"context"

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
