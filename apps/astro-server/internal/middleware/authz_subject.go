package middleware

import (
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/gin-gonic/gin"
)

// SubjectFromContext builds an authz.Subject from the authenticated request context.
func SubjectFromContext(c *gin.Context) (authz.Subject, bool) {
	user, ok := GetUser(c)
	if !ok {
		return authz.Subject{}, false
	}

	sub := authz.Subject{UserID: user.ID}
	if session, ok := GetSession(c); ok {
		if session.UserID != "" {
			sub.UserID = session.UserID
		}
		sub.OrgID = session.OrganizationID
		sub.MembershipID = session.WorkOSMembershipID
	}
	return sub, true
}
