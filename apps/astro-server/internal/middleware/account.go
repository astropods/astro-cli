package middleware

import (
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/gin-gonic/gin"
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

// HasAccountPermission checks whether a user holds a given permission on the
// resolved account. It mirrors the logic of RequireAccountPermission but
// returns a bool instead of aborting the request, so handlers can branch on it.
func HasAccountPermission(c *gin.Context, accountStore *account.AccountStore, acct *account.Account, user *auth.User, permission string) bool {
	if acct.Type == "personal" {
		isMember, err := accountStore.IsMember(acct.ID, user.ID)
		return err == nil && isMember
	}
	session, ok := GetSession(c)
	if !ok || acct.WorkOSOrganizationID == "" || session.OrganizationID != acct.WorkOSOrganizationID {
		return false
	}
	for _, p := range session.Permissions {
		if p == permission {
			return true
		}
	}
	return false
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
			// Personal accounts: only one member (the creator) who has all permissions
			isMember, err := accountStore.IsMember(acct.ID, user.ID)
			if err != nil || !isMember {
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

// RequireAccountMember checks that the authenticated user is a member of the
// resolved account. Must be used after ResolveAccount and RequireAuth.
func RequireAccountMember(accountStore *account.AccountStore) gin.HandlerFunc {
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

		isMember, err := accountStore.IsMember(acct.ID, user.ID)
		if err != nil || !isMember {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "you are not a member of this account",
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
