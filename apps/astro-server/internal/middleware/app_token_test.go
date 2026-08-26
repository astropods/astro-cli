package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/appstore"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type stubApps struct {
	app *appstore.App
	err error
}

func (s stubApps) GetByClientID(_ context.Context, _ string) (*appstore.App, error) {
	return s.app, s.err
}

func machineClaims(clientID, orgID string) *auth.JWTClaims {
	return &auth.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  clientID,
			Audience: jwt.ClaimStrings{clientID},
		},
		OrganizationID: orgID,
	}
}

func TestIsMachineTokenDiscriminatesOnAudience(t *testing.T) {
	if !isMachineToken(machineClaims("client_1", "org_1")) {
		t.Fatal("a token naming its own client in aud and sub is a machine token")
	}

	userToken := &auth.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user_01"},
		OrganizationID:   "org_1",
	}
	if isMachineToken(userToken) {
		t.Fatal("a WorkOS user token carries no aud and must not be treated as a machine token")
	}

	thirdParty := &auth.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  "user_01",
			Audience: jwt.ClaimStrings{"some_other_client"},
		},
	}
	if isMachineToken(thirdParty) {
		t.Fatal("an aud that is not the subject is not this shape")
	}
}

func appAuthFixture(t *testing.T, apps MachineAppResolver, claims *auth.JWTClaims) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	mw := &AuthMiddleware{log: logger.New("error", "json"), apps: apps}
	if !mw.authenticateApp(c, claims, "token") {
		return nil
	}
	return c
}

func TestAuthenticateAppSetsAppAndNoUser(t *testing.T) {
	apps := stubApps{app: &appstore.App{
		ID: "app-1", AccountID: "acct-1", ClientID: "client_1",
		Name: "ci", Scopes: []string{"audiences:manage"},
	}}
	c := appAuthFixture(t, apps, machineClaims("client_1", "org_1"))
	if c == nil {
		t.Fatal("expected the machine token to authenticate")
	}

	if _, isUser := GetUser(c); isUser {
		t.Fatal("a machine token must not present a user, or a client ID reads as a person")
	}
	app, ok := GetApp(c)
	if !ok || app.ID != "app-1" || app.AccountID != "acct-1" {
		t.Fatalf("app on context = %+v", app)
	}
	session, ok := GetSession(c)
	if !ok || session.OrganizationID != "org_1" {
		t.Fatalf("session should carry the token's org: %+v", session)
	}
	if len(session.Permissions) != 1 || session.Permissions[0] != "audiences:manage" {
		t.Fatalf("scopes should land in Permissions so one check reads both kinds: %+v", session.Permissions)
	}
}

func TestAuthenticateAppRejectsUnknownClient(t *testing.T) {
	if c := appAuthFixture(t, stubApps{}, machineClaims("client_gone", "org_1")); c != nil {
		t.Fatal("a client with no row must be denied, so deleting an app revokes before expiry")
	}
}

func TestAuthenticateAppRejectsOnLookupFailure(t *testing.T) {
	apps := stubApps{err: errors.New("db down")}
	if c := appAuthFixture(t, apps, machineClaims("client_1", "org_1")); c != nil {
		t.Fatal("a lookup failure must fail closed")
	}
}

func TestAuthenticateAppRejectsWithNoResolver(t *testing.T) {
	if c := appAuthFixture(t, nil, machineClaims("client_1", "org_1")); c != nil {
		t.Fatal("no resolver configured must fail closed")
	}
}

func appScopeRouter(t *testing.T, app *auth.App, acct *account.Account, required string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if app != nil {
			c.Set(string(auth.AppContextKey), app)
		}
		c.Set(string(auth.AccountContextKey), acct)
		c.Next()
	})
	router.GET("/", RequireAccountPermission(nil, required), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	return response
}

func TestAppScopeSatisfiesPermissionCheck(t *testing.T) {
	app := &auth.App{ID: "app-1", AccountID: "acct-1", Scopes: []string{"audiences:manage"}}
	acct := &account.Account{ID: "acct-1", Type: "organization", WorkOSOrganizationID: "org_1"}

	if got := appScopeRouter(t, app, acct, "audiences:manage").Code; got != http.StatusOK {
		t.Fatalf("status=%d, want 200", got)
	}
}

func TestAppWithoutScopeIsForbidden(t *testing.T) {
	app := &auth.App{ID: "app-1", AccountID: "acct-1", Scopes: []string{"audiences:read"}}
	acct := &account.Account{ID: "acct-1", Type: "organization", WorkOSOrganizationID: "org_1"}

	if got := appScopeRouter(t, app, acct, "audiences:manage").Code; got != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", got)
	}
}

func TestAppOnAnotherAccountsPathIsForbidden(t *testing.T) {
	app := &auth.App{ID: "app-1", AccountID: "acct-1", Scopes: []string{"audiences:manage"}}
	other := &account.Account{ID: "acct-2", Type: "organization", WorkOSOrganizationID: "org_2"}

	if got := appScopeRouter(t, app, other, "audiences:manage").Code; got != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", got)
	}
}

func TestAppNeverSatisfiesAHumanRolePermission(t *testing.T) {
	app := &auth.App{ID: "app-1", AccountID: "acct-1", Scopes: []string{"audiences:manage"}}
	acct := &account.Account{ID: "acct-1", Type: "organization", WorkOSOrganizationID: "org_1"}

	if got := appScopeRouter(t, app, acct, "org:manage").Code; got != http.StatusForbidden {
		t.Fatalf("status=%d, want 403: machine scopes and human roles are separate vocabularies", got)
	}
}

func TestAppIsRefusedByMembershipRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.AppContextKey), &auth.App{ID: "app-1", AccountID: "acct-1"})
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Type: "organization"})
		c.Next()
	})
	router.GET("/", RequireAccountMember(nil), func(c *gin.Context) { c.Status(http.StatusOK) })

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403: membership must never stand in for a scope", response.Code)
	}
}
