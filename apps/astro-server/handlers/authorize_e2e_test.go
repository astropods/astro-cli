package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/authorizationstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

// authorizeFixture wires a router that bypasses RequireDeployToken and stamps
// a deployment_id directly into the gin context. Lets us test the handler
// against sqlmock without the full deploy-token plumbing — token tests live
// in the deploytoken package.
type authorizeFixture struct {
	router *gin.Engine
	store  *authorizationstore.Store
	mock   sqlmock.Sqlmock
	db     *sql.DB
}

func newAuthorizeFixture(t *testing.T, deploymentID string) *authorizeFixture {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	store := authorizationstore.NewStore(db)
	log := logger.New("error", "text")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("deploy_token_deployment_id", deploymentID)
		c.Next()
	})
	r.GET("/authorize", CheckDeploymentAuthorization(log, store))
	return &authorizeFixture{router: r, store: store, mock: mock, db: db}
}

func (f *authorizeFixture) close() { _ = f.db.Close() }

func (f *authorizeFixture) call(t *testing.T, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/authorize?"+query, nil)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

func decodeAllowed(t *testing.T, w *httptest.ResponseRecorder) bool {
	t.Helper()
	var body struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	return body.Allowed
}

// A1: account grant matches a member of that account.
func TestAuthorize_AccountGrantMatch(t *testing.T) {
	f := newAuthorizeFixture(t, "dep-1")
	defer f.close()

	// anyone short-circuit miss
	f.mock.ExpectQuery("\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1 AND adapter = $2 AND subject_type = 'anyone'\n\t\tLIMIT 1\n\t").
		WithArgs("dep-1", "web").
		WillReturnError(sql.ErrNoRows)
	// account_members lookup for alice
	f.mock.ExpectQuery("\n\t\tSELECT account_id FROM account_members WHERE user_id = $1\n\t").
		WithArgs("alice").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct-Acme"))
	// grant lookup hits
	f.mock.ExpectQuery("\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1\n\t\t  AND adapter = $2\n\t\t  AND (\n\t\t    (subject_type = 'account' AND subject_id = ANY($3))\n\t\t    OR\n\t\t    (subject_type = 'user' AND subject_id = ANY($4))\n\t\t  )\n\t\tLIMIT 1\n\t").
		WithArgs("dep-1", "web", pq.Array([]string{"acct-Acme"}), pq.Array([]string{"alice"})).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))

	w := f.call(t, "identity_type=user&identity_id=alice&adapter=web")
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	if !decodeAllowed(t, w) {
		t.Fatal("expected allowed=true")
	}
}

// A2: non-member denied.
func TestAuthorize_AccountGrantNonMember(t *testing.T) {
	f := newAuthorizeFixture(t, "dep-1")
	defer f.close()

	f.mock.ExpectQuery("\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1 AND adapter = $2 AND subject_type = 'anyone'\n\t\tLIMIT 1\n\t").
		WithArgs("dep-1", "web").
		WillReturnError(sql.ErrNoRows)
	f.mock.ExpectQuery("\n\t\tSELECT account_id FROM account_members WHERE user_id = $1\n\t").
		WithArgs("bob").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"})) // bob is in no accounts
	f.mock.ExpectQuery("\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1\n\t\t  AND adapter = $2\n\t\t  AND (\n\t\t    (subject_type = 'account' AND subject_id = ANY($3))\n\t\t    OR\n\t\t    (subject_type = 'user' AND subject_id = ANY($4))\n\t\t  )\n\t\tLIMIT 1\n\t").
		WithArgs("dep-1", "web", pq.Array([]string(nil)), pq.Array([]string{"bob"})).
		WillReturnError(sql.ErrNoRows)

	w := f.call(t, "identity_type=user&identity_id=bob&adapter=web")
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	if decodeAllowed(t, w) {
		t.Fatal("expected allowed=false")
	}
}

// A6: anyone grant + authenticated user → allowed via short-circuit (no
// principal resolution needed, no account_members query).
func TestAuthorize_AnyoneAuthenticated(t *testing.T) {
	f := newAuthorizeFixture(t, "dep-1")
	defer f.close()

	f.mock.ExpectQuery("\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1 AND adapter = $2 AND subject_type = 'anyone'\n\t\tLIMIT 1\n\t").
		WithArgs("dep-1", "web").
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))

	w := f.call(t, "identity_type=user&identity_id=alice&adapter=web")
	if !decodeAllowed(t, w) {
		t.Fatal("expected allowed=true via anyone")
	}
}

// A7: anyone grant + anonymous (empty identity) → allowed.
func TestAuthorize_AnyoneAnonymous(t *testing.T) {
	f := newAuthorizeFixture(t, "dep-1")
	defer f.close()

	f.mock.ExpectQuery("\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1 AND adapter = $2 AND subject_type = 'anyone'\n\t\tLIMIT 1\n\t").
		WithArgs("dep-1", "web").
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))

	w := f.call(t, "adapter=web")
	if !decodeAllowed(t, w) {
		t.Fatal("expected allowed=true for anonymous + anyone grant")
	}
}

// A20: empty identity, no anyone grant → denied without errors.
func TestAuthorize_AnonymousNoAnyone(t *testing.T) {
	f := newAuthorizeFixture(t, "dep-1")
	defer f.close()

	f.mock.ExpectQuery("\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1 AND adapter = $2 AND subject_type = 'anyone'\n\t\tLIMIT 1\n\t").
		WithArgs("dep-1", "web").
		WillReturnError(sql.ErrNoRows)

	w := f.call(t, "adapter=web")
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	if decodeAllowed(t, w) {
		t.Fatal("expected allowed=false")
	}
}

// A13: slack with bot account grant → allowed.
func TestAuthorize_SlackBotAccountGrant(t *testing.T) {
	f := newAuthorizeFixture(t, "dep-1")
	defer f.close()

	f.mock.ExpectQuery("\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1 AND adapter = $2 AND subject_type = 'anyone'\n\t\tLIMIT 1\n\t").
		WithArgs("dep-1", "slack").
		WillReturnError(sql.ErrNoRows)
	f.mock.ExpectQuery("\n\t\tSELECT account_id FROM deployments WHERE id = $1\n\t").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct-D"))
	f.mock.ExpectQuery("\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1\n\t\t  AND adapter = $2\n\t\t  AND (\n\t\t    (subject_type = 'account' AND subject_id = ANY($3))\n\t\t    OR\n\t\t    (subject_type = 'user' AND subject_id = ANY($4))\n\t\t  )\n\t\tLIMIT 1\n\t").
		WithArgs("dep-1", "slack", pq.Array([]string{"acct-D"}), pq.Array([]string(nil))).
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))

	w := f.call(t, "identity_type=slack&identity_id=U123&adapter=slack")
	if !decodeAllowed(t, w) {
		t.Fatal("expected allowed=true for slack bot account grant")
	}
}

// A14: slack, no grant → denied.
func TestAuthorize_SlackNoGrant(t *testing.T) {
	f := newAuthorizeFixture(t, "dep-1")
	defer f.close()

	f.mock.ExpectQuery("\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1 AND adapter = $2 AND subject_type = 'anyone'\n\t\tLIMIT 1\n\t").
		WithArgs("dep-1", "slack").
		WillReturnError(sql.ErrNoRows)
	f.mock.ExpectQuery("\n\t\tSELECT account_id FROM deployments WHERE id = $1\n\t").
		WithArgs("dep-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct-D"))
	f.mock.ExpectQuery("\n\t\tSELECT 1 FROM deployment_authorization_grants\n\t\tWHERE deployment_id = $1\n\t\t  AND adapter = $2\n\t\t  AND (\n\t\t    (subject_type = 'account' AND subject_id = ANY($3))\n\t\t    OR\n\t\t    (subject_type = 'user' AND subject_id = ANY($4))\n\t\t  )\n\t\tLIMIT 1\n\t").
		WithArgs("dep-1", "slack", pq.Array([]string{"acct-D"}), pq.Array([]string(nil))).
		WillReturnError(sql.ErrNoRows)

	w := f.call(t, "identity_type=slack&identity_id=U123&adapter=slack")
	if decodeAllowed(t, w) {
		t.Fatal("expected allowed=false")
	}
}

// 400 when adapter is unknown.
func TestAuthorize_UnknownAdapter(t *testing.T) {
	f := newAuthorizeFixture(t, "dep-1")
	defer f.close()

	w := f.call(t, "identity_type=user&identity_id=alice&adapter=discord")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// 400 when identity_type is set without identity_id (or vice versa).
func TestAuthorize_PartialIdentity(t *testing.T) {
	f := newAuthorizeFixture(t, "dep-1")
	defer f.close()

	w := f.call(t, "identity_type=user&adapter=web")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// 401 when deployment_id is missing (RequireDeployToken would normally
// short-circuit; we test the handler's own guard).
func TestAuthorize_MissingDeploymentID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _, _ := sqlmock.New()
	defer db.Close()
	store := authorizationstore.NewStore(db)
	log := logger.New("error", "text")

	r := gin.New()
	r.GET("/authorize", CheckDeploymentAuthorization(log, store))

	req := httptest.NewRequest(http.MethodGet, "/authorize?adapter=web", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

var _ = middleware.DeploymentIDFromContext // keep middleware import used
