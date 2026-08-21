package handlers

import (
	"database/sql"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const (
	feedbackMaxMessageLen = 5000
	feedbackRateLimitPerH = 10
)

// FeedbackInput is the request body for submitting feedback.
type FeedbackInput struct {
	Message string `json:"message"`
	PageURL string `json:"page_url,omitempty"`
}

// FeedbackResponse is the response after submitting feedback.
type FeedbackResponse struct {
	ID string `json:"id"`
}

// SubmitFeedback handles POST /api/v1/feedback.
func SubmitFeedback(log *logger.Logger, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		var input FeedbackInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		input.Message = strings.TrimSpace(input.Message)
		input.PageURL = strings.TrimSpace(input.PageURL)

		if input.Message == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
			return
		}

		if utf8.RuneCountInString(input.Message) > feedbackMaxMessageLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": "message must be 5000 characters or fewer"})
			return
		}

		if len(input.PageURL) > 2048 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "page_url too long"})
			return
		}

		// Rate limit: max N submissions per user per hour
		var recentCount int
		err := db.QueryRowContext(c.Request.Context(),
			`SELECT COUNT(*) FROM feedback_submissions
			 WHERE user_id = $1 AND created_at > now() - interval '1 hour'`,
			user.ID,
		).Scan(&recentCount)
		if err != nil {
			log.Error("feedback: check feedback rate limit failed", "error", err, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to submit feedback"})
			return
		}

		if recentCount >= feedbackRateLimitPerH {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many feedback submissions, please try again later"})
			return
		}

		var id string
		err = db.QueryRowContext(c.Request.Context(),
			`INSERT INTO feedback_submissions (user_id, user_email, message, page_url)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id`,
			user.ID, user.Email, input.Message, input.PageURL,
		).Scan(&id)
		if err != nil {
			log.Error("feedback: insert feedback failed", "error", err, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to submit feedback"})
			return
		}

		log.Info("feedback: submitted", "feedback_id", id, "user_id", user.ID)
		c.JSON(http.StatusCreated, FeedbackResponse{ID: id})
	}
}
