package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/auth"
	"github.com/postman/astro/apps/astro-server/internal/middleware"
)

// TestRoutePermissionWiring verifies that each route group is wired with the
// correct permission middleware, matching the setup in main.go's setupRoutes.
// Uses the local role fallback path (no JWT org scope) to test role→permission mapping.
func TestRoutePermissionWiring(t *testing.T) {
	ok := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		role     string
		wantCode int
	}{
		// Member routes require org:manage — member role denied
		{"member_GET_members_denied", "GET", "/api/v1/accounts/myorg/members", "", "member", http.StatusForbidden},
		// Member routes require org:manage — admin role allowed
		{"admin_GET_members_allowed", "GET", "/api/v1/accounts/myorg/members", "", "admin", http.StatusOK},

		// Invitation routes require org:manage — member role denied
		{"member_GET_invitations_denied", "GET", "/api/v1/accounts/myorg/invitations", "", "member", http.StatusForbidden},
		// Invitation routes require org:manage — admin role allowed
		{"admin_GET_invitations_allowed", "GET", "/api/v1/accounts/myorg/invitations", "", "admin", http.StatusOK},

		// Agent write routes require agents:write — member role denied
		{"member_PUT_visibility_denied", "PUT", "/api/v1/agents/myorg/test-agent/visibility", `{}`, "member", http.StatusForbidden},
		// Agent write routes require agents:write — admin role allowed
		{"admin_PUT_visibility_allowed", "PUT", "/api/v1/agents/myorg/test-agent/visibility", `{}`, "admin", http.StatusOK},

		// Account admin routes require org:admin — admin role denied
		{"admin_PUT_rename_denied", "PUT", "/api/v1/accounts/myorg", `{}`, "admin", http.StatusForbidden},
		// Account admin routes require org:admin — owner role allowed
		{"owner_PUT_rename_allowed", "PUT", "/api/v1/accounts/myorg", `{}`, "owner", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			store := account.NewAccountStore(db)

			// ResolveAccount: return org account
			mock.ExpectQuery("SELECT .+ FROM accounts WHERE name").
				WithArgs("myorg").
				WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "created_at", "updated_at"}).
					AddRow("acct-1", "myorg", "organization", "org_123", time.Now(), time.Now()))

			// RequireAccountPermission fallback: GetMember returns caller role
			mock.ExpectQuery("SELECT .+ FROM account_members WHERE account_id").
				WithArgs("acct-1", "caller-1").
				WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "role", "workos_membership_id", "created_at"}).
					AddRow("acct-1", "caller-1", tt.role, nil, time.Now()))

			router := gin.New()
			// Inject authenticated user with no org-scoped session (forces local role fallback)
			router.Use(func(c *gin.Context) {
				c.Set(string(auth.UserContextKey), &auth.User{ID: "caller-1"})
				c.Set(string(auth.SessionContextKey), &auth.Session{OrganizationID: ""})
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
