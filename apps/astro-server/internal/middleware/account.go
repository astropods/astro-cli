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

// RequireAccountRole checks that the authenticated user has one of the required roles
// in the resolved account. Must be used after ResolveAccount and RequireAuth.
func RequireAccountRole(accountStore *account.AccountStore, roles ...string) gin.HandlerFunc {
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

		hasRole, err := accountStore.HasRole(acct.ID, user.ID, roles...)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "failed to check account role",
			})
			return
		}

		if !hasRole {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "insufficient permissions for this account",
			})
			return
		}

		c.Next()
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
