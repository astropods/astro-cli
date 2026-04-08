package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// injectTestOrgAccount sets an org account + user + session in context for testing org handlers
func injectTestOrgAccount(acct *account.Account, user *auth.User) gin.HandlerFunc {
	return func(c *gin.Context) {
		if acct != nil {
			c.Set(string(auth.AccountContextKey), acct)
		}
		if user != nil {
			c.Set(string(auth.UserContextKey), user)
		}
		c.Next()
	}
}

// injectTestOrgAccountWithSession sets account, user, and session in context
func injectTestOrgAccountWithSession(acct *account.Account, user *auth.User, session *auth.Session) gin.HandlerFunc {
	return func(c *gin.Context) {
		if acct != nil {
			c.Set(string(auth.AccountContextKey), acct)
		}
		if user != nil {
			c.Set(string(auth.UserContextKey), user)
		}
		if session != nil {
			c.Set(string(auth.SessionContextKey), session)
		}
		c.Next()
	}
}

// --- ListMembers tests ---

func TestListMembers_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery("SELECT .+ FROM account_members am LEFT JOIN account_member_workos mw").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "workos_membership_id", "created_at"}).
			AddRow("acct-1", "user-1", "wm-1", time.Now()).
			AddRow("acct-1", "user-2", nil, time.Now()))

	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	user := &auth.User{ID: "user-1"}

	router := gin.New()
	router.GET("/members", injectTestOrgAccount(acct, user), ListMembers(log, store, nil))

	req := httptest.NewRequest(http.MethodGet, "/members", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	var members []account.AccountMember
	if err := json.Unmarshal(resp["members"], &members); err != nil {
		t.Fatalf("failed to unmarshal members: %v", err)
	}

	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}
}

func TestListMembers_NoAccount(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	router := gin.New()
	router.GET("/members", injectTestOrgAccount(nil, nil), ListMembers(log, store, nil))

	req := httptest.NewRequest(http.MethodGet, "/members", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when no account, got %d", rec.Code)
	}
}

func TestListMembers_DBError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery("SELECT .+ FROM account_members am LEFT JOIN account_member_workos mw").
		WithArgs("acct-1").
		WillReturnError(sqlmock.ErrCancelled)

	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	user := &auth.User{ID: "user-1"}

	router := gin.New()
	router.GET("/members", injectTestOrgAccount(acct, user), ListMembers(log, store, nil))

	req := httptest.NewRequest(http.MethodGet, "/members", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on DB error, got %d", rec.Code)
	}
}

// --- AddMember tests (handler-level validation only) ---

func TestAddMember_InvalidBody_MissingFields(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}

	router := gin.New()
	// syncSvc is nil — we test that validation fires before sync is called
	router.POST("/members", injectTestOrgAccount(acct, nil), AddMember(log, nil, nil, nil, nil, nil))

	tests := []struct {
		name string
		body string
	}{
		{"missing user_id", `{"role": "admin"}`},
		{"missing role", `{"user_id": "user-1"}`},
		{"empty body", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/members", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAddMember_NoAccount(t *testing.T) {
	log := logger.New("error", "json")

	router := gin.New()
	router.POST("/members", injectTestOrgAccount(nil, nil), AddMember(log, nil, nil, nil, nil, nil))

	body := `{"user_id": "user-1", "role": "admin"}`
	req := httptest.NewRequest(http.MethodPost, "/members", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// --- UpdateMemberRole tests ---

func TestUpdateMemberRole_InvalidBody(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}

	router := gin.New()
	router.PUT("/members/:user_id", injectTestOrgAccount(acct, nil), UpdateMemberRole(log, nil, nil, nil))

	req := httptest.NewRequest(http.MethodPut, "/members/user-1", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateMemberRole_NoAccount(t *testing.T) {
	log := logger.New("error", "json")

	router := gin.New()
	router.PUT("/members/:user_id", injectTestOrgAccount(nil, nil), UpdateMemberRole(log, nil, nil, nil))

	body := `{"role": "admin"}`
	req := httptest.NewRequest(http.MethodPut, "/members/user-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// --- RemoveMember tests ---

func TestRemoveMember_NoAccount(t *testing.T) {
	log := logger.New("error", "json")

	router := gin.New()
	router.DELETE("/members/:user_id", injectTestOrgAccount(nil, nil), RemoveMember(log, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/members/user-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// --- CreateInvitations tests ---

func TestCreateInvitations_NonOrgAccount(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "personal", Type: "personal", WorkOSOrganizationID: ""}
	user := &auth.User{ID: "user-1", Email: "test@example.com"}

	router := gin.New()
	router.POST("/invitations", injectTestOrgAccount(acct, user), CreateInvitations(log, nil, nil))

	body := `{"invitations": [{"value": "invite@example.com", "kind": "email", "role": "member"}]}`
	req := httptest.NewRequest(http.MethodPost, "/invitations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-org account, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "invitations are only supported for organization accounts" {
		t.Errorf("unexpected error: %s", resp["error"])
	}
}

func TestCreateInvitations_NoAuth(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}

	router := gin.New()
	// No user injected
	router.POST("/invitations", injectTestOrgAccount(acct, nil), CreateInvitations(log, nil, nil))

	body := `{"invitations": [{"value": "invite@example.com", "kind": "email", "role": "member"}]}`
	req := httptest.NewRequest(http.MethodPost, "/invitations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with no user, got %d", rec.Code)
	}
}

func TestCreateInvitations_InvalidBody(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	user := &auth.User{ID: "user-1"}

	router := gin.New()
	router.POST("/invitations", injectTestOrgAccount(acct, user), CreateInvitations(log, nil, nil))

	// Empty invitations array should fail
	req := httptest.NewRequest(http.MethodPost, "/invitations", strings.NewReader(`{"invitations":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateInvitations_NoAccount(t *testing.T) {
	log := logger.New("error", "json")

	router := gin.New()
	router.POST("/invitations", injectTestOrgAccount(nil, nil), CreateInvitations(log, nil, nil))

	body := `{"invitations": [{"value": "invite@example.com", "kind": "email", "role": "member"}]}`
	req := httptest.NewRequest(http.MethodPost, "/invitations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// --- RevokeInvitation tests ---

func TestRevokeInvitation_NoAccount(t *testing.T) {
	log := logger.New("error", "json")

	router := gin.New()
	// No account injected — handler should return 500
	router.DELETE("/invitations/:id", injectTestOrgAccount(nil, nil), RevokeInvitation(log, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/invitations/inv-123", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when no account, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- ListAccountInvitations tests ---

func TestListAccountInvitations_NoWorkOSOrgID(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "personal", Type: "personal", WorkOSOrganizationID: ""}

	router := gin.New()
	router.GET("/invitations", injectTestOrgAccount(acct, nil), ListAccountInvitations(log, nil))

	req := httptest.NewRequest(http.MethodGet, "/invitations", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if string(resp["invitations"]) != "[]" {
		t.Errorf("expected empty invitations array, got %s", string(resp["invitations"]))
	}
}

func TestListAccountInvitations_NoAccount(t *testing.T) {
	log := logger.New("error", "json")

	router := gin.New()
	router.GET("/invitations", injectTestOrgAccount(nil, nil), ListAccountInvitations(log, nil))

	req := httptest.NewRequest(http.MethodGet, "/invitations", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// --- Role Escalation Prevention tests ---
// These now check session.Role from JWT instead of DB role lookup

func TestAddMember_RoleEscalation_NonOwnerCannotAssignOwner(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	caller := &auth.User{ID: "caller-1", Email: "admin@example.com"}
	session := &auth.Session{Role: "admin"}

	router := gin.New()
	router.POST("/members", injectTestOrgAccountWithSession(acct, caller, session), AddMember(log, nil, nil, nil, nil, nil))

	body := `{"user_id": "new-user", "role": "owner"}`
	req := httptest.NewRequest(http.MethodPost, "/members", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("non-owner session should not be able to assign owner role, expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAddMember_OwnerCanAssignOwner(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	caller := &auth.User{ID: "caller-1", Email: "owner@example.com"}
	session := &auth.Session{Role: "owner"}

	// syncSvc with a real store — GetByID will return "not found" instead of panicking
	syncSvc := org.NewSync(nil, store, nil)

	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("acct-1").
		WillReturnError(sqlmock.ErrCancelled)

	router := gin.New()
	router.POST("/members", injectTestOrgAccountWithSession(acct, caller, session), AddMember(log, syncSvc, store, nil, nil, nil))

	body := `{"user_id": "new-user", "role": "owner"}`
	req := httptest.NewRequest(http.MethodPost, "/members", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Should NOT be 403 — the owner guard should pass (syncSvc returns DB error → 400)
	if rec.Code == http.StatusForbidden {
		t.Errorf("owner should be able to assign owner role, got 403: %s", rec.Body.String())
	}
}

func TestUpdateMemberRole_RoleEscalation_NonOwnerCannotPromoteToOwner(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	caller := &auth.User{ID: "caller-1", Email: "admin@example.com"}
	session := &auth.Session{Role: "admin"}

	router := gin.New()
	router.PUT("/members/:user_id", injectTestOrgAccountWithSession(acct, caller, session), UpdateMemberRole(log, nil, nil, nil))

	body := `{"role": "owner"}`
	req := httptest.NewRequest(http.MethodPut, "/members/target-user", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("non-owner should not be able to promote to owner, expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateInvitations_RoleEscalation_NonOwnerCannotInviteAsOwner(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	caller := &auth.User{ID: "caller-1", Email: "admin@example.com"}
	session := &auth.Session{Role: "admin"}

	router := gin.New()
	router.POST("/invitations", injectTestOrgAccountWithSession(acct, caller, session), CreateInvitations(log, nil, nil))

	body := `{"invitations": [{"value": "invite@example.com", "kind": "email", "role": "owner"}]}`
	req := httptest.NewRequest(http.MethodPost, "/invitations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("non-owner should not be able to invite as owner, expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- Cross-account access test ---

func TestListMembers_CrossAccount_Denied(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	// ResolveAccount: return org-b
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("org-b").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name"}).
			AddRow("acct-b", "org-b", "organization", "org_b_wos", nil, time.Now(), time.Now(), ""))

	// RequireAccountPermission: IsMember returns 0 (user-a is not a member of org-b)
	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-b", "user-a").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-a"})
		c.Set(string(auth.SessionContextKey), &auth.Session{OrganizationID: ""})
		c.Next()
	})
	memberRoutes := router.Group("/accounts/:account/members")
	memberRoutes.Use(middleware.ResolveAccount(store))
	memberRoutes.Use(middleware.RequireAccountPermission(store, "org:manage"))
	memberRoutes.GET("", ListMembers(log, store, nil))

	req := httptest.NewRequest(http.MethodGet, "/accounts/org-b/members", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-account access should be denied, expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
