package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// operateRouter wires one deployment-operate route with a gate that always
// blocks, so the test asserts the refusal rather than the operation.
func operateRouter(t *testing.T, register func(*gin.Engine, *account.AccountStore, *deploymentstore.Store)) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	accountDB, accountMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	deployDB, deployMock, _ := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "member"})
		c.Next()
	})
	register(router, account.NewAccountStore(accountDB), deploymentstore.NewStore(deployDB))
	return router, deployMock, accountMock
}

// expectActiveDeploymentForMember queues the deployment read and the membership
// hit that every operate route performs before it acts.
func expectActiveDeploymentForMember(deployMock, accountMock sqlmock.Sqlmock, depID, acctID string) {
	rev := 2
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRowWithStatus(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "active", &rev, time.Now()))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
}

// A suspended account's operate routes have to refuse with the billing reason.
// Without the gate the request proceeds, so a suspended account keeps restarting
// pods, rolling revisions, and starting ingestion jobs it is not paying for.
//
// Chat and messaging already refused, but with a 404 reading "endpoint
// unavailable", which is an outage message for a billing stop.
func TestOperateRoutes_RefuseWhenBillingBlocks(t *testing.T) {
	log := logger.New("error", "json")
	gate := blockingChecker{reason: "credits_exhausted"}

	// The gate runs before the cluster client is resolved, so a nil registry and
	// a nil config are never reached on a blocked request.
	cases := []struct {
		name  string
		path  string
		body  string
		mount func(*gin.Engine, *account.AccountStore, *deploymentstore.Store)
	}{
		{
			name: "rollback",
			path: "/api/v1/deployments/%s/rollback",
			body: `{"revision": 1}`,
			mount: func(r *gin.Engine, a *account.AccountStore, d *deploymentstore.Store) {
				r.POST("/api/v1/deployments/:id/rollback", RollbackDeployment(log, a, d, &mockQueue{}, nil, nil, gate))
			},
		},
		{
			name: "restart deployment",
			path: "/api/v1/deployments/%s/restart",
			mount: func(r *gin.Engine, a *account.AccountStore, d *deploymentstore.Store) {
				r.POST("/api/v1/deployments/:id/restart", RestartDeployment(log, a, nil, nil, d, nil, gate))
			},
		},
		{
			name: "restart pod",
			path: "/api/v1/deployments/%s/pods/pod-1/restart",
			mount: func(r *gin.Engine, a *account.AccountStore, d *deploymentstore.Store) {
				r.POST("/api/v1/deployments/:id/pods/:pod/restart", RestartPod(log, a, nil, nil, d, nil, gate))
			},
		},
		{
			name: "chat conversations",
			path: "/api/v1/deployments/%s/chat/conversations",
			mount: func(r *gin.Engine, a *account.AccountStore, d *deploymentstore.Store) {
				r.POST("/api/v1/deployments/:id/chat/conversations", ListDeploymentChatConversations(log, nil, nil, a, d, gate))
			},
		},
		{
			name: "messaging proxy",
			path: "/api/v1/deployments/%s/messaging/conversations",
			mount: func(r *gin.Engine, a *account.AccountStore, d *deploymentstore.Store) {
				r.POST("/api/v1/deployments/:id/messaging/*proxyPath", ProxyDeploymentMessaging(log, a, d, nil, nil, gate))
			},
		},
		{
			name: "trigger ingestion",
			path: "/api/v1/deployments/%s/ingestion/docs/trigger",
			mount: func(r *gin.Engine, a *account.AccountStore, d *deploymentstore.Store) {
				r.POST("/api/v1/deployments/:id/ingestion/:ingestion/trigger", TriggerIngestion(log, nil, a, nil, d, nil, nil, gate))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, deployMock, accountMock := operateRouter(t, tc.mount)
			depID, acctID := deployid.New(), uuid.New().String()
			expectActiveDeploymentForMember(deployMock, accountMock, depID, acctID)

			req := httptest.NewRequest("POST", strings.Replace(tc.path, "%s", depID, 1), strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusPaymentRequired {
				t.Fatalf("status = %d, want 402: %s", w.Code, w.Body.String())
			}
			var resp map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			if resp["reason"] != "credits_exhausted" {
				t.Errorf("reason = %v, want credits_exhausted: the CLI cannot name the fix without it", resp["reason"])
			}
			if resp["code"] != "BILLING_SUSPENDED" {
				t.Errorf("code = %v, want BILLING_SUSPENDED", resp["code"])
			}
		})
	}
}

// A suspended account keeps reading. Its conversation history lives in the
// deployment's own database, so it is already unreachable while the workload is
// scaled to zero; refusing the GET on top of that would turn an outage into a
// policy, and would block the read the day that history outlives the pod.
func TestChatAndMessaging_DoNotGateReads(t *testing.T) {
	log := logger.New("error", "json")
	gate := blockingChecker{reason: "credits_exhausted"}

	cases := []struct {
		name  string
		path  string
		mount func(*gin.Engine, *account.AccountStore, *deploymentstore.Store)
	}{
		{
			name: "chat history",
			path: "/api/v1/deployments/%s/chat/conversations",
			mount: func(r *gin.Engine, a *account.AccountStore, d *deploymentstore.Store) {
				r.GET("/api/v1/deployments/:id/chat/conversations", ListDeploymentChatConversations(log, nil, nil, a, d, gate))
			},
		},
		{
			name: "messaging read",
			path: "/api/v1/deployments/%s/messaging/agent/config",
			mount: func(r *gin.Engine, a *account.AccountStore, d *deploymentstore.Store) {
				r.GET("/api/v1/deployments/:id/messaging/*proxyPath", ProxyDeploymentMessaging(log, a, d, nil, nil, gate))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, deployMock, accountMock := operateRouter(t, tc.mount)
			depID, acctID := deployid.New(), uuid.New().String()
			// Stopped, which is what a billing suspension leaves behind.
			rev := 2
			deployMock.ExpectQuery(`SELECT`).
				WillReturnRows(deploymentByIDRowWithStatus(depID, acctID, "my-agent", "build-1", "astro-abc123",
					"My Agent", `{}`, "stopped", &rev, time.Now()))
			accountMock.ExpectQuery(`SELECT`).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", strings.Replace(tc.path, "%s", depID, 1), nil))

			if w.Code == http.StatusPaymentRequired {
				t.Fatalf("read was billing-gated: %s", w.Body.String())
			}
		})
	}
}

// The revision and status checks report what is running. A non-member reaching
// them learns another tenant's deployment state from the error text alone.
func TestRollbackDeployment_DecidesMembershipBeforePreconditions(t *testing.T) {
	log := logger.New("error", "json")
	router, deployMock, accountMock := operateRouter(t, func(r *gin.Engine, a *account.AccountStore, d *deploymentstore.Store) {
		r.POST("/api/v1/deployments/:id/rollback", RollbackDeployment(log, a, d, &mockQueue{}, nil, nil, nil))
	})

	depID, acctID := deployid.New(), uuid.New().String()
	rev := 1
	// Stopped, and already on the requested revision: both preconditions fail.
	deployMock.ExpectQuery(`SELECT`).
		WillReturnRows(deploymentByIDRowWithStatus(depID, acctID, "my-agent", "build-1", "astro-abc123",
			"My Agent", `{}`, "stopped", &rev, time.Now()))
	accountMock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	req := httptest.NewRequest("POST", "/api/v1/deployments/"+depID+"/rollback", strings.NewReader(`{"revision": 1}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); strings.Contains(body, "rollback active or failed") || strings.Contains(body, "already on this revision") {
		t.Errorf("body %q leaks the deployment's state to a non-member", body)
	}
}
