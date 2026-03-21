package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// TestRoutePermissionWiring verifies that each route group is wired with the
// correct permission middleware, matching the setup in main.go's setupRoutes.
// Uses the JWT path with session scoped to the target org.
func TestRoutePermissionWiring(t *testing.T) {
	ok := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		permissions []string
		wantCode    int
	}{
		// Member routes require org:manage — member permissions denied
		{"member_GET_members_denied", "GET", "/api/v1/accounts/myorg/members", "",
			[]string{"agents:read", "deployments:write"}, http.StatusForbidden},
		// Member routes require org:manage — admin permissions allowed
		{"admin_GET_members_allowed", "GET", "/api/v1/accounts/myorg/members", "",
			[]string{"agents:read", "agents:write", "deployments:write", "org:manage"}, http.StatusOK},

		// Invitation routes require org:manage — member permissions denied
		{"member_GET_invitations_denied", "GET", "/api/v1/accounts/myorg/invitations", "",
			[]string{"agents:read", "deployments:write"}, http.StatusForbidden},
		// Invitation routes require org:manage — admin permissions allowed
		{"admin_GET_invitations_allowed", "GET", "/api/v1/accounts/myorg/invitations", "",
			[]string{"agents:read", "agents:write", "deployments:write", "org:manage"}, http.StatusOK},

		// Agent write routes require agents:write — member permissions denied
		{"member_PUT_visibility_denied", "PUT", "/api/v1/agents/myorg/test-agent/visibility", `{}`,
			[]string{"agents:read", "deployments:write"}, http.StatusForbidden},
		// Agent write routes require agents:write — admin permissions allowed
		{"admin_PUT_visibility_allowed", "PUT", "/api/v1/agents/myorg/test-agent/visibility", `{}`,
			[]string{"agents:read", "agents:write", "deployments:write", "org:manage"}, http.StatusOK},

		// Account admin routes require org:admin — admin permissions denied
		{"admin_PUT_rename_denied", "PUT", "/api/v1/accounts/myorg", `{}`,
			[]string{"agents:read", "agents:write", "deployments:write", "org:manage"}, http.StatusForbidden},
		// Account admin routes require org:admin — owner permissions allowed
		{"owner_PUT_rename_allowed", "PUT", "/api/v1/accounts/myorg", `{}`,
			[]string{"agents:read", "agents:write", "deployments:write", "org:manage", "org:admin"}, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			store := account.NewAccountStore(db)

			// ResolveAccount: return org account
			mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
				WithArgs("myorg").
				WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "avatar_version"}).
					AddRow("acct-1", "myorg", "organization", "org_123", nil, time.Now(), time.Now(), 0))

			router := gin.New()
			// Inject authenticated user with JWT scoped to the target org
			router.Use(func(c *gin.Context) {
				c.Set(string(auth.UserContextKey), &auth.User{ID: "caller-1"})
				c.Set(string(auth.SessionContextKey), &auth.Session{
					OrganizationID: "org_123",
					Permissions:    tt.permissions,
				})
				c.Next()
			})

			// Wire route groups exactly as main.go does
			v1 := router.Group("/api/v1")

			accountAdmin := v1.Group("/accounts/:account")
			accountAdmin.Use(middleware.ResolveAccount(store))
			accountAdmin.Use(middleware.RequireAccountPermission(store, "org:admin"))
			accountAdmin.PUT("", ok)

			memberRoutes := v1.Group("/accounts/:account/members")
			memberRoutes.Use(middleware.ResolveAccount(store))
			memberRoutes.Use(middleware.RequireAccountPermission(store, "org:manage"))
			memberRoutes.GET("", ok)

			invitationRoutes := v1.Group("/accounts/:account/invitations")
			invitationRoutes.Use(middleware.ResolveAccount(store))
			invitationRoutes.Use(middleware.RequireAccountPermission(store, "org:manage"))
			invitationRoutes.GET("", ok)

			agentWriteRoutes := v1.Group("/agents/:account/:name")
			agentWriteRoutes.Use(middleware.ResolveAccount(store))
			agentWriteRoutes.Use(middleware.RequireAccountPermission(store, "agents:write"))
			agentWriteRoutes.PUT("/visibility", ok)

			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}
			var req *http.Request
			if body != nil {
				req = httptest.NewRequest(tt.method, tt.path, body)
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("expected %d, got %d: %s", tt.wantCode, rec.Code, rec.Body.String())
			}
		})
	}
}
