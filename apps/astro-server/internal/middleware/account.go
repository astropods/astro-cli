package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/auth"
)

// ResolveAccount reads the :account URL param, looks up the account by name,
// and sets it in the request context.
func ResolveAccount(accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")
		if accountName == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "account name is required",
			})
			return
		}

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error": "account not found",
			})
			return
		}

		c.Set(string(auth.AccountContextKey), acct)
		c.Next()
	}
}

// RequireAccountPermission checks authorization based on account type:
//   - Personal account: owner has all permissions implicitly
//   - Organization account: JWT must be scoped to the target org via
//     switch-org; permissions are read from the JWT permissions claim
//
// Must be used after ResolveAccount and RequireAuth.
func RequireAccountPermission(accountStore *account.AccountStore, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := GetUser(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			return
		}

		acct, ok := GetAccountFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "account not resolved",
			})
			return
		}

		if acct.Type == "personal" {
			// Personal accounts: owner has all permissions
			hasRole, err := accountStore.HasRole(acct.ID, user.ID, "owner")
			if err != nil || !hasRole {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "insufficient permissions for this account",
				})
				return
			}
			c.Next()
			return
		}

		// Organization account: JWT must be scoped to this org
		session, sessionOk := GetSession(c)
		if !sessionOk || acct.WorkOSOrganizationID == "" || session.OrganizationID != acct.WorkOSOrganizationID {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "session is not scoped to this organization, use switch-org first",
			})
			return
		}

		// Check permissions from JWT
		for _, p := range session.Permissions {
			if p == permission {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "insufficient permissions for this account",
		})
	}
}

// GetAccountFromContext retrieves the resolved account from the gin context
func GetAccountFromContext(c *gin.Context) (*account.Account, bool) {
	val, exists := c.Get(string(auth.AccountContextKey))
	if !exists {
		return nil, false
	}
	acct, ok := val.(*account.Account)
	return acct, ok
}
