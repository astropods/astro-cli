package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
)

type EvaluationSetEvaluator struct {
	Key         string           `json:"key"`
	Label       string           `json:"label"`
	Description string           `json:"description,omitempty"`
	Type        string           `json:"type"`
	Output      evaluator.Output `json:"output"`
}

type EvaluationSetResponse struct {
	EvaluationRef string                   `json:"evaluation_ref"`
	Evaluators    []EvaluationSetEvaluator `json:"evaluators"`
}

// GetAgentEvaluationSet returns the evaluators an agent evaluates against, in
// definition order.
// GET /api/v1/agents/:account/:name/evaluation-set
func GetAgentEvaluationSet(
	log *logger.Logger,
	accountStore *account.AccountStore,
	agentIndex *agentindex.Index,
	resolver evalSetResolver,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		acct, accountErr := accountStore.GetByName(c.Param("account"))
		if accountErr != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		if !isAccountMember(c, accountStore, acct.ID, user.ID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this account"})
			return
		}

		agentName := c.Param("name")
		exists, err := agentIndex.Exists(acct.ID, agentName)
		if err != nil {
			log.Error("evaluation set: look up agent failed", "error", err,
				"account_id", acct.ID, "agent_name", agentName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to look up the agent"})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}

		evaluationRef, err := resolver.ActiveRef(c.Request.Context(), acct.ID, agentName)
		if err != nil {
			log.Error("evaluation set: resolve active ref failed", "error", err,
				"account_id", acct.ID, "agent_name", agentName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve the evaluation set"})
			return
		}
		set, err := resolver.Set(c.Request.Context(), evaluationRef)
		if err != nil {
			log.Error("evaluation set: resolve set failed", "error", err,
				"account_id", acct.ID, "agent_name", agentName, "evaluation_ref", evaluationRef)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve the evaluation set"})
			return
		}

		evaluators := make([]EvaluationSetEvaluator, 0, len(set))
		for _, definition := range set {
			evaluators = append(evaluators, EvaluationSetEvaluator{
				Key:         definition.Key,
				Label:       definition.Label,
				Description: definition.Description,
				Type:        string(definition.Type),
				Output:      definition.Output,
			})
		}

		c.JSON(http.StatusOK, EvaluationSetResponse{
			EvaluationRef: evaluationRef,
			Evaluators:    evaluators,
		})
	}
}
