package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const (
	userFeedbackScoreName        = "user_feedback"
	userFeedbackCommentValue     = "comment"
	userFeedbackReactionScoreKey = "reaction"
	userFeedbackCommentScoreKey  = "comment"
	maxLangfuseFeedbackTextChars = 500
)

var traceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type FeedbackScoreRequest struct {
	Source         string                `json:"source" binding:"required"`
	ConversationID string                `json:"conversation_id"`
	ResponseID     string                `json:"response_id" binding:"required"`
	TraceContext   FeedbackTraceContext  `json:"trace_context"`
	Feedback       FeedbackScoreFeedback `json:"feedback"`
	User           FeedbackScoreUser     `json:"user"`
	Timestamp      string                `json:"timestamp"`
}

type FeedbackTraceContext struct {
	Traceparent string `json:"traceparent"`
	Tracestate  string `json:"tracestate"`
}

type FeedbackScoreFeedback struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type FeedbackScoreUser struct {
	ID       string `json:"id" binding:"required"`
	Username string `json:"username"`
}

type FeedbackScoreResponse struct {
	ScoreID string `json:"score_id"`
	TraceID string `json:"trace_id"`
}

func PostDeploymentFeedbackScore(
	log *logger.Logger,
	cfg *config.Config,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		deploymentID := middleware.DeploymentIDFromContext(c)
		if deploymentID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "deployment token required"})
			return
		}

		var req FeedbackScoreRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		traceID, ok := traceIDFromFeedbackContext(req.TraceContext)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "trace context required"})
			return
		}

		score, ok := langfuseScoreFromFeedback(deploymentID, traceID, req)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported feedback"})
			return
		}

		dep, err := deploymentStore.GetDeploymentByID(deploymentID)
		if err != nil || dep == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
			return
		}

		creds, err := langfuseStore.Get(dep.AccountID)
		if err != nil {
			log.Error("feedback score: langfuse credentials lookup failed", "error", err, "deployment_id", deploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "langfuse credentials lookup failed"})
			return
		}
		if creds == nil || cfg.Deployment.LangfuseBaseURL == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "langfuse not configured for this account"})
			return
		}

		client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

		// Ownership is a best-effort gate. Feedback routinely arrives before
		// Langfuse has ingested the trace, so a not-found lookup is expected and
		// must not block scoring — Langfuse associates the score to its trace by
		// ID once the trace lands. Reject only when the trace exists and is
		// tagged for a different deployment; a genuine lookup failure is still an
		// error.
		if trace, err := client.GetTraceCore(c.Request.Context(), traceID); err == nil {
			if !traceHasDeploymentTag(trace.Tags, deploymentID) {
				c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
				return
			}
		} else if !errors.Is(err, langfuse.ErrNotFound) {
			log.Error("feedback score: langfuse trace lookup failed", "error", err, "deployment_id", deploymentID, "trace_id", traceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to verify trace ownership"})
			return
		}

		if err := client.CreateScore(c.Request.Context(), score); err != nil {
			log.Error("feedback score: langfuse create score failed",
				"error", err,
				"deployment_id", deploymentID,
				"trace_id", traceID,
				"score_name", score.Name,
			)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to create langfuse score"})
			return
		}

		c.JSON(http.StatusAccepted, FeedbackScoreResponse{
			ScoreID: score.ID,
			TraceID: traceID,
		})
	}
}

func langfuseScoreFromFeedback(deploymentID, traceID string, req FeedbackScoreRequest) (langfuse.CreateScoreRequest, bool) {
	switch req.Feedback.Kind {
	case "thumbs_up", "thumbs_down":
		return langfuse.CreateScoreRequest{
			ID:       stableScoreID(deploymentID, req.Source, req.ResponseID, req.User.ID, userFeedbackScoreName, userFeedbackReactionScoreKey),
			TraceID:  traceID,
			Name:     userFeedbackScoreName,
			Value:    req.Feedback.Kind,
			DataType: "CATEGORICAL",
		}, true
	case "comment":
		text := trimLangfuseFeedbackText(req.Feedback.Text)
		if text == "" {
			return langfuse.CreateScoreRequest{}, false
		}
		return langfuse.CreateScoreRequest{
			ID:       stableScoreID(deploymentID, req.Source, req.ResponseID, req.User.ID, userFeedbackScoreName, userFeedbackCommentScoreKey),
			TraceID:  traceID,
			Name:     userFeedbackScoreName,
			Value:    userFeedbackCommentValue,
			DataType: "CATEGORICAL",
			Comment:  text,
		}, true
	default:
		return langfuse.CreateScoreRequest{}, false
	}
}

func traceIDFromFeedbackContext(trace FeedbackTraceContext) (string, bool) {
	parts := strings.Split(strings.TrimSpace(trace.Traceparent), "-")
	if len(parts) != 4 {
		return "", false
	}
	if id := normalizeTraceID(parts[1]); id != "" {
		return id, true
	}
	return "", false
}

func normalizeTraceID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if !traceIDPattern.MatchString(id) || id == "00000000000000000000000000000000" {
		return ""
	}
	return id
}

func trimLangfuseFeedbackText(text string) string {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= maxLangfuseFeedbackTextChars {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxLangfuseFeedbackTextChars])
}

func stableScoreID(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return "astro-feedback-" + hex.EncodeToString(h.Sum(nil))[:32]
}
