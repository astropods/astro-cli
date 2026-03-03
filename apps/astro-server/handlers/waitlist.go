package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"github.com/postman/astro/apps/astro-server/internal/waitlist"
)

// WaitlistSignupRequest represents the request body for joining the waitlist
type WaitlistSignupRequest struct {
	Name  string `json:"name" binding:"required"`
	Email string `json:"email" binding:"required,email"`
}

// JoinWaitlist handles POST /api/v1/waitlist
func JoinWaitlist(log *logger.Logger, store *waitlist.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req WaitlistSignupRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Name and email are required."})
			return
		}

		req.Email = strings.ToLower(strings.TrimSpace(req.Email))
		req.Name = strings.TrimSpace(req.Name)

		entry, err := store.Add(req.Email, req.Name)
		if err != nil {
			// Check for unique constraint violation (duplicate email)
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				c.JSON(http.StatusConflict, gin.H{"error": "This email is already on the waitlist."})
				return
			}
			log.Error("Failed to add to waitlist", "error", err, "email", req.Email)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to join waitlist."})
			return
		}

		c.JSON(http.StatusCreated, entry)
	}
}
