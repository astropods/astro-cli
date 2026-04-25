package handlers

import (
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/authorizationstore"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

var validAuthRoles = map[string]bool{"viewer": true, "admin": true, "none": true}

// CheckDeploymentAuthorization is called by messaging containers to check whether an identity
// is allowed to access a deployment.
//
// The deployment ID comes from the signed deploy token (set by RequireDeployToken middleware).
// Identity resolution:
//   - identity_type=user: WorkOS user ID resolved to account(s) via account_members
//   - identity_type=slack: resolved to the deployment owner's account (from the deploy token)
func CheckDeploymentAuthorization(log *logger.Logger, authStore *authorizationstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		deploymentID := middleware.DeploymentIDFromContext(c)
		accountID := middleware.AccountIDFromContext(c)
		identityType := c.Query("identity_type")
		identityID := c.Query("identity_id")
		adapter := c.Query("adapter")

		if identityType == "" || identityID == "" || adapter == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "identity_type, identity_id, and adapter are required"})
			return
		}
		if adapter != authorizationstore.AdapterWeb && adapter != authorizationstore.AdapterSlack {
			c.JSON(http.StatusBadRequest, gin.H{"error": "adapter must be one of: web, slack"})
			return
		}

		var allowed bool
		var err error

		switch identityType {
		case authorizationstore.IdentityTypeUser:
			accountIDs, err := authStore.AccountIDsForUser(identityID)
			if err != nil {
				log.Error("failed to resolve user accounts", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "authorization check failed"})
				return
			}
			for _, aid := range accountIDs {
				if ok, err := authStore.IsAllowedByAccount(deploymentID, aid, adapter); err != nil {
					log.Error("authorization check failed", "deployment_id", deploymentID, "error", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "authorization check failed"})
					return
				} else if ok {
					allowed = true
					break
				}
			}

		case authorizationstore.IdentityTypeSlack:
			// Slack users resolve to the deployment owner's account (Slack bot is per-account)
			allowed, err = authStore.IsAllowedByAccount(deploymentID, accountID, adapter)
			if err != nil {
				log.Error("authorization check failed", "deployment_id", deploymentID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "authorization check failed"})
				return
			}

		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown identity_type"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"allowed": allowed})
	}
}

// GetDeploymentAuthorization returns the access policy and grants for a deployment.
func GetDeploymentAuthorization(log *logger.Logger, authStore *authorizationstore.Store, deployStore *deploymentstore.Store, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		policy, err := authStore.GetPolicy(dep.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get policy"})
			return
		}

		grants, err := authStore.ListGrants(dep.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list grants"})
			return
		}

		defaultRole := "none"
		if policy != nil {
			defaultRole = policy.DefaultRole
		}

		c.JSON(http.StatusOK, gin.H{
			"default_role": defaultRole,
			"grants":       grants,
		})
	}
}

// SetDeploymentPolicy sets the default_role for a deployment's access policy.
func SetDeploymentPolicy(log *logger.Logger, authStore *authorizationstore.Store, deployStore *deploymentstore.Store, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		var body struct {
			DefaultRole string `json:"default_role" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if !validAuthRoles[body.DefaultRole] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "default_role must be one of: viewer, admin, none"})
			return
		}

		if err := authStore.SetPolicy(dep.ID, body.DefaultRole); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set policy"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// UpsertDeploymentGrant adds or updates an account's authorization grant on a deployment.
func UpsertDeploymentGrant(log *logger.Logger, authStore *authorizationstore.Store, deployStore *deploymentstore.Store, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		var body struct {
			AccountID string `json:"account_id" binding:"required"`
			Adapter   string `json:"adapter" binding:"required"`
			Role      string `json:"role" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if !validAuthRoles[body.Role] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "role must be one of: viewer, admin, none"})
			return
		}
		if body.Adapter != authorizationstore.AdapterWeb && body.Adapter != authorizationstore.AdapterSlack {
			c.JSON(http.StatusBadRequest, gin.H{"error": "adapter must be one of: web, slack"})
			return
		}

		if err := authStore.UpsertGrant(dep.ID, body.AccountID, body.Adapter, body.Role); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upsert grant"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// DeleteDeploymentGrant removes an account's authorization grant from a deployment.
func DeleteDeploymentGrant(log *logger.Logger, authStore *authorizationstore.Store, deployStore *deploymentstore.Store, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		grantAccountID := c.Param("account_id")
		adapter := c.Param("adapter")
		if err := authStore.DeleteGrant(dep.ID, grantAccountID, adapter); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete grant"})
			return
		}

		c.Status(http.StatusNoContent)
	}
}
