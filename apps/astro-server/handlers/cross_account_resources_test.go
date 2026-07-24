package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/metricsstore"
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
		if len(selected) != 3 {
			t.Fatalf("selected %d memberships, want 3", len(selected))
		}
		if len(rejected) != 0 {
			t.Fatalf("rejected memberships = %#v, want none", rejected)
		}
	})

	t.Run("requested order with duplicates removed", func(t *testing.T) {
		selected, rejected := selectCrossAccountMemberships(
			memberships,
			[]string{" gamma ", "alpha", "gamma"},
		)
		if len(selected) != 2 || selected[0].Name != "gamma" || selected[1].Name != "alpha" {
			t.Fatalf("selected memberships = %#v, want gamma then alpha", selected)
		}
		if len(rejected) != 0 {
			t.Fatalf("rejected memberships = %#v, want none", rejected)
		}
	})

	t.Run("foreign account becomes an account-level failure", func(t *testing.T) {
		selected, rejected := selectCrossAccountMemberships(
			memberships,
			[]string{"alpha", "foreign"},
		)
		if len(selected) != 1 || selected[0].Name != "alpha" {
			t.Fatalf("selected memberships = %#v, want [alpha]", selected)
		}
		if len(rejected) != 1 || rejected[0] != "foreign" {
			t.Fatalf("rejected memberships = %#v, want [foreign]", rejected)
		}
	})
}

func TestCrossAccountResourceResultSerializesTerminalPageMetadata(t *testing.T) {
	body, err := json.Marshal(CrossAccountResourceResult[[]string]{
		Account: "alpha",
		Data:    []string{},
	})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	for _, field := range []string{"count", "limit", "offset", "has_more"} {
		if _, ok := result[field]; !ok {
			t.Fatalf("%s missing from terminal page response: %s", field, body)
		}
	}
	if string(result["has_more"]) != "false" {
		t.Fatalf("has_more = %s, want false", result["has_more"])
	}
}

func TestLoadCrossAccountResourcesBoundsConcurrencyAndPreservesFailures(t *testing.T) {
	const accountCount = 15
	memberships := make([]account.AccountWithRole, accountCount)
	for i := range memberships {
		memberships[i] = account.AccountWithRole{
			ID:   fmt.Sprintf("acct-%02d", i),
			Name: fmt.Sprintf("account-%02d", i),
		}
	}

	started := make(chan struct{}, accountCount)
	release := make(chan struct{})
	type loadResult struct {
		results []CrossAccountResourceResult[string]
		failed  []string
	}
	done := make(chan loadResult, 1)

	go func() {
		results, failed := loadCrossAccountResources(
			context.Background(),
			logger.New("error", "json"),
			"test",
			memberships,
			func(_ context.Context, membership account.AccountWithRole) (crossAccountResourcePage[string], error) {
				started <- struct{}{}
				<-release
				if membership.Name == "account-08" {
					return crossAccountResourcePage[string]{}, errors.New("unavailable")
				}
				return crossAccountResourcePage[string]{data: membership.ID}, nil
			},
		)
		done <- loadResult{results: results, failed: failed}
	}()

	for i := 0; i < crossAccountResourceConcurrency; i++ {
		<-started
	}
	select {
	case <-started:
		t.Fatalf("more than %d account loads started concurrently", crossAccountResourceConcurrency)
	default:
	}
	close(release)

	loaded := <-done
	if len(loaded.results) != accountCount-1 {
		t.Fatalf("successful results = %d, want %d", len(loaded.results), accountCount-1)
	}
	if len(loaded.failed) != 1 || loaded.failed[0] != "account-08" {
		t.Fatalf("failed accounts = %#v, want [account-08]", loaded.failed)
	}
	for i := 1; i < len(loaded.results); i++ {
		if loaded.results[i-1].Account > loaded.results[i].Account {
			t.Fatalf("results are not in membership order: %#v", loaded.results)
		}
	}
}

var crossAccountKnowledgeColumns = []string{
	"id", "account_id", "name", "arn", "provider", "mode", "status", "storage",
	"storage_class", "public", "public_host", "encrypted_data_key", "kms_key_arn",
	"error", "created_at", "updated_at", "count",
}

func setupCrossAccountKnowledgeRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock) {
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
	knowledgeStore := knowledgestore.NewStore(db)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET(
		"/api/v1/me/knowledge",
		ListCrossAccountKnowledgeStores(logger.New("error", "json"), accountStore, knowledgeStore),
	)
	return router, mock
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

func TestListCrossAccountKnowledgeStoresPreservesPartialFailure(t *testing.T) {
	router, mock := setupCrossAccountKnowledgeRouter(t)
	expectCrossAccountMemberships(mock)
	mock.ExpectQuery(`FROM knowledge_stores WHERE account_id`).
		WithArgs("acct-1", defaultBlueprintListLimit, 0).
		WillReturnRows(sqlmock.NewRows(crossAccountKnowledgeColumns))
	mock.ExpectQuery(`FROM knowledge_stores WHERE account_id`).
		WithArgs("acct-2", defaultBlueprintListLimit, 0).
		WillReturnError(errors.New("database unavailable"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/knowledge", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response CrossAccountKnowledgeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].Account != "alpha" {
		t.Fatalf("results = %#v, want only alpha", response.Results)
	}
	if response.Results[0].Data == nil {
		t.Fatal("empty successful account encoded as null, want []")
	}
	if len(response.FailedAccounts) != 1 || response.FailedAccounts[0] != "beta" {
		t.Fatalf("failed accounts = %#v, want [beta]", response.FailedAccounts)
	}
}

func TestListCrossAccountKnowledgeStoresSupportsTargetedRetry(t *testing.T) {
	router, mock := setupCrossAccountKnowledgeRouter(t)
	expectCrossAccountMemberships(mock)
	mock.ExpectQuery(`FROM knowledge_stores WHERE account_id`).
		WithArgs("acct-2", defaultBlueprintListLimit, 0).
		WillReturnRows(sqlmock.NewRows(crossAccountKnowledgeColumns))

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/me/knowledge?account=beta",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response CrossAccountKnowledgeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].Account != "beta" {
		t.Fatalf("results = %#v, want only beta", response.Results)
	}
}

func TestListCrossAccountKnowledgeStoresReturnsPaginationMetadata(t *testing.T) {
	router, mock := setupCrossAccountKnowledgeRouter(t)
	expectCrossAccountMemberships(mock)
	now := time.Now()
	mock.ExpectQuery(`FROM knowledge_stores WHERE account_id`).
		WithArgs("acct-1", 1, 0).
		WillReturnRows(sqlmock.NewRows(crossAccountKnowledgeColumns).AddRow(
			"store-1", "acct-1", "primary", "", "postgres", "managed", "ready", "10Gi",
			nil, false, nil, nil, nil, nil, now, now, 2,
		))

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/me/knowledge?account=alpha&limit=1",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response CrossAccountKnowledgeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("results = %#v, want one account", response.Results)
	}
	result := response.Results[0]
	if result.Count != 2 || result.Limit != 1 || result.Offset != 0 || !result.HasMore {
		t.Fatalf("pagination = %#v, want first page of two stores", result)
	}
	if len(result.Data) != 1 || result.Data[0].Name != "primary" {
		t.Fatalf("stores = %#v, want primary", result.Data)
	}
}

func TestListCrossAccountKnowledgeStoresReportsForeignAccountRejection(t *testing.T) {
	router, mock := setupCrossAccountKnowledgeRouter(t)
	expectCrossAccountMemberships(mock)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/me/knowledge?account=foreign",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response CrossAccountKnowledgeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Results) != 0 {
		t.Fatalf("results = %#v, want none", response.Results)
	}
	if len(response.FailedAccounts) != 0 {
		t.Fatalf("failed accounts = %#v, want none", response.FailedAccounts)
	}
	if len(response.RejectedAccounts) != 1 || response.RejectedAccounts[0] != "foreign" {
		t.Fatalf("rejected accounts = %#v, want [foreign]", response.RejectedAccounts)
	}
}

var crossAccountBlueprintColumns = []string{
	"account_id", "name", "registry", "visibility", "avatar_colors",
	"avatar_updated_at", "created_at", "updated_at", "build_id",
	"ecr_namespace", "spec_json", "readme", "agent_card_json",
	"validation_warnings", "published_at", "version_updated_at",
	"commit_message", "commit_sha", "repo_full_name", "version_count", "total_count",
}

func setupCrossAccountBlueprintRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock) {
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
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET(
		"/api/v1/me/blueprints",
		ListCrossAccountBlueprints(
			logger.New("error", "json"),
			agentindex.NewIndexWithDB(db),
			accountStore,
			heartstore.New(db),
			metricsstore.New(db),
			deploymentstore.NewStore(db),
			nil,
			nil,
			nil,
		),
	)
	return router, mock
}

func TestListCrossAccountBlueprintsReturnsBoundedPageAndPartialFailures(t *testing.T) {
	router, mock := setupCrossAccountBlueprintRouter(t)
	expectCrossAccountMemberships(mock)

	now := time.Now()
	rows := sqlmock.NewRows(crossAccountBlueprintColumns)
	for i := 0; i < defaultBlueprintListLimit; i++ {
		rows.AddRow(
			"acct-1",
			fmt.Sprintf("blueprint-%03d", i),
			"registry.example",
			"private",
			nil,
			nil,
			now,
			now,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			0,
			101,
		)
	}
	mock.ExpectQuery(`FROM agents a`).
		WithArgs("acct-1", defaultBlueprintListLimit, 0).
		WillReturnRows(rows)
	mock.ExpectQuery(`FROM agents a`).
		WithArgs("acct-2", defaultBlueprintListLimit, 0).
		WillReturnError(errors.New("database unavailable"))
	mock.ExpectQuery(`FROM agent_hearts`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"agent_name", "count"}))
	mock.ExpectQuery(`FROM agent_message_counts`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"agent_name", "count"}))
	mock.ExpectQuery(`FROM deployments`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"agent_name", "count"}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/blueprints", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response CrossAccountBlueprintsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].Account != "alpha" {
		t.Fatalf("results = %#v, want only alpha", response.Results)
	}
	got := response.Results[0].Data
	if got.Agents == nil || len(got.Agents) != defaultBlueprintListLimit {
		t.Fatalf("agents = %#v, want %d non-null agents", got.Agents, defaultBlueprintListLimit)
	}
	if got.Count != 101 || got.Limit != defaultBlueprintListLimit || got.Offset != 0 || !got.HasMore {
		t.Fatalf("envelope = %#v, want bounded first page of 101 items", got)
	}
	if len(response.FailedAccounts) != 1 || response.FailedAccounts[0] != "beta" {
		t.Fatalf("failed accounts = %#v, want [beta]", response.FailedAccounts)
	}
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

func setupCrossAccountDeploymentRouter(
	t *testing.T,
	cache *recordingCache,
) (*gin.Engine, sqlmock.Sqlmock) {
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
	router.GET(
		"/api/v1/me/deployments",
		ListCrossAccountDeployments(
			log,
			accountStore,
			deployStore,
			agentIndex,
			nil,
			nil,
			cache,
		),
	)
	router.GET(
		"/api/v1/deployments",
		ListDeployments(log, accountStore, deployStore, agentIndex, nil, nil, cache),
	)
	return router, mock
}

func TestListCrossAccountDeploymentsUsesCacheAndPreservesPartialFailures(t *testing.T) {
	cached, err := json.Marshal(ListDeploymentsResponse{
		Deployments: []AgentDeploymentSummary{
			{ID: "deployment-1", Name: "first"},
			{ID: "deployment-2", Name: "second"},
		},
		Count: 2,
	})
	if err != nil {
		t.Fatalf("marshal cache fixture: %v", err)
	}
	cache := &recordingCache{
		entries: map[string][]byte{deploycache.KeyFor("acct-1"): cached},
	}
	router, mock := setupCrossAccountDeploymentRouter(t, cache)
	expectCrossAccountMemberships(mock)
	mock.ExpectQuery(`FROM deployments`).
		WithArgs("acct-2", 1, 1).
		WillReturnError(errors.New("database unavailable"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/deployments?limit=1&offset=1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response CrossAccountDeploymentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].Account != "alpha" {
		t.Fatalf("results = %#v, want cached alpha result", response.Results)
	}
	result := response.Results[0]
	if len(result.Data.Deployments) != 1 || result.Data.Deployments[0].ID != "deployment-2" {
		t.Fatalf("deployments = %#v, want second cached deployment", result.Data.Deployments)
	}
	if result.Count != 2 || result.Data.Count != 2 || result.Limit != 1 || result.Offset != 1 || result.HasMore {
		t.Fatalf("page metadata = %#v, want final one-item page of two deployments", result)
	}
	if len(response.FailedAccounts) != 1 || response.FailedAccounts[0] != "beta" {
		t.Fatalf("failed accounts = %#v, want [beta]", response.FailedAccounts)
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.gets) != 2 {
		t.Fatalf("cache reads = %#v, want one per account", cache.gets)
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
	cache := &recordingCache{
		entries: map[string][]byte{deploycache.KeyFor("acct-1"): cached},
	}
	router, mock := setupCrossAccountDeploymentRouter(t, cache)
	expectDeploymentAccountAccess(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments?account=alpha", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != string(cached) {
		t.Fatalf("body = %q, want exact cached bytes %q", rec.Body.String(), cached)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func TestListDeploymentsWritesCacheMissesAsCachedBytes(t *testing.T) {
	cache := &recordingCache{entries: make(map[string][]byte)}
	router, mock := setupCrossAccountDeploymentRouter(t, cache)
	expectDeploymentAccountAccess(mock)
	mock.ExpectQuery(`FROM deployments`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments?account=alpha", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	cache.mu.Lock()
	cached := cache.entries[deploycache.KeyFor("acct-1")]
	cache.mu.Unlock()
	if len(cached) == 0 {
		t.Fatal("cache miss response was not cached")
	}
	if rec.Body.String() != string(cached) {
		t.Fatalf("body = %q, want exact cached bytes %q", rec.Body.String(), cached)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}
