package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/auth"
)

// --- ResolveAccount tests ---

func TestResolveAccount_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	mock.ExpectQuery("SELECT .+ FROM accounts WHERE name").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "created_at", "updated_at"}).
			AddRow("acct-1", "myorg", "organization", "org_123", time.Now(), time.Now()))

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

	mock.ExpectQuery("SELECT .+ FROM accounts WHERE name").
		WithArgs("unknown").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "created_at", "updated_at"}))

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

func TestRequireAccountPermission_PersonalAccount_Owner(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	// Owner check: HasRole query
	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1", pq.Array([]string{"owner"})).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router := setupPermissionTestRouter(store, "org:admin",
		&auth.User{ID: "user-1"}, nil,
		&account.Account{ID: "acct-1", Name: "personal", Type: "personal"})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("personal owner should have all permissions, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequireAccountPermission_PersonalAccount_NotOwner(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)

	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-2", pq.Array([]string{"owner"})).
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
			Permissions:    []string{"agents:read", "agents:write", "agents:deploy"},
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
			Permissions:    []string{"agents:read", "agents:deploy"},
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
