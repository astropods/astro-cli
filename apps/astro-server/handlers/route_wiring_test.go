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

	baseMember := []string{"agents:read", "agents:write", "deployments:read", "deployments:write"}
	orgAdminNoVault := []string{"agents:read", "agents:write", "deployments:write", "org:manage"}
	withVaultRead := append(orgAdminNoVault, "variable:read")
	withVaultWrite := append(orgAdminNoVault, "variable:write")

	tests := []struct {
		name         string
		method       string
		path         string
		body         string
		permissions  []string
		wantCode     int
		sessionOrgID string // empty → org_123 (matches resolved account)
	}{
		// Member routes require org:manage — member permissions denied
		{"member_GET_members_denied", "GET", "/api/v1/accounts/myorg/members", "",
			[]string{"agents:read", "deployments:write"}, http.StatusForbidden, ""},
		// Member routes require org:manage — admin permissions allowed
		{"admin_GET_members_allowed", "GET", "/api/v1/accounts/myorg/members", "",
			[]string{"agents:read", "agents:write", "deployments:write", "org:manage"}, http.StatusOK, ""},

		// Invitation routes require org:manage — member permissions denied
		{"member_GET_invitations_denied", "GET", "/api/v1/accounts/myorg/invitations", "",
			[]string{"agents:read", "deployments:write"}, http.StatusForbidden, ""},
		// Invitation routes require org:manage — admin permissions allowed
		{"admin_GET_invitations_allowed", "GET", "/api/v1/accounts/myorg/invitations", "",
			[]string{"agents:read", "agents:write", "deployments:write", "org:manage"}, http.StatusOK, ""},

		// Agent write routes require agents:write — member permissions denied
		{"member_PUT_visibility_denied", "PUT", "/api/v1/agents/myorg/test-agent/visibility", `{}`,
			[]string{"agents:read", "deployments:write"}, http.StatusForbidden, ""},
		// Agent write routes require agents:write — admin permissions allowed
		{"admin_PUT_visibility_allowed", "PUT", "/api/v1/agents/myorg/test-agent/visibility", `{}`,
			[]string{"agents:read", "agents:write", "deployments:write", "org:manage"}, http.StatusOK, ""},

		// Account admin routes require org:admin — admin permissions denied
		{"admin_PUT_rename_denied", "PUT", "/api/v1/accounts/myorg", `{}`,
			[]string{"agents:read", "agents:write", "deployments:write", "org:manage"}, http.StatusForbidden, ""},
		// Account admin routes require org:admin — owner permissions allowed
		{"owner_PUT_rename_allowed", "PUT", "/api/v1/accounts/myorg", `{}`,
			[]string{"agents:read", "agents:write", "deployments:write", "org:manage", "org:admin"}, http.StatusOK, ""},

		// Quota increase requires org:admin — member denied
		{"member_POST_quota_increase_denied", "POST", "/api/v1/accounts/myorg/quota-increase", `{}`,
			baseMember, http.StatusForbidden, ""},
		// Quota increase requires org:admin — admin allowed
		{"admin_POST_quota_increase_allowed", "POST", "/api/v1/accounts/myorg/quota-increase", `{}`,
			[]string{"agents:read", "agents:write", "deployments:write", "org:manage", "org:admin"}, http.StatusOK, ""},

		// Vault GET routes require variable:read
		{"member_GET_variables_denied", "GET", "/api/v1/accounts/myorg/variables", "",
			baseMember, http.StatusForbidden, ""},
		{"admin_GET_variables_denied_without_variable_read", "GET", "/api/v1/accounts/myorg/variables", "",
			orgAdminNoVault, http.StatusForbidden, ""},
		{"admin_GET_variables_allowed_with_variable_read", "GET", "/api/v1/accounts/myorg/variables", "",
			withVaultRead, http.StatusOK, ""},
		{"admin_GET_variable_by_name_allowed_with_variable_read", "GET", "/api/v1/accounts/myorg/variables/MY_KEY", "",
			withVaultRead, http.StatusOK, ""},
		{"GET_variables_denied_write_only_jwt", "GET", "/api/v1/accounts/myorg/variables", "",
			withVaultWrite, http.StatusForbidden, ""},

		{"GET_variables_denied_wrong_org_jwt", "GET", "/api/v1/accounts/myorg/variables", "",
			withVaultRead, http.StatusForbidden, "org_other"},

		// Vault mutations require variable:write
		{"member_POST_variables_denied", "POST", "/api/v1/accounts/myorg/variables", `{"variables":[]}`,
			baseMember, http.StatusForbidden, ""},
		{"POST_variables_denied_read_only_jwt", "POST", "/api/v1/accounts/myorg/variables", `{"variables":[]}`,
			withVaultRead, http.StatusForbidden, ""},
		{"POST_variables_allowed_write_only_jwt", "POST", "/api/v1/accounts/myorg/variables", `{"variables":[]}`,
			withVaultWrite, http.StatusOK, ""},

		{"member_PUT_variables_denied", "PUT", "/api/v1/accounts/myorg/variables/Foo", `{}`,
			baseMember, http.StatusForbidden, ""},
		{"PUT_variables_denied_read_only_jwt", "PUT", "/api/v1/accounts/myorg/variables/Foo", `{}`,
			withVaultRead, http.StatusForbidden, ""},
		{"PUT_variables_allowed_write_only_jwt", "PUT", "/api/v1/accounts/myorg/variables/Foo", `{}`,
			withVaultWrite, http.StatusOK, ""},

		{"member_DELETE_variables_denied", "DELETE", "/api/v1/accounts/myorg/variables/Foo", "",
			baseMember, http.StatusForbidden, ""},
		{"DELETE_variables_denied_read_only_jwt", "DELETE", "/api/v1/accounts/myorg/variables/Foo", "",
			withVaultRead, http.StatusForbidden, ""},
		{"DELETE_variables_allowed_write_only_jwt", "DELETE", "/api/v1/accounts/myorg/variables/Foo", "",
			withVaultWrite, http.StatusOK, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			store := account.NewAccountStore(db)

			// ResolveAccount: return org account
			mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
				WithArgs("myorg").
				WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
					AddRow("acct-1", "myorg", "organization", "org_123", nil, time.Now(), time.Now(), "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

			router := gin.New()
			sessionOrgID := tt.sessionOrgID
			if sessionOrgID == "" {
				sessionOrgID = "org_123"
			}
			router.Use(func(c *gin.Context) {
				c.Set(string(auth.UserContextKey), &auth.User{ID: "caller-1"})
				c.Set(string(auth.SessionContextKey), &auth.Session{
					OrganizationID: sessionOrgID,
					Permissions:    tt.permissions,
				})
				c.Next()
			})

			v1 := router.Group("/api/v1")

			accountAdmin := v1.Group("/accounts/:account")
			accountAdmin.Use(middleware.ResolveAccount(store))
			accountAdmin.Use(middleware.RequireAccountPermission(store, "org:admin"))
			accountAdmin.PUT("", ok)
			accountAdmin.POST("/quota-increase", ok)

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

			accountVarsRead := v1.Group("/accounts/:account")
			accountVarsRead.Use(middleware.ResolveAccount(store))
			accountVarsRead.Use(middleware.RequireAccountPermission(store, "variable:read"))
			accountVarsRead.GET("/variables", ok)
			accountVarsRead.GET("/variables/:varName", ok)

			accountVarsWrite := v1.Group("/accounts/:account")
			accountVarsWrite.Use(middleware.ResolveAccount(store))
			accountVarsWrite.Use(middleware.RequireAccountPermission(store, "variable:write"))
			accountVarsWrite.POST("/variables", ok)
			accountVarsWrite.PUT("/variables/:varName", ok)
			accountVarsWrite.DELETE("/variables/:varName", ok)

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
