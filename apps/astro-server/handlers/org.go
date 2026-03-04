package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"github.com/postman/astro/apps/astro-server/internal/middleware"
	"github.com/postman/astro/apps/astro-server/internal/org"
)

// requireOwnerForOwnerRole guards against non-owners assigning the owner role.
// For org accounts, checks session.Role from JWT. For personal accounts (no WorkOS org),
// any member is owner so always allows.
// Returns true if the request should continue, false if it was aborted with 403.
func requireOwnerForOwnerRole(c *gin.Context, requestedRole string) bool {
	if requestedRole != "owner" {
		return true
	}
	session, ok := middleware.GetSession(c)
	if !ok || session.Role != "owner" {
		c.JSON(http.StatusForbidden, gin.H{"error": "only owners can assign the owner role"})
		return false
	}
	return true
}

// --- Member Management ---

// AddMemberRequest represents the request to add a member to an account.
type AddMemberRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Role   string `json:"role" binding:"required"`
}

// ChangeMemberRoleRequest represents the request to change a member's role.
type ChangeMemberRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// ListMembers handles GET /api/v1/accounts/:account/members
func ListMembers(log *logger.Logger, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		members, err := accountStore.GetMembersForAccount(acct.ID)
		if err != nil {
			log.Error("Failed to list members", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list members"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"members": members})
	}
}

// AddMember handles POST /api/v1/accounts/:account/members
func AddMember(log *logger.Logger, syncSvc *org.Sync, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		var req AddMemberRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		if !requireOwnerForOwnerRole(c, req.Role) {
			return
		}

		member, err := syncSvc.AddMember(c.Request.Context(), acct.ID, req.UserID, req.Role)
		if err != nil {
			log.Error("Failed to add member", "error", err, "account_id", acct.ID, "user_id", req.UserID)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to add member",
				"details": err.Error(),
			})
			return
		}

		log.Info("Member added", "account_id", acct.ID, "user_id", req.UserID, "role", req.Role)
		c.JSON(http.StatusCreated, gin.H{"member": member})
	}
}

// UpdateMemberRole handles PUT /api/v1/accounts/:account/members/:user_id
func UpdateMemberRole(log *logger.Logger, syncSvc *org.Sync, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		userID := c.Param("user_id")

		var req ChangeMemberRoleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		if !requireOwnerForOwnerRole(c, req.Role) {
			return
		}

		if err := syncSvc.ChangeMemberRole(c.Request.Context(), acct.ID, userID, req.Role); err != nil {
			log.Error("Failed to update member role", "error", err, "account_id", acct.ID, "user_id", userID)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to update member role",
				"details": err.Error(),
			})
			return
		}

		log.Info("Member role updated", "account_id", acct.ID, "user_id", userID, "role", req.Role)
		c.JSON(http.StatusOK, gin.H{"message": "role updated"})
	}
}

// RemoveMember handles DELETE /api/v1/accounts/:account/members/:user_id
func RemoveMember(log *logger.Logger, syncSvc *org.Sync) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		userID := c.Param("user_id")

		if err := syncSvc.RemoveMember(c.Request.Context(), acct.ID, userID); err != nil {
			log.Error("Failed to remove member", "error", err, "account_id", acct.ID, "user_id", userID)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to remove member",
				"details": err.Error(),
			})
			return
		}

		log.Info("Member removed", "account_id", acct.ID, "user_id", userID)
		c.JSON(http.StatusOK, gin.H{"message": "member removed"})
	}
}

// --- Invitation Management ---

// CreateInvitationRequest represents the request to send an invitation.
type CreateInvitationRequest struct {
	Email    string `json:"email" binding:"required"`
	RoleSlug string `json:"role" binding:"required"`
}

// CreateInvitation handles POST /api/v1/accounts/:account/invitations
func CreateInvitation(log *logger.Logger, orgClient *org.Client, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		if acct.WorkOSOrganizationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invitations are only supported for organization accounts"})
			return
		}

		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		var req CreateInvitationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		if !requireOwnerForOwnerRole(c, req.RoleSlug) {
			return
		}

		inv, err := orgClient.SendInvitation(c.Request.Context(), acct.WorkOSOrganizationID, req.Email, user.ID, req.RoleSlug)
		if err != nil {
			log.Error("Failed to send invitation", "error", err, "account_id", acct.ID, "email", req.Email)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to send invitation",
				"details": err.Error(),
			})
			return
		}

		log.Info("Invitation sent", "account_id", acct.ID, "email", req.Email)
		c.JSON(http.StatusCreated, gin.H{"invitation": inv})
	}
}

// ListAccountInvitations handles GET /api/v1/accounts/:account/invitations
func ListAccountInvitations(log *logger.Logger, orgClient *org.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		if acct.WorkOSOrganizationID == "" {
			c.JSON(http.StatusOK, gin.H{"invitations": []any{}})
			return
		}

		invitations, err := orgClient.ListInvitations(c.Request.Context(), acct.WorkOSOrganizationID)
		if err != nil {
			log.Error("Failed to list invitations", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list invitations"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"invitations": invitations})
	}
}

// RevokeInvitation handles DELETE /api/v1/accounts/:account/invitations/:id
func RevokeInvitation(log *logger.Logger, orgClient *org.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		invitationID := c.Param("id")

		// Verify the invitation belongs to this account's WorkOS org to prevent IDOR
		inv, err := orgClient.GetInvitation(c.Request.Context(), invitationID)
		if err != nil {
			log.Error("Failed to get invitation", "error", err, "invitation_id", invitationID)
			c.JSON(http.StatusNotFound, gin.H{"error": "invitation not found"})
			return
		}
		if inv.OrganizationID != acct.WorkOSOrganizationID {
			c.JSON(http.StatusNotFound, gin.H{"error": "invitation not found"})
			return
		}

		if err := orgClient.RevokeInvitation(c.Request.Context(), invitationID); err != nil {
			log.Error("Failed to revoke invitation", "error", err, "invitation_id", invitationID)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to revoke invitation",
				"details": err.Error(),
			})
			return
		}

		log.Info("Invitation revoked", "invitation_id", invitationID, "account_id", acct.ID)
		c.JSON(http.StatusOK, gin.H{"message": "invitation revoked"})
	}
}
