package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/gin-gonic/gin"
)

// CreateAccountRequest represents the request body for creating an account
type CreateAccountRequest struct {
	Name        string `json:"name" binding:"required"`
	Type        string `json:"type" binding:"required"`
	Invitations []struct {
		Value string `json:"value"`
		Kind  string `json:"kind"`
		Role  string `json:"role"`
	} `json:"invitations,omitempty"`
}

// AccountOwner represents the owner's public profile in account responses
type AccountOwner struct {
	FirstName         string `json:"first_name,omitempty"`
	LastName          string `json:"last_name,omitempty"`
	ProfilePictureURL string `json:"profile_picture_url,omitempty"`
}

// AccountResponse represents an account in API responses
type AccountResponse struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Type          string             `json:"type"`
	Owner         *AccountOwner      `json:"owner,omitempty"`
	Invitations   []org.InviteResult `json:"invitations,omitempty"`
	AvatarVersion int                `json:"avatar_version"`
	CreatedAt     string             `json:"created_at"`
	UpdatedAt     string             `json:"updated_at"`
}

// AccountWithRoleResponse represents an account in the profile response
type AccountWithRoleResponse struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Type          string         `json:"type"`
	AvatarVersion int            `json:"avatar_version"`
	Agents        []AgentSummary `json:"agents,omitempty"`
}

// AgentSummary represents a brief summary of an agent for the profile response
type AgentSummary struct {
	Name       string `json:"name"`
	Registry   string `json:"registry"`
	Visibility string `json:"visibility"`
	BuildCount int    `json:"build_count"`
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
// For organization accounts, also creates a WorkOS Organization and links it.
// If omClient is non-nil, creates a corresponding OpenMeter customer (non-blocking).
func CreateAccount(log *logger.Logger, accountStore *account.AccountStore, orgClient *org.Client, orgSync *org.Sync, omClient *openmeter.Client, defaultPlan string) gin.HandlerFunc {
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

		// Check reserved/denied names for user registration
		if err := account.CheckAccountNameRestricted(req.Name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid account name",
				"details": err.Error(),
			})
			return
		}

		// Step 1: Create local account (Astro is source of truth)
		acct, err := accountStore.Create(req.Name, req.Type, user.ID)
		if err != nil {
			log.Error("Failed to create account", "error", err, "name", req.Name)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to create account",
				"details": err.Error(),
			})
			return
		}

		// Step 2: For org accounts, create WorkOS Organization and link
		if req.Type == "organization" && orgClient != nil {
			ctx := c.Request.Context()

			// Create WorkOS organization with external_id = account.ID
			workosOrg, err := orgClient.CreateOrganization(ctx, req.Name, acct.ID)
			if err != nil {
				log.Error("Failed to create WorkOS organization", "error", err, "account_id", acct.ID)
				// Compensating action: delete local account
				_ = accountStore.DeleteByID(acct.ID)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "failed to create organization",
					"details": err.Error(),
				})
				return
			}

			// Link WorkOS org to local account
			if err := accountStore.SetWorkOSOrganizationID(acct.ID, workosOrg.ID); err != nil {
				log.Error("Failed to link WorkOS org", "error", err, "account_id", acct.ID)
				_ = orgClient.DeleteOrganization(ctx, workosOrg.ID)
				_ = accountStore.DeleteByID(acct.ID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to link organization"})
				return
			}
			acct.WorkOSOrganizationID = workosOrg.ID

			// Create WorkOS membership for creator as owner
			m, err := orgClient.CreateMembership(ctx, workosOrg.ID, user.ID, "owner")
			if err != nil {
				log.Warn("Failed to create WorkOS membership for org creator", "error", err)
				// Non-fatal: local membership already exists from Create()
			} else {
				// Update local member with WorkOS membership ID
				_ = accountStore.UpsertMemberByWorkosMembershipID(acct.ID, user.ID, m.ID)
			}
		}

		// Create OpenMeter customer (non-blocking — failure is logged, not fatal)
		if omClient != nil {
			customerID, omErr := omClient.CreateCustomer(c.Request.Context(), acct.ID, acct.Name, acct.Type, user.Email)
			if omErr != nil {
				log.Error("Failed to create OpenMeter customer", "error", omErr, "account_id", acct.ID)
			} else {
				if storeErr := accountStore.SetOpenMeterCustomerID(acct.ID, customerID); storeErr != nil {
					log.Error("Failed to store OpenMeter customer ID", "error", storeErr, "account_id", acct.ID)
				}

				// Auto-subscribe to default plan if configured
				if defaultPlan != "" && customerID != "" {
					if subErr := omClient.CreateSubscription(c.Request.Context(), customerID, defaultPlan); subErr != nil {
						log.Error("Failed to auto-subscribe account to default plan", "error", subErr, "account_id", acct.ID, "plan", defaultPlan)
					} else {
						log.Info("Auto-subscribed account to default plan", "account_id", acct.ID, "plan", defaultPlan)
					}
				}
			}
		}

		log.Info("Account created", "id", acct.ID, "name", acct.Name, "type", acct.Type, "user_id", user.ID)

		resp := AccountResponse{
			ID:            acct.ID,
			Name:          acct.Name,
			Type:          acct.Type,
			AvatarVersion: acct.AvatarVersion,
			CreatedAt:     acct.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:     acct.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}

		// Send invitations if provided (non-fatal — log failures, still return success)
		if len(req.Invitations) > 0 && acct.WorkOSOrganizationID != "" && orgSync != nil {
			reqs := make([]org.InviteRequest, 0, len(req.Invitations))
			for _, inv := range req.Invitations {
				role := inv.Role
				if role == "" {
					role = "member"
				}
				reqs = append(reqs, org.InviteRequest{
					Kind:     inv.Kind,
					Value:    inv.Value,
					RoleSlug: role,
				})
			}
			results := orgSync.SendBulkInvitations(c.Request.Context(), acct.WorkOSOrganizationID, user.ID, reqs)
			resp.Invitations = results
			for _, r := range results {
				if !r.Success {
					log.Warn("Invitation failed during account creation", "value", r.Value, "error", r.Error)
				}
			}
		}

		c.JSON(http.StatusCreated, resp)
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
			ID:            acct.ID,
			Name:          acct.Name,
			Type:          acct.Type,
			AvatarVersion: acct.AvatarVersion,
			CreatedAt:     acct.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:     acct.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}

		// Best-effort: look up owner profile for personal accounts
		if acct.Type == "personal" {
			if ownerID, err := accountStore.GetFirstMemberUserID(acct.ID); err == nil {
				if user, err := workos.GetUser(c.Request.Context(), ownerID); err == nil {
					resp.Owner = &AccountOwner{
						FirstName:         user.FirstName,
						LastName:          user.LastName,
						ProfilePictureURL: user.ProfilePictureURL,
					}
				}
			}
		}

		c.JSON(http.StatusOK, resp)
	}
}

// DeleteAccount handles DELETE /api/v1/accounts/:account (owner only)
// Soft-deletes the account, enqueues undeploy jobs for active deployments,
// and cleans up WorkOS org best-effort.
func DeleteAccount(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, queue DeployQueue, orgClient *org.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		ctx := c.Request.Context()

		// Soft-delete — point of no return
		if err := accountStore.MarkDeleted(acct.ID); err != nil {
			if strings.Contains(err.Error(), "not found or already deleted") {
				c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
				return
			}
			log.Error("Failed to mark account deleted", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete account"})
			return
		}

		// Enqueue undeploy for all visible deployments (reuses existing undeploy pipeline)
		deps, err := deployStore.GetVisibleDeploymentsByAccount(acct.ID)
		if err != nil {
			log.Error("Failed to list deployments for deleted account", "error", err, "account_id", acct.ID)
		} else {
			for _, dep := range deps {
				if err := EnqueueUndeploy(ctx, deployStore, queue, dep.ID); err != nil {
					log.Error("Failed to enqueue undeploy for deleted account", "error", err, "deployment_id", dep.ID, "account_id", acct.ID)
				}
			}
		}

		// Clean up WorkOS organization (best-effort)
		if acct.WorkOSOrganizationID != "" && orgClient != nil {
			if err := orgClient.DeleteOrganization(ctx, acct.WorkOSOrganizationID); err != nil {
				log.Error("Failed to delete WorkOS organization", "error", err, "workos_org_id", acct.WorkOSOrganizationID, "account_id", acct.ID)
			}
		}

		log.Info("Account deleted", "account_id", acct.ID, "account_name", acct.Name)

		c.JSON(http.StatusOK, gin.H{"message": "account deleted"})
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

// UpdateProfileRequest represents the request body for updating user profile
type UpdateProfileRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// UpdateProfile handles PATCH /api/v1/me (protected)
func UpdateProfile(log *logger.Logger, workos *auth.WorkOSClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		var req UpdateProfileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		updated, err := workos.UpdateUser(c.Request.Context(), user.ID, req.FirstName, req.LastName)
		if err != nil {
			log.Error("Failed to update user profile", "error", err, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
			return
		}

		log.Info("User profile updated", "user_id", user.ID)
		c.JSON(http.StatusOK, UpdateProfileResponse{
			User: &ProfileUser{
				ID:        updated.ID,
				Email:     updated.Email,
				FirstName: updated.FirstName,
				LastName:  updated.LastName,
			},
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
			}

			// Include agent summaries for each account
			agents, err := agentIndex.ListForAccount(a.ID)
			if err == nil {
				summaries := make([]AgentSummary, 0, len(agents))
				for _, agent := range agents {
					summaries = append(summaries, AgentSummary{
						Name:       agent.Name,
						Registry:   agent.Registry,
						Visibility: agent.Visibility,
						BuildCount: len(agent.Versions),
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

		// Validate name format and restrictions
		if err := account.ValidateAccountName(name); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"available": false,
				"reason":    err.Error(),
			})
			return
		}
		if err := account.CheckAccountNameRestricted(name); err != nil {
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

// SearchAccountResult represents a single account in search results
type SearchAccountResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// SearchAccounts handles GET /api/v1/accounts/search (protected)
// Query params: q (required), type (optional: personal|organization), limit (optional, default 10, max 10)
func SearchAccounts(log *logger.Logger, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := strings.ToLower(strings.TrimSpace(c.Query("q")))
		if len(q) < 3 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' must be at least 3 characters"})
			return
		}

		accountType := c.Query("type")
		if accountType != "" && accountType != "personal" && accountType != "organization" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "type must be 'personal' or 'organization'"})
			return
		}

		limit := 10
		if limitStr := c.Query("limit"); limitStr != "" {
			if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 10 {
				limit = parsed
			}
		}

		accounts, err := accountStore.Search(q, accountType, limit)
		if err != nil {
			log.Error("Failed to search accounts", "error", err, "query", q)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search accounts"})
			return
		}

		results := make([]SearchAccountResult, 0, len(accounts))
		for _, a := range accounts {
			results = append(results, SearchAccountResult{
				ID:   a.ID,
				Name: a.Name,
				Type: a.Type,
			})
		}

		c.JSON(http.StatusOK, gin.H{"results": results})
	}
}
