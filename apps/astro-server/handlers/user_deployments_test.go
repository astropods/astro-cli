package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

func TestEnrichUserDeploymentRowsReturnsPartialResultAndError(t *testing.T) {
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
	mock.ExpectQuery(`(?s)SELECT sc.deployment_id.*FROM deployment_sidecars`).
		WithArgs(pq.Array([]string{"dep-1"})).
		WillReturnError(errors.New("temporary database error"))
	mock.ExpectQuery(`(?s)SELECT id.*FROM deployments.*interfaces,adapters`).
		WithArgs(pq.Array([]string{"dep-1"})).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	rows := []deploymentstore.UserDeployment{{
		Deployment: &deploymentstore.Deployment{
			ID:         "dep-1",
			AccountID:  "acct-1",
			AgentName:  "agent-1",
			BuildID:    "build-1",
			Status:     deploymentstore.StatusActive,
			DeployedAt: time.Now(),
		},
		AccountName: "alpha",
	}}
	result, enrichmentErr := enrichUserDeploymentRows(
		context.Background(),
		userDeploymentListDependencies{
			log:         logger.New("error", "json"),
			deployments: deploymentstore.NewStore(db),
		},
		rows,
	)
	if enrichmentErr == nil {
		t.Fatal("enrichment error = nil, want degraded result")
	}
	if len(result) != 1 || result[0].ID != "dep-1" || result[0].AccountName != "alpha" {
		t.Fatalf("result = %#v, want partial primary deployment", result)
	}
}

func TestParseUserDeploymentRequestRejectsAmbiguousOrLegacyInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	memberships := []account.AccountWithRole{{ID: "acct-1", Name: "alpha"}}
	tests := []string{
		"/me/deployments",
		"/me/deployments?account=alpha&scope=all",
		"/me/deployments?account=alpha&offset=",
		"/me/deployments?account=alpha&cursor=not-a-cursor",
		"/me/deployments?account=alpha&limit=0",
		"/me/deployments?account=",
		"/me/deployments?account=alpha&q=" + strings.Repeat("x", maxListQueryLen+1),
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodGet, target, nil)
			if _, err := parseUserDeploymentRequest(context, memberships); err == nil {
				t.Fatalf("parseUserDeploymentRequest(%q) succeeded", target)
			}
		})
	}
}

func TestUserDeploymentsSearchesTheSelectedScope(t *testing.T) {
	cache := &recordingCache{entries: map[string][]byte{}}
	router, mock := setupCrossAccountDeploymentRouter(t, cache)
	expectCrossAccountMemberships(mock)
	mock.ExpectQuery(`(?s)FROM deployments d.*strpos\(lower\(d.agent_name\), lower\(\$5\)\).*ORDER BY d.deployed_at DESC, d.id DESC.*LIMIT \$6`).
		WithArgs("user-1", pq.Array([]string{"acct-1", "acct-2"}), nil, nil, "support", userDeploymentDefaultLimit+1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/me/deployments?scope=all&q=%20SuPpOrT%20",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestUserDeploymentCursorRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wantTime := time.Date(2026, 8, 3, 12, 34, 56, 123456000, time.UTC)
	cursor, err := encodeUserDeploymentCursor(&deploymentstore.Deployment{
		ID:         "dep-1",
		DeployedAt: wantTime,
	})
	if err != nil {
		t.Fatalf("encodeUserDeploymentCursor: %v", err)
	}
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(
		http.MethodGet,
		"/me/deployments?account=alpha&cursor="+cursor,
		nil,
	)
	request, err := parseUserDeploymentRequest(
		context,
		[]account.AccountWithRole{{ID: "acct-1", Name: "alpha"}},
	)
	if err != nil {
		t.Fatalf("parseUserDeploymentRequest: %v", err)
	}
	if request.cursor == nil || request.cursor.ID != "dep-1" || !request.cursor.DeployedAt.Equal(wantTime) {
		t.Fatalf("cursor = %#v, want dep-1 at %s", request.cursor, wantTime)
	}
}

func TestUserDeploymentsRequiresExplicitScope(t *testing.T) {
	router, mock := setupCrossAccountDeploymentRouter(t, &recordingCache{entries: map[string][]byte{}})
	expectCrossAccountMemberships(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/deployments", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestUserDeploymentsReturnsOneGlobalMembershipGuardedPage(t *testing.T) {
	cache := &recordingCache{entries: map[string][]byte{}}
	router, mock := setupCrossAccountDeploymentRouter(t, cache)
	expectCrossAccountMemberships(mock)
	mock.ExpectQuery(`(?s)FROM deployments d.*JOIN account_members am.*ORDER BY d.deployed_at DESC, d.id DESC`).
		WithArgs("user-1", pq.Array([]string{"acct-1", "acct-2"}), nil, nil, "", 2).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/me/deployments?scope=all&limit=1",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response UserDeploymentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !response.Scope.All || len(response.Scope.Accounts) != 2 {
		t.Fatalf("scope = %#v, want explicit all with two memberships", response.Scope)
	}
	if response.Deployments == nil || len(response.Deployments) != 0 {
		t.Fatalf("deployments = %#v, want non-null empty page", response.Deployments)
	}
	if response.Page.Limit != 1 || response.Page.NextCursor != "" {
		t.Fatalf("page = %#v, want terminal first page", response.Page)
	}
	if got := rec.Header().Get("X-Astro-Cache"); got != "miss" {
		t.Fatalf("X-Astro-Cache = %q, want miss", got)
	}
}

func TestUserDeploymentsReportsRejectedAccountsWithoutExpandingScope(t *testing.T) {
	cache := &recordingCache{entries: map[string][]byte{}}
	router, mock := setupCrossAccountDeploymentRouter(t, cache)
	expectCrossAccountMemberships(mock)
	mock.ExpectQuery(`(?s)FROM deployments d.*JOIN account_members am`).
		WithArgs("user-1", pq.Array([]string{"acct-1"}), nil, nil, "", userDeploymentDefaultLimit+1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/me/deployments?account=alpha&account=foreign",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response UserDeploymentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(response.Scope.Accounts) != 1 || response.Scope.Accounts[0] != "alpha" {
		t.Fatalf("scope = %#v, want alpha only", response.Scope)
	}
	if len(response.RejectedAccounts) != 1 || response.RejectedAccounts[0] != "foreign" {
		t.Fatalf("rejected = %#v, want foreign", response.RejectedAccounts)
	}
}
