package handlers

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
)

const testOrgID = "org_test"

type stubRoles struct {
	roles  map[string]string
	called bool
	calls  int
}

func (s *stubRoles) GetMembershipRoles(_ context.Context, _ string) map[string]string {
	s.called = true
	s.calls++
	return s.roles
}

// visibilityCtx builds a request whose session carries the given permissions and
// is scoped to sessionOrg.
func visibilityCtx(sessionOrg string, permissions ...string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set(string(auth.SessionContextKey), &auth.Session{
		OrganizationID: sessionOrg,
		Permissions:    permissions,
	})
	return c
}

func orgAccount() *account.Account {
	return &account.Account{ID: "acct-1", Type: "organization", WorkOSOrganizationID: testOrgID}
}

// WorkOS grants org:admin to owner only, so gating on it restricted every org
// admin to their own row. org:manage is what both roles carry.
func TestInsightsSeesEveryone_AdminPermission(t *testing.T) {
	cases := map[string]struct {
		permissions []string
		want        bool
	}{
		"admin and owner both carry org:manage": {[]string{"org:manage"}, true},
		"owner-only org:admin is not required":  {[]string{"org:manage", "org:admin"}, true},
		"a plain member sees only themselves":   {nil, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := visibilityCtx(testOrgID, tc.permissions...)
			got := insightsSeesEveryone(c, nil, nil, orgAccount(), &auth.User{ID: "user_1"})
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// The account switcher moves ?account= without a WorkOS org switch, so a caller
// viewing another of their organizations carries a session scoped elsewhere.
// Reading permissions from it reports them as unprivileged — including an owner
// looking at their own organization.
func TestInsightsSeesEveryone_SessionScopedToAnotherOrg(t *testing.T) {
	elsewhere := visibilityCtx("org_somewhere_else")

	if insightsSeesEveryone(elsewhere, nil, nil, orgAccount(), &auth.User{ID: "user_1"}) {
		t.Fatal("without a role lookup there is nothing to fall back to")
	}

	for role, want := range map[string]bool{"owner": true, "admin": true, "member": false} {
		t.Run(role, func(t *testing.T) {
			roles := &stubRoles{roles: map[string]string{testOrgID: role}}
			got := insightsSeesEveryone(visibilityCtx("org_somewhere_else"), nil, roles,
				orgAccount(), &auth.User{ID: "user_1"})
			if got != want {
				t.Errorf("%s = %v, want %v", role, got, want)
			}
			if !roles.called {
				t.Error("expected the role lookup to be consulted")
			}
		})
	}
}

// A session already on this organization has an authoritative answer. Falling
// back would let a role outrank a permission deliberately withheld from it.
func TestInsightsSeesEveryone_NoFallbackWhenSessionMatches(t *testing.T) {
	roles := &stubRoles{roles: map[string]string{testOrgID: "owner"}}
	c := visibilityCtx(testOrgID) // scoped here, but carrying no permissions

	if insightsSeesEveryone(c, nil, roles, orgAccount(), &auth.User{ID: "user_1"}) {
		t.Error("a session on this org must be trusted as-is")
	}
	if roles.called {
		t.Error("the role lookup must not run when the session is scoped here")
	}
}

// The lookup is a WorkOS call on a polled endpoint, so a second read inside the
// window must not reach it.
func TestCachedOrgRoles_ServesWithinTTL(t *testing.T) {
	inner := &stubRoles{roles: map[string]string{testOrgID: "owner"}}
	cache := NewCachedOrgRoles(inner)

	first := cache.GetMembershipRoles(context.Background(), "user_1")
	second := cache.GetMembershipRoles(context.Background(), "user_1")

	if first[testOrgID] != "owner" || second[testOrgID] != "owner" {
		t.Fatalf("got %v then %v, want owner both times", first, second)
	}
	if inner.calls != 1 {
		t.Errorf("inner called %d times, want 1", inner.calls)
	}
}

// Caching a failure would stretch a transient outage into a minute of demoting
// owners to their own row.
func TestCachedOrgRoles_DoesNotCacheFailure(t *testing.T) {
	inner := &stubRoles{roles: nil}
	cache := NewCachedOrgRoles(inner)

	cache.GetMembershipRoles(context.Background(), "user_1")
	cache.GetMembershipRoles(context.Background(), "user_1")

	if inner.calls != 2 {
		t.Errorf("inner called %d times, want 2 — a failed lookup must be retried", inner.calls)
	}
}
