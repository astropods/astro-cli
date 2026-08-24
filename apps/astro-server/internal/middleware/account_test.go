package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/gin-gonic/gin"
)

// --- ResolveAccount tests ---

func TestResolveAccount_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("acct-1", "myorg", "organization", "org_123", nil, time.Now(), time.Now(), "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	router := gin.New()
	router.GET("/accounts/:account", ResolveAccount(store), func(c *gin.Context) {
		acct, ok := GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no account"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": acct.ID, "name": acct.Name, "workos_org_id": acct.WorkOSOrganizationID})
	})

	req := httptest.NewRequest(http.MethodGet, "/accounts/myorg", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResolveAccount_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("unknown").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}))

	router := gin.New()
	router.GET("/accounts/:account", ResolveAccount(store), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/accounts/unknown", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- RequireAccountPermission tests ---

// helper: injects user, session, and account into gin context
func setupPermissionTestRouter(
	store *account.AccountStore,
	permission string,
	user *auth.User,
	session *auth.Session,
	acct *account.Account,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		if user != nil {
			c.Set(string(auth.UserContextKey), user)
		}
		if session != nil {
			c.Set(string(auth.SessionContextKey), session)
		}
		if acct != nil {
			c.Set(string(auth.AccountContextKey), acct)
		}
		c.Next()
	}, RequireAccountPermission(store, permission), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "allowed"})
	})
	return router
}

func TestRequireAccountPermission_NoUser(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	router := setupPermissionTestRouter(store, "agents:read", nil, nil, &account.Account{
		ID: "acct-1", Name: "test", Type: "personal",
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAccountPermission_NoAccount(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	router := setupPermissionTestRouter(store, "agents:read",
		&auth.User{ID: "user-1"}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestRequireAccountPermission_PersonalAccount_Member(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	// IsMember check
	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router := setupPermissionTestRouter(store, "org:admin",
		&auth.User{ID: "user-1"}, nil,
		&account.Account{ID: "acct-1", Name: "personal", Type: "personal"})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("personal account member should have all permissions, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequireAccountPermission_PersonalAccount_NotMember(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	router := setupPermissionTestRouter(store, "agents:read",
		&auth.User{ID: "user-2"}, nil,
		&account.Account{ID: "acct-1", Name: "personal", Type: "personal"})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("non-owner of personal account should be forbidden, got %d", rec.Code)
	}
}

func TestRequireAccountPermission_OrgAccount_JWTPath_Granted(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	// Session JWT is scoped to the org and includes the required permission
	router := setupPermissionTestRouter(store, "agents:write",
		&auth.User{ID: "user-1"},
		&auth.Session{
			OrganizationID: "org_123",
			Permissions:    []string{"agents:read", "agents:write", "deployments:write"},
		},
		&account.Account{
			ID: "acct-1", Name: "myorg", Type: "organization",
			WorkOSOrganizationID: "org_123",
		})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("JWT with matching org and permission should be allowed, got %d", rec.Code)
	}
}

func TestRequireAccountPermission_OrgAccount_JWTPath_Denied(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	// Session JWT scoped to org but missing required permission
	router := setupPermissionTestRouter(store, "org:admin",
		&auth.User{ID: "user-1"},
		&auth.Session{
			OrganizationID: "org_123",
			Permissions:    []string{"agents:read", "deployments:write"},
		},
		&account.Account{
			ID: "acct-1", Name: "myorg", Type: "organization",
			WorkOSOrganizationID: "org_123",
		})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("JWT missing required permission should be forbidden, got %d", rec.Code)
	}
}

func TestRequireAccountPermission_OrgAccount_OrgMismatch_Rejected(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	// Session JWT scoped to a different org — should be rejected
	router := setupPermissionTestRouter(store, "agents:read",
		&auth.User{ID: "user-1"},
		&auth.Session{OrganizationID: "org_other"},
		&account.Account{
			ID: "acct-1", Name: "myorg", Type: "organization",
			WorkOSOrganizationID: "org_123",
		})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("mismatched org should be forbidden, got %d", rec.Code)
	}
}

// --- RequireAccountMember tests ---

func setupMemberTestRouter(
	store *account.AccountStore,
	user *auth.User,
	acct *account.Account,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		if user != nil {
			c.Set(string(auth.UserContextKey), user)
		}
		if acct != nil {
			c.Set(string(auth.AccountContextKey), acct)
		}
		c.Next()
	}, RequireAccountMember(store), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "allowed"})
	})
	return router
}

func TestRequireAccountMember_NoUser(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	router := setupMemberTestRouter(store, nil, &account.Account{
		ID: "acct-1", Name: "test", Type: "personal",
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAccountMember_NoAccount(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	router := setupMemberTestRouter(store, &auth.User{ID: "user-1"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestRequireAccountMember_IsMember(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router := setupMemberTestRouter(store,
		&auth.User{ID: "user-1"},
		&account.Account{ID: "acct-1", Name: "myorg", Type: "organization"})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("member should be allowed, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequireAccountMember_NotMember(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	router := setupMemberTestRouter(store,
		&auth.User{ID: "user-2"},
		&account.Account{ID: "acct-1", Name: "myorg", Type: "organization"})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("non-member should be forbidden, got %d", rec.Code)
	}
}

func TestRequireAccountPermission_OrgAccount_NoSession_Rejected(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	// No session at all
	router := setupPermissionTestRouter(store, "agents:read",
		&auth.User{ID: "user-1"},
		nil,
		&account.Account{
			ID: "acct-1", Name: "myorg", Type: "organization",
			WorkOSOrganizationID: "org_123",
		})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("no session should be forbidden, got %d", rec.Code)
	}
}

func setupOwnerTestRouter(store *account.AccountStore, user *auth.User, acct *account.Account) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		if user != nil {
			c.Set(string(auth.UserContextKey), user)
		}
		if acct != nil {
			c.Set(string(auth.AccountContextKey), acct)
		}
		c.Next()
	}, RequireAccountOwner(store), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return router
}

func TestRequireAccountOwner(t *testing.T) {
	cases := map[string]struct {
		owner    any
		callerID string
		want     int
	}{
		"the recorded owner passes":            {"user-1", "user-1", http.StatusOK},
		"another member is refused":            {"user-1", "user-2", http.StatusForbidden},
		"an unrecorded owner refuses everyone": {nil, "user-1", http.StatusForbidden},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			store := account.NewAccountStore(db)
			mock.ExpectQuery("SELECT owner_user_id FROM accounts").
				WithArgs("acct-1").
				WillReturnRows(sqlmock.NewRows([]string{"owner_user_id"}).AddRow(tc.owner))

			router := setupOwnerTestRouter(store,
				&auth.User{ID: tc.callerID},
				&account.Account{ID: "acct-1", Name: "myorg", Type: "organization"})

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))

			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestRequireAccountOwner_IgnoresTheRoleClaim(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	mock.ExpectQuery("SELECT owner_user_id FROM accounts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id"}).AddRow("user-1"))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-2"})
		c.Set(string(auth.SessionContextKey), &auth.Session{
			OrganizationID: "org_1", Role: "owner", Permissions: []string{"org:admin", "org:manage"},
		})
		c.Set(string(auth.AccountContextKey), &account.Account{ID: "acct-1", Type: "organization"})
		c.Next()
	}, RequireAccountOwner(store), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/test", nil))

	if rec.Code != http.StatusForbidden {
		t.Errorf("a WorkOS owner role must not stand in for the column, got %d: %s", rec.Code, rec.Body.String())
	}
}
