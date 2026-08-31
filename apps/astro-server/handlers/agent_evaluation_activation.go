package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/evalagentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldefinitionstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldocument"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
)

type RegisterAgentEvaluation struct {
	EvaluationYAML string            `json:"evaluation_yaml"`
	PromptFiles    map[string]string `json:"prompt_files"`
}

type AgentEvaluationActivationResponse struct {
	EvaluationRef string `json:"evaluation_ref"`
}

type evaluationActivation struct {
	clear          bool
	evaluationRef  string
	definitionJSON json.RawMessage
}

func parseEvaluationActivation(raw json.RawMessage) (*evaluationActivation, error) {
	if raw == nil {
		return nil, nil
	}
	if string(raw) == "null" {
		return &evaluationActivation{clear: true}, nil
	}
	var body RegisterAgentEvaluation
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("invalid evaluation object: %w", err)
	}
	result, err := evaldocument.Parse(body.EvaluationYAML, body.PromptFiles)
	if err != nil {
		return nil, err
	}
	definitionJSON, err := json.Marshal(result.Document)
	if err != nil {
		return nil, fmt.Errorf("marshal evaluation document: %w", err)
	}
	return &evaluationActivation{evaluationRef: result.EvaluationRef, definitionJSON: definitionJSON}, nil
}

func applyEvaluationActivation(
	ctx context.Context,
	db *sql.DB,
	accountID, agentName string,
	activation *evaluationActivation,
) error {
	if activation == nil {
		return nil
	}
	if activation.clear {
		return evalagentstore.NewStore(db).Clear(ctx, accountID, agentName)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin evaluation activation: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if err := evaldefinitionstore.CreateTx(ctx, tx, activation.evaluationRef, activation.definitionJSON); err != nil {
		return err
	}
	if err := evalagentstore.SetTx(ctx, tx, accountID, agentName, activation.evaluationRef); err != nil {
		return err
	}
	return tx.Commit()
}

// PutAgentEvaluationSet activates a custom evaluation set for an agent,
// independently of any build or registration.
// PUT /api/v1/agents/:account/:name/evaluation-set
func PutAgentEvaluationSet(log *logger.Logger, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentName := c.Param("name")

		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		body, err := c.GetRawData()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		activation, err := parseEvaluationActivation(body)
		if err != nil {
			log.Error("agent evaluation set: invalid evaluation configuration", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid evaluation configuration",
				"details": err.Error(),
			})
			return
		}
		if activation == nil || activation.clear {
			c.JSON(http.StatusBadRequest, gin.H{"error": "evaluation_yaml is required"})
			return
		}

		if err := applyEvaluationActivation(c.Request.Context(), db, acct.ID, agentName, activation); err != nil {
			log.Error("agent evaluation set: activate evaluation set failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to activate the evaluation set",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, AgentEvaluationActivationResponse{EvaluationRef: activation.evaluationRef})
	}
}
