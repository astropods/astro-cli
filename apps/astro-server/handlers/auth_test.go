package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/workos/workos-go/v6/pkg/workos_errors"
)

// ─── stubs for fetchAccounts unit tests ──────────────────────────────────────

type stubOrgSyncer struct {
	roles      map[string]string
	syncCalls  int
	syncUserID string
}

func (s *stubOrgSyncer) GetMembershipRoles(_ context.Context, _ string) map[string]string {
	return s.roles
}

func (s *stubOrgSyncer) SyncMembershipsForUser(_ context.Context, userID string) error {
	s.syncCalls++
	s.syncUserID = userID
	return nil
}

type stubAccountGetter struct {
	accounts []account.AccountWithRole
	err      error
}

func (s *stubAccountGetter) GetAccountsForUser(_ string) ([]account.AccountWithRole, error) {
	return s.accounts, s.err
}

func (s *stubAccountGetter) TouchAvatarUpdatedAtByName(_ string) (time.Time, error) {
	return time.Time{}, nil
}

func init() {
	gin.SetMode(gin.TestMode)
}

// createTestAuthHandler creates an AuthHandler with minimal config for testing
// Note: This won't have a functional WorkOS client, but we can test cookie behavior
func createTestAuthHandler(sameSite string) *AuthHandler {
	log := logger.New("error", "json")

	cfg := &config.Config{
		Auth: config.AuthConfig{
			CookieName:     "test_session",
			CookiePassword: "test-password-that-is-32-chars!!",
			CookieDomain:   "",
			CookieSecure:   false,
			CookieSameSite: sameSite,
			CookieMaxAge:   7 * 24 * time.Hour,
			SessionMaxAge:  24 * time.Hour,
			FrontendURL:    "http://localhost:5173",
		},
	}

	sessionManager := auth.NewSessionManager(cfg.Auth.CookiePassword, cfg.Auth.SessionMaxAge)

	return &AuthHandler{
		log:            log,
		cfg:            cfg,
		sessionManager: sessionManager,
	}
}

// TestSetSameSiteMode_Lax verifies that SameSite=Lax is set correctly
func TestSetSameSiteMode_Lax(t *testing.T) {
	handler := createTestAuthHandler("Lax")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		c.SetCookie("test_cookie", "value", 3600, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Check the Set-Cookie header
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie to be set")
	}

	cookie := cookies[0]
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax, got %v", cookie.SameSite)
	}
}

// TestSetSameSiteMode_Strict verifies that SameSite=Strict is set correctly
func TestSetSameSiteMode_Strict(t *testing.T) {
	handler := createTestAuthHandler("Strict")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		c.SetCookie("test_cookie", "value", 3600, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie to be set")
	}

	cookie := cookies[0]
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite=Strict, got %v", cookie.SameSite)
	}
}

// TestSetSameSiteMode_None verifies that SameSite=None is set correctly
func TestSetSameSiteMode_None(t *testing.T) {
	handler := createTestAuthHandler("None")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		c.SetCookie("test_cookie", "value", 3600, "/", "", true, true) // Secure must be true for SameSite=None
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie to be set")
	}

	cookie := cookies[0]
	if cookie.SameSite != http.SameSiteNoneMode {
		t.Errorf("expected SameSite=None, got %v", cookie.SameSite)
	}
}

// TestSetSameSiteMode_DefaultsToLax verifies that unknown values default to Lax
func TestSetSameSiteMode_DefaultsToLax(t *testing.T) {
	handler := createTestAuthHandler("InvalidValue")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		c.SetCookie("test_cookie", "value", 3600, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie to be set")
	}

	cookie := cookies[0]
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax for invalid config value, got %v", cookie.SameSite)
	}
}

// TestSetSameSiteMode_EmptyDefaultsToLax verifies that empty string defaults to Lax
func TestSetSameSiteMode_EmptyDefaultsToLax(t *testing.T) {
	handler := createTestAuthHandler("")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		c.SetCookie("test_cookie", "value", 3600, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie to be set")
	}

	cookie := cookies[0]
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax for empty config, got %v", cookie.SameSite)
	}
}

// TestSameSiteInSetCookieHeader verifies the raw Set-Cookie header contains SameSite
func TestSameSiteInSetCookieHeader(t *testing.T) {
	tests := []struct {
		name             string
		sameSiteConfig   string
		expectedInHeader string
	}{
		{
			name:             "Lax mode",
			sameSiteConfig:   "Lax",
			expectedInHeader: "SameSite=Lax",
		},
		{
			name:             "Strict mode",
			sameSiteConfig:   "Strict",
			expectedInHeader: "SameSite=Strict",
		},
		{
			name:             "None mode",
			sameSiteConfig:   "None",
			expectedInHeader: "SameSite=None",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := createTestAuthHandler(tt.sameSiteConfig)

			router := gin.New()
			router.GET("/test", func(c *gin.Context) {
				handler.setSameSiteMode(c)
				c.SetCookie("test_cookie", "value", 3600, "/", "", tt.sameSiteConfig == "None", true)
				c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			// Check the raw Set-Cookie header
			setCookieHeader := rec.Header().Get("Set-Cookie")
			if setCookieHeader == "" {
				t.Fatal("expected Set-Cookie header to be present")
			}

			if !strings.Contains(setCookieHeader, tt.expectedInHeader) {
				t.Errorf("expected Set-Cookie header to contain %q, got: %s", tt.expectedInHeader, setCookieHeader)
			}
		})
	}
}

// TestMultipleCookiesSameSite verifies SameSite is applied to multiple cookies
func TestMultipleCookiesSameSite(t *testing.T) {
	handler := createTestAuthHandler("Lax")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		c.SetCookie("cookie1", "value1", 3600, "/", "", false, true)
		c.SetCookie("cookie2", "value2", 3600, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("expected 2 cookies, got %d", len(cookies))
	}

	for _, cookie := range cookies {
		if cookie.SameSite != http.SameSiteLaxMode {
			t.Errorf("expected cookie %s to have SameSite=Lax, got %v", cookie.Name, cookie.SameSite)
		}
	}
}

// TestCookieClearingSameSite verifies SameSite is set even when clearing cookies
func TestCookieClearingSameSite(t *testing.T) {
	handler := createTestAuthHandler("Lax")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		// Clear a cookie by setting maxAge to -1
		c.SetCookie("session", "", -1, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie to be set (for clearing)")
	}

	cookie := cookies[0]
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax even when clearing cookie, got %v", cookie.SameSite)
	}
}

// TestMe_ReturnsPermissions verifies that the /auth/me endpoint includes
// permissions from the sealed session in the response.
func TestMe_ReturnsPermissions(t *testing.T) {
	handler := createTestAuthHandler("Lax")

	// Create a session with permissions and seal it
	sessionData := &auth.SessionData{
		Session: &auth.Session{
			ID:          "session_perm",
			UserID:      "user_perm",
			Role:        "admin",
			Permissions: []string{"admin:view", "deployments:write"},
			AccessToken: "token",
			ExpiresAt:   time.Now().Add(1 * time.Hour),
			CreatedAt:   time.Now(),
		},
		User: &auth.User{
			ID:    "user_perm",
			Email: "admin@example.com",
		},
	}

	sealed, err := handler.sessionManager.SealSession(sessionData)
	if err != nil {
		t.Fatalf("failed to seal session: %v", err)
	}

	router := gin.New()
	router.GET("/auth/me", handler.Me())

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{
		Name:  handler.cfg.Auth.CookieName,
		Value: sealed,
	})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp auth.AuthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Role != "admin" {
		t.Errorf("Role = %q, want %q", resp.Role, "admin")
	}
	if len(resp.Permissions) != 2 {
		t.Fatalf("Permissions length = %d, want 2", len(resp.Permissions))
	}
	if resp.Permissions[0] != "admin:view" || resp.Permissions[1] != "deployments:write" {
		t.Errorf("Permissions = %v, want [admin:view deployments:write]", resp.Permissions)
	}
}

func TestMe_HydratesLegacyWorkOSMembershipIDFromJWTWithoutDBFallback(t *testing.T) {
	handler := createTestAuthHandler("Lax")
	resolver := &stubMembershipIDResolver{membershipID: "om_from_db"}
	handler.membershipResolver = resolver
	accessToken := testAccessTokenWithClaims(t, map[string]any{
		"organization_membership_id": "om_from_jwt",
	})

	router := gin.New()
	router.GET("/auth/me", handler.Me())

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{
		Name:  handler.cfg.Auth.CookieName,
		Value: sealedSessionCookieWithAccessToken(t, handler, accessToken),
	})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	setCookies := rec.Result().Cookies()
	require.NotEmpty(t, setCookies)

	sessionData, err := handler.sessionManager.UnsealSession(setCookies[0].Value)
	require.NoError(t, err)
	assert.Equal(t, "om_from_jwt", sessionData.Session.WorkOSMembershipID)
	assert.Zero(t, resolver.calls)
}

func TestMe_DoesNotQueryDBWhenJWTMembershipClaimMissing(t *testing.T) {
	handler := createTestAuthHandler("Lax")
	resolver := &stubMembershipIDResolver{membershipID: "om_from_db"}
	handler.membershipResolver = resolver

	router := gin.New()
	router.GET("/auth/me", handler.Me())

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{
		Name:  handler.cfg.Auth.CookieName,
		Value: sealedSessionCookie(t, handler),
	})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Result().Cookies())
	assert.Zero(t, resolver.calls)
}

// TestFetchAccounts covers the fetchAccounts helper in isolation.
func TestFetchAccounts(t *testing.T) {
	orgAcct := account.AccountWithRole{
		ID:                   "org-1",
		Name:                 "my-org",
		Type:                 "organization",
		WorkOSOrganizationID: "wos-org-abc",
		DisplayName:          "My Org",
	}
	personalAcct := account.AccountWithRole{
		ID:          "personal-1",
		Name:        "alice",
		Type:        "personal",
		DisplayName: "Alice",
	}

	t.Run("nil orgSync and nil accountStore returns empty slice", func(t *testing.T) {
		h := createTestAuthHandler("Lax")
		got := h.fetchAccounts(context.Background(), "user-1")
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("accounts present but no orgSync gives empty role", func(t *testing.T) {
		h := createTestAuthHandler("Lax")
		h.accountStore = &stubAccountGetter{accounts: []account.AccountWithRole{orgAcct}}
		got := h.fetchAccounts(context.Background(), "user-1")
		require.Len(t, got, 1)
		assert.Equal(t, "org-1", got[0].ID)
		assert.Empty(t, got[0].Role)
	})

	t.Run("org account gets role from orgSync WorkOS ID map", func(t *testing.T) {
		h := createTestAuthHandler("Lax")
		h.orgSync = &stubOrgSyncer{roles: map[string]string{"wos-org-abc": "admin"}}
		h.accountStore = &stubAccountGetter{accounts: []account.AccountWithRole{orgAcct, personalAcct}}
		got := h.fetchAccounts(context.Background(), "user-1")
		require.Len(t, got, 2)
		assert.Equal(t, "admin", got[0].Role)
		assert.Empty(t, got[1].Role, "personal account should have no role")
	})

	t.Run("accountStore error returns empty non-nil slice", func(t *testing.T) {
		h := createTestAuthHandler("Lax")
		h.orgSync = &stubOrgSyncer{roles: map[string]string{"wos-org-abc": "member"}}
		h.accountStore = &stubAccountGetter{err: fmt.Errorf("db unavailable")}
		got := h.fetchAccounts(context.Background(), "user-1")
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
}

// TestMe_ReturnsEmptyPermissions verifies that the response always contains
// a permissions array (never null), even when the session has no permissions.
func TestMe_ReturnsEmptyPermissions(t *testing.T) {
	handler := createTestAuthHandler("Lax")

	sessionData := &auth.SessionData{
		Session: &auth.Session{
			ID:          "session_no_perms",
			UserID:      "user_no_perms",
			AccessToken: "token",
			ExpiresAt:   time.Now().Add(1 * time.Hour),
			CreatedAt:   time.Now(),
		},
		User: &auth.User{
			ID:    "user_no_perms",
			Email: "user@example.com",
		},
	}

	sealed, err := handler.sessionManager.SealSession(sessionData)
	if err != nil {
		t.Fatalf("failed to seal session: %v", err)
	}

	router := gin.New()
	router.GET("/auth/me", handler.Me())

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{
		Name:  handler.cfg.Auth.CookieName,
		Value: sealed,
	})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	// Decode as raw JSON to verify permissions is [] not null
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	permsRaw, ok := raw["permissions"]
	if !ok {
		t.Fatal("response missing 'permissions' field")
	}
	if string(permsRaw) != "[]" {
		t.Errorf("permissions = %s, want []", string(permsRaw))
	}
}

// stubOrgRefresher implements orgTokenRefresher for SwitchOrg tests.
type stubOrgRefresher struct {
	result *auth.RefreshResult
	err    error
}

func (s *stubOrgRefresher) AuthenticateWithRefreshTokenForOrg(_ context.Context, _, _ string) (*auth.RefreshResult, error) {
	return s.result, s.err
}

func sealedSessionCookie(t *testing.T, h *AuthHandler) string {
	return sealedSessionCookieWithAccessToken(t, h, "access-token")
}

func sealedSessionCookieWithAccessToken(t *testing.T, h *AuthHandler, accessToken string) string {
	t.Helper()
	sessionData := &auth.SessionData{
		Session: &auth.Session{
			ID:             "sess-1",
			UserID:         "user-1",
			OrganizationID: "org-1",
			RefreshToken:   "refresh-token",
			AccessToken:    accessToken,
			ExpiresAt:      time.Now().Add(1 * time.Hour),
			CreatedAt:      time.Now(),
		},
		User: &auth.User{ID: "user-1", Email: "user@example.com"},
	}
	sealed, err := h.sessionManager.SealSession(sessionData)
	require.NoError(t, err)
	return sealed
}

func TestSwitchOrg_SessionExpired_Returns401(t *testing.T) {
	handler := createTestAuthHandler("Lax")
	handler.orgRefresher = &stubOrgRefresher{
		err: fmt.Errorf("failed to refresh token: %w", workos_errors.HTTPError{
			Code:      400,
			ErrorCode: "invalid_grant",
			Message:   "invalid_grant Session has already ended",
		}),
	}

	router := gin.New()
	router.POST("/auth/switch-org", handler.SwitchOrg())

	body := `{"organization_id":"org-2"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/switch-org", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: handler.cfg.Auth.CookieName, Value: sealedSessionCookie(t, handler)})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "session_expired", resp["error"])
}

func TestSwitchOrg_OtherError_Returns400(t *testing.T) {
	handler := createTestAuthHandler("Lax")
	handler.orgRefresher = &stubOrgRefresher{
		err: fmt.Errorf("some other workos error"),
	}

	router := gin.New()
	router.POST("/auth/switch-org", handler.SwitchOrg())

	body := `{"organization_id":"org-2"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/switch-org", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: handler.cfg.Auth.CookieName, Value: sealedSessionCookie(t, handler)})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "switch_failed", resp["error"])
}

type stubMembershipIDResolver struct {
	membershipID string
	orgErr       error
	calls        int
}

func (s *stubMembershipIDResolver) GetByWorkOSOrganizationID(orgID string) (*account.Account, error) {
	s.calls++
	if s.orgErr != nil {
		return nil, s.orgErr
	}
	return &account.Account{ID: "acct-1", WorkOSOrganizationID: orgID}, nil
}

func (s *stubMembershipIDResolver) GetMember(_, _ string) (*account.AccountMember, error) {
	s.calls++
	return &account.AccountMember{WorkOSMembershipID: s.membershipID}, nil
}

func testAccessTokenWithClaims(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))
	return header + "." + encodedPayload + "." + signature
}

func TestSwitchOrg_PopulatesWorkOSMembershipIDFromJWT(t *testing.T) {
	handler := createTestAuthHandler("Lax")
	handler.orgRefresher = &stubOrgRefresher{
		result: &auth.RefreshResult{
			AccessToken: testAccessTokenWithClaims(t, map[string]any{
				"sid":                        "session_1",
				"role":                       "admin",
				"permissions":                []string{"org:manage"},
				"organization_membership_id": "om_from_jwt",
			}),
			RefreshToken: "refresh-token-new",
		},
	}

	router := gin.New()
	router.POST("/auth/switch-org", handler.SwitchOrg())

	body := `{"organization_id":"org-2"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/switch-org", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: handler.cfg.Auth.CookieName, Value: sealedSessionCookie(t, handler)})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	setCookies := rec.Result().Cookies()
	require.NotEmpty(t, setCookies)
	sessionData, err := handler.sessionManager.UnsealSession(setCookies[0].Value)
	require.NoError(t, err)
	assert.Equal(t, "om_from_jwt", sessionData.Session.WorkOSMembershipID)
	assert.Equal(t, "org-2", sessionData.Session.OrganizationID)
}

func TestPopulateSessionMembership_DBFallbackWhenClaimMissing(t *testing.T) {
	handler := createTestAuthHandler("Lax")
	handler.membershipResolver = &stubMembershipIDResolver{membershipID: "om_from_db"}

	session := &auth.Session{
		UserID:         "user-1",
		OrganizationID: "org-2",
	}
	handler.populateSessionMembership(session, auth.TokenClaims{})

	assert.Equal(t, "om_from_db", session.WorkOSMembershipID)
}

func TestPopulateSessionMembership_OverwritesStaleMembershipOnOrgSwitch(t *testing.T) {
	handler := createTestAuthHandler("Lax")
	handler.membershipResolver = &stubMembershipIDResolver{membershipID: "om_for_org_2"}

	session := &auth.Session{
		UserID:             "user-1",
		OrganizationID:     "org-2",
		WorkOSMembershipID: "om_stale_from_org_1",
	}
	handler.populateSessionMembership(session, auth.TokenClaims{})

	assert.Equal(t, "om_for_org_2", session.WorkOSMembershipID)
}

func TestPopulateSessionMembership_LogsExpectedFailuresAtDebug(t *testing.T) {
	tests := []struct {
		name     string
		resolver auth.MembershipIDResolver
	}{
		{
			name:     "member has no WorkOS membership link",
			resolver: &stubMembershipIDResolver{},
		},
		{
			name: "organization has no local account yet",
			resolver: &stubMembershipIDResolver{
				orgErr: fmt.Errorf("account not found for workos org: %w", account.ErrAccountNotFound),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			handler := createTestAuthHandler("Lax")
			handler.log = &logger.Logger{Logger: slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))}
			handler.membershipResolver = tt.resolver
			session := &auth.Session{UserID: "user-1", OrganizationID: "org-1"}

			handler.populateSessionMembership(session, auth.TokenClaims{})

			assert.Empty(t, session.WorkOSMembershipID)
			assert.Contains(t, output.String(), "level=DEBUG")
			assert.NotContains(t, output.String(), "level=WARN")
		})
	}
}

func TestPopulateSessionMembership_LogsQueryFailuresAtWarn(t *testing.T) {
	var output bytes.Buffer
	handler := createTestAuthHandler("Lax")
	handler.log = &logger.Logger{Logger: slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	handler.membershipResolver = &stubMembershipIDResolver{orgErr: errors.New("database unavailable")}
	session := &auth.Session{UserID: "user-1", OrganizationID: "org-1"}

	handler.populateSessionMembership(session, auth.TokenClaims{})

	assert.Empty(t, session.WorkOSMembershipID)
	assert.Contains(t, output.String(), "level=WARN")
}

func TestSwitchOrg_SyncsMembershipsBeforePopulate(t *testing.T) {
	handler := createTestAuthHandler("Lax")
	orgSync := &stubOrgSyncer{}
	handler.orgSync = orgSync
	handler.orgRefresher = &stubOrgRefresher{
		result: &auth.RefreshResult{
			AccessToken: testAccessTokenWithClaims(t, map[string]any{
				"sid":  "session_1",
				"role": "member",
			}),
			RefreshToken: "refresh-token-new",
		},
	}

	router := gin.New()
	router.POST("/auth/switch-org", handler.SwitchOrg())

	body := `{"organization_id":"org-2"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/switch-org", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: handler.cfg.Auth.CookieName, Value: sealedSessionCookie(t, handler)})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, orgSync.syncCalls)
	assert.Equal(t, "user-1", orgSync.syncUserID)
}

func TestSwitchOrg_PopulatesWorkOSMembershipIDFromDBWhenClaimMissing(t *testing.T) {
	handler := createTestAuthHandler("Lax")
	handler.membershipResolver = &stubMembershipIDResolver{membershipID: "om_from_db"}
	handler.orgRefresher = &stubOrgRefresher{
		result: &auth.RefreshResult{
			AccessToken: testAccessTokenWithClaims(t, map[string]any{
				"sid":  "session_1",
				"role": "admin",
			}),
			RefreshToken: "refresh-token-new",
		},
	}

	router := gin.New()
	router.POST("/auth/switch-org", handler.SwitchOrg())

	body := `{"organization_id":"org-2"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/switch-org", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: handler.cfg.Auth.CookieName, Value: sealedSessionCookie(t, handler)})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	sessionData, err := handler.sessionManager.UnsealSession(rec.Result().Cookies()[0].Value)
	require.NoError(t, err)
	assert.Equal(t, "om_from_db", sessionData.Session.WorkOSMembershipID)
}
