package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func TestDeploymentListHandlersApplyFGAVisibility(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		wire   func(*gin.Engine, *account.AccountStore, *deploymentstore.Store, deploymentVisibilityResolver)
		expect func(sqlmock.Sqlmock)
		assert func(*testing.T, []byte)
	}{
		{
			name: "count", path: "/deployments/count?account=acme",
			wire: func(router *gin.Engine, accounts *account.AccountStore, deployments *deploymentstore.Store, visibility deploymentVisibilityResolver) {
				router.GET("/deployments/count", CountDeployments(logger.New("error", "json"), accounts, deployments, visibility))
			},
			expect: func(mock sqlmock.Sqlmock) {
				expectAccountByName(mock)
				expectAccountMembership(mock)
				mock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*FROM deployments d`).
					WithArgs("user-1", "acct-1", sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			},
			assert: func(t *testing.T, body []byte) {
				var response struct {
					Count int `json:"count"`
				}
				if json.Unmarshal(body, &response) != nil || response.Count != 1 {
					t.Fatalf("body=%s", body)
				}
			},
		},
		{
			name: "summary", path: "/deployments/summary",
			wire: func(router *gin.Engine, accounts *account.AccountStore, deployments *deploymentstore.Store, visibility deploymentVisibilityResolver) {
				router.GET("/deployments/summary", ListDeploymentsSummary(logger.New("error", "json"), accounts, deployments, nil, visibility))
			},
			expect: func(mock sqlmock.Sqlmock) {
				expectAccountList(mock)
				mock.ExpectQuery(`(?s)SELECT id, account_id, agent_name.*FROM deployments`).WillReturnRows(
					sqlmock.NewRows([]string{"id", "account_id", "agent_name", "display_name", "status", "avatar_colors", "avatar_updated_at", "deployed_at"}).
						AddRow("dep-readable", "acct-1", "reader", "Readable", "active", nil, nil, time.Now()).
						AddRow("dep-hidden", "acct-1", "hidden", "Hidden", "active", nil, nil, time.Now()),
				)
				expectReadableIDs(mock)
			},
			assert: assertOnlyReadableDeployment,
		},
		{
			name: "single account", path: "/deployments?account=acme",
			wire: func(router *gin.Engine, accounts *account.AccountStore, deployments *deploymentstore.Store, visibility deploymentVisibilityResolver) {
				router.GET("/deployments", ListDeployments(logger.New("error", "json"), accounts, deployments, nil, nil, nil, k8scache.NoopCache{}, visibility))
			},
			expect: func(mock sqlmock.Sqlmock) {
				expectAccountByName(mock)
				expectAccountMembership(mock)
				expectDeploymentRows(mock, `status != 'undeployed'`)
				expectReadableIDs(mock)
			},
			assert: assertOnlyReadableDeployment,
		},
		{
			name: "cross account", path: "/deployments?build_id=build-1",
			wire: func(router *gin.Engine, accounts *account.AccountStore, deployments *deploymentstore.Store, visibility deploymentVisibilityResolver) {
				router.GET("/deployments", ListDeployments(logger.New("error", "json"), accounts, deployments, nil, nil, nil, k8scache.NoopCache{}, visibility))
			},
			expect: func(mock sqlmock.Sqlmock) {
				expectAccountList(mock)
				expectDeploymentRows(mock, `build_id = ANY`)
				expectReadableIDs(mock)
			},
			assert: assertOnlyReadableDeployment,
		},
		{
			name: "history", path: "/agents/acme/agent/deployment/history",
			wire: func(router *gin.Engine, accounts *account.AccountStore, deployments *deploymentstore.Store, visibility deploymentVisibilityResolver) {
				router.GET("/agents/:account/:name/deployment/history", GetDeploymentHistory(logger.New("error", "json"), accounts, deployments, visibility))
			},
			expect: func(mock sqlmock.Sqlmock) {
				expectAccountByName(mock)
				expectAccountMembership(mock)
				now := time.Now()
				mock.ExpectQuery(`(?s)FROM deployment_revisions dr`).WithArgs("acct-1", "agent").WillReturnRows(
					sqlmock.NewRows([]string{"id", "agent_name", "revision", "build_id", "namespace", "display_name", "is_current", "status", "created_at", "source", "commit_sha", "branch", "commit_message", "repo_full_name", "deployed_by"}).
						AddRow("dep-readable", "agent", 2, "build-2", "ns", "Readable", true, "active", now, "direct", "", "", "", "", "user-1").
						AddRow("dep-readable-undeployed", "agent", 1, "build-1", "ns", "Readable prior revision", false, "undeployed", now, "direct", "", "", "", "", "user-1").
						AddRow("dep-hidden", "agent", 0, "build-0", "ns", "Hidden", false, "undeployed", now, "direct", "", "", "", "", "user-1"),
				)
				expectReadableHistoryIDs(mock)
			},
			assert: func(t *testing.T, body []byte) {
				assertReadableDeployments(t, body, 2, "dep-readable", "dep-readable-undeployed")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			mock.MatchExpectationsInOrder(false)
			accounts := account.NewAccountStore(db)
			deployments := deploymentstore.NewStore(db)
			visibility := &staticDeploymentVisibility{visibility: authz.DeploymentVisibility{
				FGAAccountIDs:         []string{"acct-1"},
				ReadableDeploymentIDs: []string{"dep-readable", "dep-readable-undeployed"},
			}}
			router := authenticatedDeploymentVisibilityRouter()
			test.wire(router, accounts, deployments, visibility)
			test.expect(mock)

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			test.assert(t, response.Body.Bytes())
			if (test.name == "count" || test.name == "single account") &&
				(len(visibility.accounts) != 1 || visibility.accounts[0].ID != "acct-1") {
				t.Fatalf("visibility accounts = %#v, want only acct-1", visibility.accounts)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDeploymentListResolverErrorReturnsServiceUnavailable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectAccountList(mock)
	router := authenticatedDeploymentVisibilityRouter()
	router.GET("/deployments/summary", ListDeploymentsSummary(
		logger.New("error", "json"), account.NewAccountStore(db), deploymentstore.NewStore(db), nil,
		&staticDeploymentVisibility{err: errors.New("WorkOS unavailable")},
	))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/deployments/summary", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func authenticatedDeploymentVisibilityRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	return router
}

func expectAccountByName(mock sqlmock.Sqlmock) {
	now := time.Now()
	mock.ExpectQuery(`(?s)FROM accounts a.*WHERE a.name`).WithArgs("acme").WillReturnRows(
		sqlmock.NewRows(account.SQLMockScanColumns).
			AddRow(account.SQLMockScanRow("acct-1", "acme", "organization", "org-1", nil, now, now)...),
	)
}

func expectAccountMembership(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM account_members`).WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
}

func expectAccountList(mock sqlmock.Sqlmock) {
	now := time.Now()
	mock.ExpectQuery(`(?s)JOIN account_members am.*WHERE am.user_id`).WithArgs("user-1").WillReturnRows(
		sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "created_at", "updated_at", "display_name", "avatar_updated_at"}).
			AddRow("acct-1", "acme", "organization", "org-1", now, now, "Acme", nil),
	)
}

func expectDeploymentRows(mock sqlmock.Sqlmock, query string) {
	now := time.Now()
	mock.ExpectQuery(query).WillReturnRows(
		sqlmock.NewRows([]string{"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id", "status", "error_message", "error_details", "status_changed_at", "current_revision", "deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at"}).
			AddRow("dep-readable", "acct-1", nil, "agent", "build-1", "ns", "Readable", `{}`, nil, nil, nil, "active", nil, nil, now, 1, now, nil, nil, nil).
			AddRow("dep-hidden", "acct-1", nil, "agent", "build-1", "ns", "Hidden", `{}`, nil, nil, nil, "active", nil, nil, now, 1, now, nil, nil, nil),
	)
}

func expectReadableIDs(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`(?s)SELECT d.id.*JOIN account_members.*\$5::boolean OR d.status <> 'undeployed'`).
		WithArgs("user-1", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), false).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("dep-readable"))
}

func expectReadableHistoryIDs(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`(?s)SELECT d.id.*JOIN account_members.*\$5::boolean OR d.status <> 'undeployed'`).
		WithArgs("user-1", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), true).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).
			AddRow("dep-readable").
			AddRow("dep-readable-undeployed"))
}

func assertOnlyReadableDeployment(t *testing.T, body []byte) {
	assertReadableDeployments(t, body, 1, "dep-readable")
}

func assertReadableDeployments(t *testing.T, body []byte, wantCount int, wantIDs ...string) {
	t.Helper()
	for _, id := range wantIDs {
		if !containsAll(string(body), id) {
			t.Fatalf("authorized deployment %q missing from body=%s", id, body)
		}
	}
	if containsAll(string(body), `dep-hidden`) {
		t.Fatalf("body=%s", body)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if raw, ok := envelope["count"]; ok {
		var count int
		if json.Unmarshal(raw, &count) != nil || count != wantCount {
			t.Fatalf("filtered count=%s, want %d", raw, wantCount)
		}
	}
}
