package handlers

import (
	"context"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
)

// insightsElevatedPermission is what "may see every developer's usage" means on
// the Insights surfaces.
//
// org:manage rather than org:admin: WorkOS grants org:admin to owner only, so
// gating on it silently restricted every org admin to their own row.
const insightsElevatedPermission = "org:manage"

// orgRoleLookup resolves a user's role per WorkOS organization. Satisfied by
// *org.Sync; nil disables the cross-organization fallback below.
type orgRoleLookup interface {
	GetMembershipRoles(ctx context.Context, userID string) map[string]string
}

// orgRoleTTL bounds how stale a cached role may be. The underlying lookup is a
// WorkOS API call, and this one sits on a page endpoint the client re-polls, so
// uncached it is a third-party round trip per reader per poll per tab.
const orgRoleTTL = time.Minute

// CachedOrgRoles memoises role lookups per user for orgRoleTTL. A role change
// takes effect within that window, which is the same order of staleness as the
// session claim the lookup stands in for.
type CachedOrgRoles struct {
	inner  orgRoleLookup
	mu     sync.Mutex
	byUser map[string]cachedRoles
}

type cachedRoles struct {
	roles     map[string]string
	expiresAt time.Time
}

func NewCachedOrgRoles(inner orgRoleLookup) *CachedOrgRoles {
	return &CachedOrgRoles{inner: inner, byUser: map[string]cachedRoles{}}
}

func (c *CachedOrgRoles) GetMembershipRoles(ctx context.Context, userID string) map[string]string {
	if c == nil || c.inner == nil {
		return nil
	}
	now := time.Now()

	c.mu.Lock()
	hit, ok := c.byUser[userID]
	c.mu.Unlock()
	if ok && now.Before(hit.expiresAt) {
		return hit.roles
	}

	roles := c.inner.GetMembershipRoles(ctx, userID)
	if roles == nil {
		// A failed lookup is not cached: caching it would extend a transient
		// WorkOS outage into a minute of demoting owners to their own row.
		return nil
	}

	c.mu.Lock()
	// Expiry is only observed on read, so sweep while the lock is held or a
	// user who never returns keeps their entry for the life of the process.
	for id, entry := range c.byUser {
		if now.After(entry.expiresAt) {
			delete(c.byUser, id)
		}
	}
	c.byUser[userID] = cachedRoles{roles: roles, expiresAt: now.Add(orgRoleTTL)}
	c.mu.Unlock()
	return roles
}

// elevatedRoles may see other people's usage. Mirrors the roles the client
// treats as administrative.
var elevatedRoles = map[string]bool{"owner": true, "admin": true}

// insightsSeesEveryone reports whether the caller may see per-developer usage
// for this account, rather than only their own.
//
// The session's permissions are authoritative when it is scoped to this
// account's organization. It often is not: the Insights account switcher moves
// the ?account= param without performing a WorkOS org switch, so a caller
// viewing one of their other organizations carries a session scoped elsewhere
// and reads as unprivileged — including an owner looking at their own org.
//
// The fallback asks WorkOS for the caller's role in that organization, so the
// answer stops depending on which organization the session is pointed at.
//
// It matches on role slug, not on the permission itself, so it assumes the
// default role-to-permission mapping. An organization that removes org:manage
// from its admin role would still be treated as elevated here while an in-org
// session correctly is not. Resolving the role's permissions would close that.
func insightsSeesEveryone(
	c *gin.Context,
	accountStore *account.AccountStore,
	roles orgRoleLookup,
	acct *account.Account,
	user *auth.User,
) bool {
	if middleware.HasAccountPermission(c, accountStore, acct, user, insightsElevatedPermission) {
		return true
	}
	if roles == nil || acct.WorkOSOrganizationID == "" {
		return false
	}
	// Only when the session is scoped elsewhere. A session on this organization
	// already answered above, and asking again would let a role outrank a
	// permission that was deliberately withheld.
	if session, ok := middleware.GetSession(c); ok && session.OrganizationID == acct.WorkOSOrganizationID {
		return false
	}
	byOrg := roles.GetMembershipRoles(c.Request.Context(), user.ID)
	if byOrg == nil {
		// The lookup reports failure as an absent map, which is indistinguishable
		// from "no memberships" and quietly restricts an owner to their own row.
		return false
	}
	return elevatedRoles[byOrg[acct.WorkOSOrganizationID]]
}
