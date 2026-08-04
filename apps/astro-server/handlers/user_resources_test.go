package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func TestSelectCrossAccountMemberships(t *testing.T) {
	memberships := []account.AccountWithRole{
		{ID: "acct-1", Name: "alpha"},
		{ID: "acct-2", Name: "beta"},
		{ID: "acct-3", Name: "gamma"},
	}

	t.Run("all memberships when no filter is supplied", func(t *testing.T) {
		selected, rejected := selectCrossAccountMemberships(memberships, nil)
		if len(selected) != 3 || len(rejected) != 0 {
			t.Fatalf("selected = %#v, rejected = %#v", selected, rejected)
		}
	})

	t.Run("requested order with duplicates removed", func(t *testing.T) {
		selected, rejected := selectCrossAccountMemberships(memberships, []string{" gamma ", "alpha", "gamma"})
		if len(selected) != 2 || selected[0].Name != "gamma" || selected[1].Name != "alpha" {
			t.Fatalf("selected memberships = %#v, want gamma then alpha", selected)
		}
		if len(rejected) != 0 {
			t.Fatalf("rejected memberships = %#v, want none", rejected)
		}
	})

	t.Run("foreign account is rejected", func(t *testing.T) {
		selected, rejected := selectCrossAccountMemberships(memberships, []string{"alpha", "foreign"})
		if len(selected) != 1 || selected[0].Name != "alpha" {
			t.Fatalf("selected memberships = %#v, want alpha", selected)
		}
		if len(rejected) != 1 || rejected[0] != "foreign" {
			t.Fatalf("rejected memberships = %#v, want foreign", rejected)
		}
	})
}

func TestUserResourceCacheKeyIncludesScopeRequestAndGenerations(t *testing.T) {
	scope := userResourceScopeRequest{canonicalAccounts: []string{"alpha"}}
	identity := struct {
		Limit int `json:"limit"`
	}{Limit: 50}
	base, err := userResourceCacheKey("user-1", scope, identity, []string{"acct-1:generation-1"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{
		mustUserResourceCacheKey(t, "user-2", scope, identity, []string{"acct-1:generation-1"}),
		mustUserResourceCacheKey(t, "user-1", userResourceScopeRequest{canonicalAccounts: []string{"beta"}}, identity, []string{"acct-1:generation-1"}),
		mustUserResourceCacheKey(t, "user-1", scope, struct {
			Limit int `json:"limit"`
		}{Limit: 100}, []string{"acct-1:generation-1"}),
		mustUserResourceCacheKey(t, "user-1", scope, identity, []string{"acct-1:generation-2"}),
	}
	for _, candidate := range cases {
		if candidate == base {
			t.Fatal("cache identity omitted a user, scope, request, or generation dimension")
		}
	}
}

func TestUserResourceCacheKeyIgnoresRejectedAccounts(t *testing.T) {
	identity := struct {
		Limit int `json:"limit"`
	}{Limit: 50}
	first := mustUserResourceCacheKey(t, "user-1", userResourceScopeRequest{
		canonicalAccounts: []string{"alpha"},
		rejected:          []string{"foreign-a"},
	}, identity, []string{"acct-1:generation-1"})
	second := mustUserResourceCacheKey(t, "user-1", userResourceScopeRequest{
		canonicalAccounts: []string{"alpha"},
		rejected:          []string{"foreign-b"},
	}, identity, []string{"acct-1:generation-1"})
	if first != second {
		t.Fatal("rejected account names changed the shared cache key")
	}
}

func mustUserResourceCacheKey(
	t *testing.T,
	userID string,
	scope userResourceScopeRequest,
	identity any,
	generations []string,
) string {
	t.Helper()
	key, err := userResourceCacheKey(userID, scope, identity, generations)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func expectCrossAccountMemberships(mock sqlmock.Sqlmock) {
	now := time.Now()
	mock.ExpectQuery(`FROM accounts a`).
		WithArgs("user-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "type", "workos_org_id", "cluster_id", "created_at",
			"updated_at", "display_name", "avatar_updated_at",
		}).
			AddRow("acct-1", "alpha", "organization", "", "", now, now, "Alpha", nil).
			AddRow("acct-2", "beta", "organization", "", "", now, now, "Beta", nil))
}

type recordingCache struct {
	mu      sync.Mutex
	entries map[string][]byte
	gets    []string
}

func (c *recordingCache) Get(_ context.Context, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gets = append(c.gets, key)
	value, ok := c.entries[key]
	return value, ok
}

func (c *recordingCache) Set(_ context.Context, key string, data []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = data
	return nil
}

func (c *recordingCache) Invalidate(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
	return nil
}

func setupCrossAccountDeploymentRouter(t *testing.T, cache *recordingCache) (*gin.Engine, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
		_ = db.Close()
	})
	mock.MatchExpectationsInOrder(false)

	accountStore := account.NewAccountStore(db)
	deployStore := deploymentstore.NewStore(db)
	agentIndex := agentindex.NewIndexWithDB(db)
	log := logger.New("error", "json")
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/me/deployments", ListUserDeployments(log, accountStore, deployStore, agentIndex, nil, cache))
	router.GET("/api/v1/deployments", ListDeployments(log, accountStore, deployStore, agentIndex, nil, nil, cache))
	return router, mock
}

func TestUserResourceListRejectsTooManyAccountParameters(t *testing.T) {
	cache := &recordingCache{entries: map[string][]byte{}}
	router, mock := setupCrossAccountDeploymentRouter(t, cache)
	expectCrossAccountMemberships(mock)
	query := url.Values{}
	for range maxUserResourceAccountParams + 1 {
		query.Add("account", "foreign")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/deployments?"+query.Encode(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestParseUserResourceScopeAllKeepsLargeMembershipSet(t *testing.T) {
	memberships := make([]account.AccountWithRole, maxUserResourceAccountParams+1)
	for i := range memberships {
		memberships[i] = account.AccountWithRole{
			ID:   fmt.Sprintf("acct-%d", i),
			Name: fmt.Sprintf("account-%d", i),
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?scope=all", nil)

	scope, err := parseUserResourceScope(c, memberships)
	if err != nil {
		t.Fatalf("parseUserResourceScope: %v", err)
	}
	if !scope.all || len(scope.selected) != len(memberships) {
		t.Fatalf("scope = %#v, want all %d memberships", scope, len(memberships))
	}
}

func expectDeploymentAccountAccess(mock sqlmock.Sqlmock) {
	now := time.Now()
	mock.ExpectQuery(`FROM accounts a`).
		WithArgs("alpha").
		WillReturnRows(sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow("acct-1", "alpha", "organization", nil, nil, now, now)...))
	mock.ExpectQuery(`FROM account_members`).
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
}

func TestListDeploymentsWritesCacheHitsWithoutReencoding(t *testing.T) {
	cached := []byte(`{"deployments":[],"count":0,"raw_cache_marker":true}`)
	cache := &recordingCache{entries: map[string][]byte{deploycache.KeyFor("acct-1"): cached}}
	router, mock := setupCrossAccountDeploymentRouter(t, cache)
	expectDeploymentAccountAccess(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments?account=alpha", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != string(cached) {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func TestListDeploymentsWritesCacheMissesAsCachedBytes(t *testing.T) {
	cache := &recordingCache{entries: make(map[string][]byte)}
	router, mock := setupCrossAccountDeploymentRouter(t, cache)
	expectDeploymentAccountAccess(mock)
	mock.ExpectQuery(`FROM deployments`).WithArgs("acct-1").WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments?account=alpha", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	cache.mu.Lock()
	cached := cache.entries[deploycache.KeyFor("acct-1")]
	cache.mu.Unlock()
	if len(cached) == 0 || rec.Body.String() != string(cached) {
		t.Fatalf("response was not written as the cached bytes")
	}
}
