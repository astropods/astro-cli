package handlers

// Covers the chat proxy (forwardChat + the four chat handlers): auth, deployment
// resolution, conversation-id validation/escaping at the boundary (the fix for
// query-injection into the identity-injected sidecar call), and that the id and
// OIDC identity header reach the upstream. Mirrors the messaging-proxy harness.
//
//	go test ./handlers -run TestChatProxy -v

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

const testConvID = "d4d407c3-0146-4834-8021-2a9850169554"

func setupChatRouter(upstreamURL string, withAuth bool) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New()
	deployDB, deployMock, _ := sqlmock.New()

	accountStore := account.NewAccountStore(accountDB)
	deployStore := deploymentstore.NewStore(deployDB)
	log := logger.New("error", "json")
	cfg := &config.Config{}
	cfg.Deployment.MessagingURLOverride = upstreamURL

	router := gin.New()
	if withAuth {
		router.Use(setAuthUser("user-workos-1"))
	}
	router.GET("/deployments/:id/chat/conversations",
		ListDeploymentChatConversations(log, cfg, nil, accountStore, deployStore, nil))
	router.GET("/deployments/:id/chat/conversations/:conversationId",
		GetDeploymentChatConversation(log, cfg, nil, accountStore, deployStore, nil))
	router.PUT("/deployments/:id/chat/conversations/:conversationId/title",
		SetDeploymentChatConversationTitle(log, cfg, nil, accountStore, deployStore, nil))
	router.DELETE("/deployments/:id/chat/conversations/:conversationId",
		DeleteDeploymentChatConversation(log, cfg, nil, accountStore, deployStore, nil))

	return router, accountMock, deployMock
}

func TestChatProxy_NoAuth(t *testing.T) {
	router, _, _ := setupChatRouter("", false)

	req := httptest.NewRequest(http.MethodGet, "/deployments/dep-1/chat/conversations", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

// A non-UUID conversation id is rejected at the boundary with 400 and never
// reaches the sidecar. This is the query-injection guard: gin URL-decodes path
// params, so `abc%3Fadmin%3D1` would otherwise splice `?admin=1` onto the
// trusted upstream URL.
func TestChatProxy_InvalidConversationID(t *testing.T) {
	var upstreamHits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&upstreamHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// "abc%3Fadmin%3D1" decodes to "abc?admin=1" — the exact query-injection the
	// UUID guard blocks. (Ids containing an encoded "/" like "../admin" are
	// rejected earlier by gin's router as 404, so they never reach the handler.)
	for _, raw := range []string{"not-a-uuid", "abc%3Fadmin%3D1"} {
		router, _, _ := setupChatRouter(upstream.URL, true)
		req := httptest.NewRequest(http.MethodGet,
			"/deployments/dep-1/chat/conversations/"+raw, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("id=%q: expected 400, got %d: %s", raw, rec.Code, rec.Body.String())
		}
	}
	if h := atomic.LoadInt32(&upstreamHits); h != 0 {
		t.Errorf("invalid ids must not reach the sidecar, got %d upstream hits", h)
	}
}

func TestChatProxy_GetConversation_Success(t *testing.T) {
	var gotPath, gotIdentity string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotIdentity = r.Header.Get(oidcIdentityHeader)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"conversation_id":"`+testConvID+`","messages":[]}`)
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupChatRouter(upstream.URL, true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodGet,
		"/deployments/dep-1/chat/conversations/"+testConvID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if want := "/api/chat/conversations/" + testConvID; gotPath != want {
		t.Errorf("upstream path = %q, want %q", gotPath, want)
	}
	if gotIdentity != "user-workos-1" {
		t.Errorf("upstream identity header = %q, want user-workos-1", gotIdentity)
	}
	if !strings.Contains(rec.Body.String(), testConvID) {
		t.Errorf("response not forwarded: %s", rec.Body.String())
	}
}

func TestChatProxy_DeploymentNotFound(t *testing.T) {
	router, _, deployMock := setupChatRouter("http://unused", true)
	expectDeploymentNotFound(deployMock, "dep-1")

	req := httptest.NewRequest(http.MethodGet,
		"/deployments/dep-1/chat/conversations/"+testConvID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// A stopped deployment still serves its DB-backed records, so the chat page loads
// and lists conversations against it. That must 404 before any upstream dial
// rather than 503 against the dead backend, which would trip
// AstroServerHigh5xxRateByRoute for a deployment nobody is running.
func TestChatProxy_NotRunningDeploymentReturns404(t *testing.T) {
	var upstreamHit atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamHit.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupChatRouter(upstream.URL, true)
	expectDeploymentLookupWithStatus(deployMock, "dep-1", "acct-1", "my-agent", "build-1", "test-ns", "stopped")
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-workos-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest(http.MethodGet, "/deployments/dep-1/chat/conversations", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for stopped deployment, got %d: %s", rec.Code, rec.Body.String())
	}
	if upstreamHit.Load() {
		t.Error("upstream was dialed for a stopped deployment; expected 404 before proxying")
	}
}

func TestChatProxy_UpstreamError_502(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	upstream.Close() // closed server -> connection refused -> 502

	router, accountMock, deployMock := setupChatRouter(upstream.URL, true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodGet,
		"/deployments/dep-1/chat/conversations/"+testConvID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
}
