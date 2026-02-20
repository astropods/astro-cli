package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/auth"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"github.com/postman/astro/apps/astro-server/internal/middleware"
)

// CreateAccountRequest represents the request body for creating an account
type CreateAccountRequest struct {
	Name string `json:"name" binding:"required"`
	Type string `json:"type" binding:"required"`
}

// AccountOwner represents the owner's public profile in account responses
type AccountOwner struct {
	FirstName         string `json:"first_name,omitempty"`
	LastName          string `json:"last_name,omitempty"`
	ProfilePictureURL string `json:"profile_picture_url,omitempty"`
}

// AccountResponse represents an account in API responses
type AccountResponse struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Type      string        `json:"type"`
	Owner     *AccountOwner `json:"owner,omitempty"`
	CreatedAt string        `json:"created_at"`
	UpdatedAt string        `json:"updated_at"`
}

// AccountWithRoleResponse represents an account with the user's role
type AccountWithRoleResponse struct {
	ID     string              `json:"id"`
	Name   string              `json:"name"`
	Type   string              `json:"type"`
	Role   string              `json:"role"`
	Agents []AgentSummary      `json:"agents,omitempty"`
}

// AgentSummary represents a brief summary of an agent for the profile response
type AgentSummary struct {
	Name                  string `json:"name"`
	Registry              string `json:"registry"`
	BuildCount            int    `json:"build_count"`
	PublishedVersionCount int    `json:"published_version_count"`
}

// ProfileResponse represents the /api/v1/me response
type ProfileResponse struct {
	User     *ProfileUser              `json:"user"`
	Accounts []AccountWithRoleResponse `json:"accounts"`
}

// ProfileUser is a subset of user info for the profile endpoint
type ProfileUser struct {
	ID        string `json:"id"`
	Email     string `json:"email,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

// CreateAccount handles POST /api/v1/accounts
func CreateAccount(log *logger.Logger, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CreateAccountRequest
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

		// For personal accounts, check user doesn't already have one
		if req.Type == "personal" {
			hasPersonal, err := accountStore.HasPersonalAccount(user.ID)
			if err != nil {
				log.Error("Failed to check personal account", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check existing accounts"})
				return
			}
			if hasPersonal {
				c.JSON(http.StatusConflict, gin.H{"error": "you already have a personal account"})
				return
			}
		}

		acct, err := accountStore.Create(req.Name, req.Type, user.ID)
		if err != nil {
			log.Error("Failed to create account", "error", err, "name", req.Name)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to create account",
				"details": err.Error(),
			})
			return
		}

		log.Info("Account created", "id", acct.ID, "name", acct.Name, "type", acct.Type, "user_id", user.ID)

		c.JSON(http.StatusCreated, AccountResponse{
			ID:        acct.ID,
			Name:      acct.Name,
			Type:      acct.Type,
			CreatedAt: acct.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt: acct.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
}

// GetAccount handles GET /api/v1/accounts/:account (public)
func GetAccount(log *logger.Logger, accountStore *account.AccountStore, workos *auth.WorkOSClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		resp := AccountResponse{
			ID:        acct.ID,
			Name:      acct.Name,
			Type:      acct.Type,
			CreatedAt: acct.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt: acct.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}

		// Best-effort: look up owner profile for public display
		if ownerID, err := accountStore.GetOwnerUserID(acct.ID); err == nil {
			if user, err := workos.GetUser(c.Request.Context(), ownerID); err == nil {
				resp.Owner = &AccountOwner{
					FirstName:         user.FirstName,
					LastName:          user.LastName,
					ProfilePictureURL: user.ProfilePictureURL,
				}
			}
		}

		c.JSON(http.StatusOK, resp)
	}
}

// RenameAccountRequest represents the request body for renaming an account
type RenameAccountRequest struct {
	Name string `json:"name" binding:"required"`
}

// RenameAccount handles PUT /api/v1/accounts/:account (owner only)
func RenameAccount(log *logger.Logger, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RenameAccountRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		if err := accountStore.Rename(acct.ID, req.Name); err != nil {
			log.Error("Failed to rename account", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to rename account",
				"details": err.Error(),
			})
			return
		}

		log.Info("Account renamed", "id", acct.ID, "old_name", acct.Name, "new_name", req.Name)

		c.JSON(http.StatusOK, gin.H{
			"message": "account renamed",
			"name":    req.Name,
		})
	}
}

// GetProfile handles GET /api/v1/me (protected)
func GetProfile(log *logger.Logger, accountStore *account.AccountStore, agentIndex *agentindex.Index) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		accounts, err := accountStore.GetAccountsForUser(user.ID)
		if err != nil {
			log.Error("Failed to get accounts for user", "error", err, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get accounts"})
			return
		}

		accountResponses := make([]AccountWithRoleResponse, 0, len(accounts))
		for _, a := range accounts {
			resp := AccountWithRoleResponse{
				ID:   a.ID,
				Name: a.Name,
				Type: a.Type,
				Role: a.Role,
			}

			// Include agent summaries for each account
			agents, err := agentIndex.ListForAccount(a.ID)
			if err == nil {
				summaries := make([]AgentSummary, 0, len(agents))
				for _, agent := range agents {
					publishedVersions, _ := agentIndex.GetPublishedVersionsForAgent(a.ID, agent.Name)
					summaries = append(summaries, AgentSummary{
						Name:                  agent.Name,
						Registry:              agent.Registry,
						BuildCount:            len(agent.Versions),
						PublishedVersionCount: len(publishedVersions),
					})
				}
				resp.Agents = summaries
			}

			accountResponses = append(accountResponses, resp)
		}

		c.JSON(http.StatusOK, ProfileResponse{
			User: &ProfileUser{
				ID:        user.ID,
				Email:     user.Email,
				FirstName: user.FirstName,
				LastName:  user.LastName,
			},
			Accounts: accountResponses,
		})
	}
}

// CheckAccountName handles GET /api/v1/accounts/check/:name (public)
// Returns whether an account name is available
func CheckAccountName(log *logger.Logger, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")

		// Validate name format first
		if err := account.ValidateAccountName(name); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"available": false,
				"reason":    err.Error(),
			})
			return
		}

		// Check if name is taken
		_, err := accountStore.GetByName(name)
		if err != nil {
			// Not found = available
			c.JSON(http.StatusOK, gin.H{"available": true})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"reason":    "name is already taken",
		})
	}
}
