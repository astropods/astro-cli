package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/gin-gonic/gin"
)

func TestSubjectFromContext_noUser(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	_, ok := SubjectFromContext(c)
	if ok {
		t.Fatal("SubjectFromContext() ok = true, want false")
	}
}

func TestSubjectFromContext_userOnly(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(string(auth.UserContextKey), &auth.User{ID: "user-from-context"})

	sub, ok := SubjectFromContext(c)
	if !ok {
		t.Fatal("SubjectFromContext() ok = false, want true")
	}
	if sub.UserID != "user-from-context" {
		t.Fatalf("UserID = %q, want %q", sub.UserID, "user-from-context")
	}
	if sub.OrgID != "" {
		t.Fatalf("OrgID = %q, want empty", sub.OrgID)
	}
	if sub.MembershipID != "" {
		t.Fatalf("MembershipID = %q, want empty", sub.MembershipID)
	}
}

func TestSubjectFromContext_userAndSession(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(string(auth.UserContextKey), &auth.User{ID: "user-from-context"})
	c.Set(string(auth.SessionContextKey), &auth.Session{
		UserID:             "user-from-session",
		OrganizationID:     "org_01HXYZ",
		WorkOSMembershipID: "om_01KRC3M3HC3T700J1SZ173FWHH",
	})

	sub, ok := SubjectFromContext(c)
	if !ok {
		t.Fatal("SubjectFromContext() ok = false, want true")
	}
	if sub.UserID != "user-from-session" {
		t.Fatalf("UserID = %q, want session UserID %q", sub.UserID, "user-from-session")
	}
	if sub.OrgID != "org_01HXYZ" {
		t.Fatalf("OrgID = %q, want %q", sub.OrgID, "org_01HXYZ")
	}
	if sub.MembershipID != "om_01KRC3M3HC3T700J1SZ173FWHH" {
		t.Fatalf("MembershipID = %q, want om_01KRC3M3HC3T700J1SZ173FWHH", sub.MembershipID)
	}
}

func TestSubjectFromContext_prefersSessionUserIDOnMismatch(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(string(auth.UserContextKey), &auth.User{ID: "stale-user-id"})
	c.Set(string(auth.SessionContextKey), &auth.Session{
		UserID:         "jwt-subject",
		OrganizationID: "org_01HXYZ",
	})

	sub, ok := SubjectFromContext(c)
	if !ok {
		t.Fatal("SubjectFromContext() ok = false, want true")
	}
	if sub.UserID != "jwt-subject" {
		t.Fatalf("UserID = %q, want jwt subject %q", sub.UserID, "jwt-subject")
	}
}

func TestSubjectFromContext_orgIDIsCallerJWTScopeNotResourceAccountOrg(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
	c.Set(string(auth.SessionContextKey), &auth.Session{
		UserID:         "user-1",
		OrganizationID: "org_session_scope",
	})
	// If SubjectFromContext incorrectly read the resolved account's WorkOS org from
	// context, OrgID would be org_resource — it must stay on session scope instead.
	c.Set(string(auth.AccountContextKey), &account.Account{
		Type:                 "organization",
		WorkOSOrganizationID: "org_resource",
	})

	sub, ok := SubjectFromContext(c)
	if !ok {
		t.Fatal("SubjectFromContext() ok = false, want true")
	}
	if sub.OrgID != "org_session_scope" {
		t.Fatalf("OrgID = %q, want JWT session scope %q (not resource account org)", sub.OrgID, "org_session_scope")
	}
	if sub.OrgID == "org_resource" {
		t.Fatal("OrgID must not be taken from resolved account in gin context")
	}
}

func TestSubjectFromContext_emptySessionUserIDFallsBackToUserID(t *testing.T) {
	t.Parallel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(string(auth.UserContextKey), &auth.User{ID: "user-from-context"})
	c.Set(string(auth.SessionContextKey), &auth.Session{
		UserID:         "",
		OrganizationID: "org_01HXYZ",
	})

	sub, ok := SubjectFromContext(c)
	if !ok {
		t.Fatal("SubjectFromContext() ok = false, want true")
	}
	if sub.UserID != "user-from-context" {
		t.Fatalf("UserID = %q, want fallback to user.ID %q", sub.UserID, "user-from-context")
	}
}
