package middleware

import (
	"net/http"
	"slices"

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

// GetApp returns the authenticated machine app, when the caller is one.
func GetApp(c *gin.Context) (*auth.App, bool) {
	value, exists := c.Get(string(auth.AppContextKey))
	if !exists {
		return nil, false
	}
	app, ok := value.(*auth.App)
	return app, ok
}

// appHoldsScope answers a permission check for a machine caller. An app is
// bound to one account, so the account is compared directly rather than through
// a membership row, and the scope is matched against the app's own vocabulary.
func appHoldsScope(app *auth.App, acct *account.Account, scope string) bool {
	if app.AccountID != acct.ID {
		return false
	}
	return slices.Contains(app.Scopes, scope)
}

func sessionScopedToAccount(c *gin.Context, acct *account.Account) (*auth.Session, bool) {
	session, ok := GetSession(c)
	if !ok || session == nil || acct.WorkOSOrganizationID == "" {
		return nil, false
	}
	return session, session.OrganizationID == acct.WorkOSOrganizationID
}

// HasAccountPermission checks whether a user holds a given permission on the
// resolved account. It mirrors the logic of RequireAccountPermission but
// returns a bool instead of aborting the request, so handlers can branch on it.
func HasAccountPermission(c *gin.Context, accountStore *account.AccountStore, acct *account.Account, user *auth.User, permission string) bool {
	if session, scoped := sessionScopedToAccount(c, acct); scoped {
		if slices.Contains(session.Permissions, permission) {
			return true
		}
	}
	// Remove this fallback once the web app scopes its session at login and the
	// CLI mints an org-scoped token for personal accounts too (cmd/account.go).
	// Check first that the WorkOS owner role carries every permission a
	// personal-account route asks for, or an owner loses their own account.
	if acct.Type != "personal" {
		return false
	}
	isMember, err := accountStore.IsMember(acct.ID, user.ID)
	return err == nil && isMember
}

// RequireAccountPermission authorizes a caller against the resolved account:
//   - Session scoped to the account's organization: the JWT permission claims
//     decide, for a personal account as much as an organization one
//   - Personal account with an unscoped session: membership decides, since the
//     account has one member
//   - Organization account with an unscoped session: refused, use switch-org
//
// Must be used after ResolveAccount and RequireAuth.
func RequireAccountPermission(accountStore *account.AccountStore, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if app, isApp := GetApp(c); isApp {
			acct, ok := GetAccountFromContext(c)
			if !ok {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "account not resolved",
				})
				return
			}
			if !appHoldsScope(app, acct, permission) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "this app does not hold the " + permission + " scope on this account",
				})
				return
			}
			c.Next()
			return
		}
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

		if HasAccountPermission(c, accountStore, acct, user, permission) {
			c.Next()
			return
		}

		if _, scoped := sessionScopedToAccount(c, acct); !scoped && acct.Type != "personal" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "session is not scoped to this organization, use switch-org first",
			})
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "insufficient permissions for this account",
		})
	}
}

// RequireAccountOwner checks that the caller is the account's recorded owner.
// Ownership lives in accounts.owner_user_id, not in the WorkOS role claim, so
// an account whose owner is unrecorded has no one who passes this check.
// Must be used after ResolveAccount and RequireAuth.
func RequireAccountOwner(accountStore *account.AccountStore) gin.HandlerFunc {
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

		owner, err := accountStore.OwnerUserID(acct.ID)
		if err != nil || owner == "" || owner != user.ID {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "only the account owner can perform this action",
			})
			return
		}

		c.Next()
	}
}

// RequireAccountMember checks that the authenticated user is a member of the
// resolved account. Must be used after ResolveAccount and RequireAuth.
func RequireAccountMember(accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		// A machine app is not a member of anything. Routes it should reach
		// declare a scope through RequireAccountPermission instead, so
		// membership never stands in for authorization.
		if _, isApp := GetApp(c); isApp {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "this endpoint requires a signed-in member",
			})
			return
		}
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
