package handlers

import (
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// TransferAgentRequest represents the request to transfer an agent to another account.
type TransferAgentRequest struct {
	TargetAccount string `json:"target_account" binding:"required"`
}

// TransferAgent handles POST /api/v1/agents/:account/:name/transfer
// Moves an agent and all its versions from the source account to the target account.
// The caller must be a member of both accounts. The agent's ECR namespace is preserved
// so existing images continue to resolve correctly.
func TransferAgent(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore, avatarStore *avatar.Store, auditStore *auditlog.Store, deployStore *deploymentstore.Store, cache k8scache.Cache, queue notifyQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		sourceAccountName := c.Param("account")
		agentName := c.Param("name")

		var req TransferAgentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		// Resolve source account
		sourceAcct, err := accountStore.GetByName(sourceAccountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "source account not found"})
			return
		}

		// Resolve target account
		targetAcct, err := accountStore.GetByName(req.TargetAccount)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "target account not found"})
			return
		}

		// Caller must be a member of both accounts
		if !isAccountMember(c, accountStore, sourceAcct.ID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "you must be a member of the source account"})
			return
		}
		if !isAccountMember(c, accountStore, targetAcct.ID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "you must be a member of the target account"})
			return
		}

		// Verify agent exists in source account
		_, err = index.Get(sourceAcct.ID, agentName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found in source account"})
			return
		}

		// Check for name collision in target account
		if _, err := index.Get(targetAcct.ID, agentName); err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "an agent with this name already exists in the target account"})
			return
		}

		// Transfer
		if err := index.Transfer(sourceAcct.ID, targetAcct.ID, agentName); err != nil {
			log.Error("Failed to transfer agent",
				"agent", agentName,
				"source", sourceAccountName,
				"target", req.TargetAccount,
				"error", err,
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to transfer agent",
				"details": err.Error(),
			})
			return
		}

		// Transfer mutates `source_account_id` on every cross-account
		// deployment of this agent. Bust each affected account's deploy cache
		// so the new lineage (and any latest_build_id changes that follow)
		// shows up immediately.
		//
		// We explicitly bust the SOURCE account first: its cached payload
		// still describes the agent's pre-transfer lineage, and legacy rows
		// with `source_account_id IS NULL` (pre-migration) wouldn't be
		// caught by the lineage lookup below (that query only matches by
		// the NEW target). The target-lineage scan picks up everyone else,
		// including the source if it owns any post-transfer rows.
		_ = deploycache.Invalidate(c.Request.Context(), cache, sourceAcct.ID)
		if affected := deploycache.InvalidateForLineage(c.Request.Context(), cache, deployStore, targetAcct.ID, agentName); len(affected) > 0 {
			log.Info("Transfer: invalidated deploy cache for downstream consumers",
				"agent", agentName,
				"affected_accounts", len(affected),
			)
		}

		// Move avatar in storage if the agent has one
		if avatarStore != nil {
			if exists, _ := avatarStore.AgentAvatarExists(c.Request.Context(), sourceAccountName, agentName); exists {
				if err := avatarStore.MoveAgentAvatar(c.Request.Context(), sourceAccountName, req.TargetAccount, agentName); err != nil {
					log.Warn("Failed to move agent avatar during transfer (avatar may be stale)",
						"agent", agentName,
						"source", sourceAccountName,
						"target", req.TargetAccount,
						"error", err,
					)
				} else if _, err := index.TouchAvatarUpdatedAt(targetAcct.ID, agentName); err != nil {
					log.Warn("Failed to stamp agent avatar_updated_at after transfer",
						"agent", agentName, "target", req.TargetAccount, "error", err)
				}
			}
		}

		log.Info("Agent transferred",
			"agent", agentName,
			"source", sourceAccountName,
			"target", req.TargetAccount,
			"user_id", user.ID,
		)

		evt := auditlog.FromGinContext(c, sourceAcct.ID)
		evt.Action = auditlog.AgentTransfer
		evt.ResourceType = "agent"
		evt.ResourceID = agentName
		evt.ResourceName = agentName
		evt.Description = "Transferred agent " + agentName + " to " + req.TargetAccount
		evt.Metadata = map[string]any{
			"source_account": sourceAccountName,
			"target_account": req.TargetAccount,
		}
		auditStore.LogAsync(log, evt)

		emitNotify(c, log, queue, notify.AgentTransferred(targetAcct.ID, req.TargetAccount, agentName))

		c.JSON(http.StatusOK, gin.H{
			"message":        "agent transferred successfully",
			"agent":          agentName,
			"source_account": sourceAccountName,
			"target_account": req.TargetAccount,
		})
	}
}
