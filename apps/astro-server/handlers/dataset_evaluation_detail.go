package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalrunstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
)

type DatasetTraceEvaluator struct {
	Key         string            `json:"key"`
	Label       string            `json:"label,omitempty"`
	Description string            `json:"description,omitempty"`
	Type        string            `json:"type,omitempty"`
	Output      *evaluator.Output `json:"output,omitempty"`
	Status      string            `json:"status"`
	Value       any               `json:"value"`
	Confidence  float64           `json:"confidence"`
	Explanation string            `json:"explanation"`
	Error       *string           `json:"error"`
}

type DatasetTraceEvaluationResponse struct {
	TraceID       string                  `json:"trace_id"`
	UserID        string                  `json:"user_id,omitempty"`
	UserDetails   *UserDetails            `json:"user_details,omitempty"`
	Input         any                     `json:"input"`
	Output        any                     `json:"output"`
	EvaluationRef string                  `json:"evaluation_ref"`
	Run           *DatasetReviewQueueRun  `json:"run"`
	Evaluators    []DatasetTraceEvaluator `json:"evaluators"`
}

type datasetTraceEvaluationStore interface {
	LatestRuns(context.Context, string, []string) (map[string]evalrunstore.Run, error)
	EvaluatorResults(context.Context, string) ([]evalrunstore.Result, error)
}

// GetDatasetTraceEvaluation returns the active evaluation set for one trace,
// with each evaluator's recorded result. The review queue carries only run
// status, so this is what a selected trace expands into.
func GetDatasetTraceEvaluation(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore *langfuse.Store,
	runStore datasetTraceEvaluationStore,
	slackStore *slackidentity.Store,
	resolver evalSetResolver,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}
		traceID := strings.TrimSpace(c.Param("trace_id"))
		if traceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "trace id is required"})
			return
		}

		dataset, ok := loadDataset(c, log, datasetStore, lctx.DeploymentID)
		if !ok {
			return
		}

		trace, err := lctx.Client.GetTrace(c.Request.Context(), traceID)
		if err != nil {
			if errors.Is(err, langfuse.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
				return
			}
			log.Error("trace evaluation: load trace failed", "error", err, "deployment_id", lctx.DeploymentID, "trace_id", traceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load the trace"})
			return
		}
		if !langfuse.HasDeploymentTag(trace.Tags, lctx.DeploymentID) {
			c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
			return
		}

		response := DatasetTraceEvaluationResponse{
			TraceID:    traceID,
			UserID:     trace.UserID,
			Input:      trace.Input,
			Output:     trace.Output,
			Evaluators: []DatasetTraceEvaluator{},
		}
		if trace.UserID != "" {
			hydrator := newUserDetailsHydrator(log, slackStore, accountStore, []string{trace.UserID}, "dataset-trace-evaluation")
			response.UserDetails = traceUserDetailsFromHydrator(trace.UserID, hydrator)
		}

		runs, err := runStore.LatestRuns(c.Request.Context(), dataset.ID, []string{traceID})
		if err != nil {
			log.Error("trace evaluation: load evaluation run failed", "error", err, "deployment_id", lctx.DeploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load the evaluation run"})
			return
		}
		run, hasRun := runs[traceID]
		if !hasRun {
			c.JSON(http.StatusOK, response)
			return
		}

		response.EvaluationRef = run.EvaluationRef
		queueRun := DatasetReviewQueueRun{ID: run.ID, Status: string(run.Status)}
		if run.ErrorMessage != "" {
			message := run.ErrorMessage
			queueRun.Error = &message
		}
		response.Run = &queueRun

		results, err := runStore.EvaluatorResults(c.Request.Context(), run.ID)
		if err != nil {
			log.Error("trace evaluation: load evaluator results failed", "error", err, "deployment_id", lctx.DeploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load evaluator results"})
			return
		}
		response.Evaluators = orderedTraceEvaluators(c.Request.Context(), resolver, run.EvaluationRef, results)
		c.JSON(http.StatusOK, response)
	}
}

// orderedTraceEvaluators returns one entry per result. A reference this build
// cannot resolve costs labels and ordering rather than failing the read.
func orderedTraceEvaluators(
	ctx context.Context,
	resolver evalSetResolver,
	evaluationRef string,
	results []evalrunstore.Result,
) []DatasetTraceEvaluator {
	set, _ := resolver.Set(ctx, evaluationRef)
	groups := evaluatorsBySet(set, results,
		func(result evalrunstore.Result) string { return result.EvaluatorKey })

	out := make([]DatasetTraceEvaluator, 0, len(groups))
	for _, group := range groups {
		out = append(out, newDatasetTraceEvaluator(group.Rows[0], group.Definition))
	}
	return out
}

func newDatasetTraceEvaluator(
	result evalrunstore.Result,
	definition evaluator.Evaluator,
) DatasetTraceEvaluator {
	out := DatasetTraceEvaluator{
		Key:         result.EvaluatorKey,
		Status:      string(result.Status),
		Value:       result.Value,
		Confidence:  result.Confidence,
		Explanation: result.Explanation,
	}
	if definition.Key != "" {
		output := definition.Output
		out.Label, out.Type, out.Output = definition.Label, string(definition.Type), &output
		out.Description = definition.Description
	}
	if result.ErrorMessage != "" {
		message := result.ErrorMessage
		out.Error = &message
	}
	return out
}
