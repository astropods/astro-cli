package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func setupUserResourceRouter(t *testing.T) (*gin.Engine, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet SQL expectations: %v", err)
		}
		_ = db.Close()
	})
	mock.MatchExpectationsInOrder(false)
	cache := &recordingCache{entries: make(map[string][]byte)}
	accounts := account.NewAccountStore(db)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/me/blueprints", ListUserBlueprints(
		logger.New("error", "json"), agentindex.NewIndexWithDB(db), accounts,
		avatar.NewStore(nil, "https://assets.example.com"), cache,
	))
	router.GET("/me/knowledge", ListUserKnowledgeStores(logger.New("error", "json"), accounts, knowledgestore.NewStore(db), cache))
	return router, mock
}

func TestListUserBlueprintsUsesOneGlobalPageAndOneMetadataBatch(t *testing.T) {
	router, mock := setupUserResourceRouter(t)
	expectCrossAccountMemberships(mock)
	now := time.Now()
	mock.ExpectQuery(`FROM agents a`).
		WithArgs("user-1", sqlmock.AnyArg(), 2).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "name", "registry", "visibility", "name_reserved",
			"avatar_colors", "avatar_updated_at", "account_name",
			"build_id", "spec_json", "readme", "agent_card_json", "validation_warnings",
			"published_at", "commit_message", "commit_sha", "repo_full_name",
		}).AddRow(
			"acct-1", "alpha-agent", "registry", "private", false,
			[]byte(`{"primary":"#fff"}`), now, "alpha",
			nil, nil, nil, nil, nil, nil, nil, nil, nil,
		))
	mock.ExpectQuery(`WITH refs AS`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "name", "heart_count", "lifetime_messages", "deploy_count", "publishers",
		}).AddRow("acct-1", "alpha-agent", 7, 12, 2, []byte(`[{"name":"Ada","account":"ada"}]`)))

	req := httptest.NewRequest(http.MethodGet, "/me/blueprints?scope=all&limit=1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var response UserBlueprintsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Blueprints) != 1 || response.Blueprints[0].Account != "alpha" {
		t.Fatalf("blueprints = %#v", response.Blueprints)
	}
	if response.Blueprints[0].HeartCount != 7 || response.Blueprints[0].Metrics.LifetimeMessages != 12 {
		t.Fatalf("metadata = %#v", response.Blueprints[0])
	}
	wantAvatarURL := "https://assets.example.com/avatars/agents/alpha/alpha-agent.jpg?v=" + fmt.Sprint(now.Unix())
	if response.Blueprints[0].AvatarURL != wantAvatarURL {
		t.Fatalf("avatar_url = %q, want %q", response.Blueprints[0].AvatarURL, wantAvatarURL)
	}
	if !response.Scope.All || len(response.Scope.Accounts) != 2 {
		t.Fatalf("scope = %#v", response.Scope)
	}
	if got := rec.Header().Get("X-Astro-Cache"); got != "miss" {
		t.Fatalf("cache = %q, want miss", got)
	}
}

func TestListUserKnowledgeStoresUsesOneGlobalKeysetPage(t *testing.T) {
	router, mock := setupUserResourceRouter(t)
	expectCrossAccountMemberships(mock)
	now := time.Now()
	mock.ExpectQuery(`FROM knowledge_stores ks`).
		WithArgs("user-1", sqlmock.AnyArg(), 2).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "name", "arn", "provider", "mode", "status",
			"error", "annotations",
			"created_at", "updated_at", "account_name",
		}).
			AddRow("store-2", "acct-2", "second", "arn:2", "postgres", "external", "ready", nil, nil, now, now, "beta").
			AddRow("store-1", "acct-1", "first", "arn:1", "postgres", "external", "ready", nil, nil, now.Add(-time.Minute), now, "alpha"))

	req := httptest.NewRequest(http.MethodGet, "/me/knowledge?scope=all&limit=1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var response UserKnowledgeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Stores) != 1 || response.Stores[0].Account != "beta" || response.Stores[0].AccountID != "acct-2" {
		t.Fatalf("stores = %#v", response.Stores)
	}
	if response.Page.NextCursor == "" {
		t.Fatalf("page = %#v, want continuation", response.Page)
	}
}

func TestListUserKnowledgeStoresSearchesBeforePagination(t *testing.T) {
	router, mock := setupUserResourceRouter(t)
	expectCrossAccountMemberships(mock)
	mock.ExpectQuery(`(?s)FROM knowledge_stores ks.*strpos\(lower\(ks.name\), lower\(\$3\)\).*ORDER BY ks.created_at DESC, ks.id DESC.*LIMIT \$4`).
		WithArgs("user-1", sqlmock.AnyArg(), "second", 51).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "name", "arn", "provider", "mode", "status",
			"error", "annotations",
			"created_at", "updated_at", "account_name",
		}))

	req := httptest.NewRequest(http.MethodGet, "/me/knowledge?scope=all&q=%20SeCoNd%20", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListUserKnowledgeStoresSearchCursorUsesDistinctPlaceholders(t *testing.T) {
	router, mock := setupUserResourceRouter(t)
	expectCrossAccountMemberships(mock)
	cursorTime := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	cursor, err := encodeUserKnowledgeCursor(&knowledgestore.KnowledgeStore{
		ID:        "store-1",
		CreatedAt: cursorTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`(?s)FROM knowledge_stores ks.*strpos\(lower\(ks.name\), lower\(\$3\)\).*\(ks.created_at, ks.id\) < \(\$4, \$5\).*LIMIT \$6`).
		WithArgs("user-1", sqlmock.AnyArg(), "memory", cursorTime, "store-1", 51).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "name", "arn", "provider", "mode", "status",
			"error", "annotations",
			"created_at", "updated_at", "account_name",
		}))

	req := httptest.NewRequest(http.MethodGet, "/me/knowledge?scope=all&q=memory&cursor="+cursor, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUserKnowledgeSearchHasABoundedLength(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/?account=alpha&q="+strings.Repeat("x", maxListQueryLen+1),
		nil,
	)
	_, err := parseUserKnowledgeRequest(c, []account.AccountWithRole{{ID: "acct-1", Name: "alpha"}})
	if err == nil {
		t.Fatal("overlong knowledge search should be rejected")
	}
}

func TestUserBlueprintFiltersNormalizeCase(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?account=alpha&q=SuPpOrT&tag=ToOl", nil)

	request, err := parseUserBlueprintRequest(c, []account.AccountWithRole{{ID: "acct-1", Name: "alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	if request.filters.Query != "support" || request.filters.Tag != "tool" {
		t.Fatalf("filters = %#v, want lowercased query and tag", request.filters)
	}
}

func TestUserResourceListsRequireExplicitScope(t *testing.T) {
	router, mock := setupUserResourceRouter(t)
	expectCrossAccountMemberships(mock)
	req := httptest.NewRequest(http.MethodGet, "/me/knowledge", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestUserBlueprintCursorIsBoundToSort(t *testing.T) {
	now := time.Now()
	encoded, err := encodeUserBlueprintCursor(agentindex.UserBlueprint{
		Agent: &agentindex.Agent{AccountID: "acct-1", Name: "agent"}, PublishedAt: &now,
	}, "newest")
	if err != nil {
		t.Fatal(err)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?account=alpha&sort=name&cursor="+encoded, nil)
	_, err = parseUserBlueprintRequest(c, []account.AccountWithRole{{ID: "acct-1", Name: "alpha"}})
	if err == nil {
		t.Fatal("cursor from a different sort should be rejected")
	}
}

func TestUserBlueprintNewestSortRejectsBroadScope(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?scope=all&sort=newest", nil)
	_, err := parseUserBlueprintRequest(c, []account.AccountWithRole{
		{ID: "acct-1", Name: "alpha"},
		{ID: "acct-2", Name: "beta"},
	})
	if err == nil {
		t.Fatal("broad sort=newest should be rejected until it has an index-backed order")
	}
}
