package handlers

import (
	"fmt"
	"io"
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

func setupMessagingProxyRouter(upstreamURL string, withAuth bool) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
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

	handler := ProxyDeploymentMessaging(log, accountStore, deployStore, nil, cfg, nil)
	router.Any("/deployments/:id/messaging/*proxyPath", handler)

	return router, accountMock, deployMock
}

func expectMessagingProxyAuth(accountMock, deployMock sqlmock.Sqlmock) {
	expectDeploymentLookup(deployMock, "dep-1", "acct-1", "my-agent", "build-1", "test-ns")
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-workos-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
}

func TestMessagingProxy_NoAuth(t *testing.T) {
	router, _, _ := setupMessagingProxyRouter("", false)

	req := httptest.NewRequest(http.MethodPost, "/deployments/dep-1/messaging/conversations", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMessagingProxy_DeploymentNotFound(t *testing.T) {
	router, _, deployMock := setupMessagingProxyRouter("", true)
	expectDeploymentNotFound(deployMock, "dep-1")

	req := httptest.NewRequest(http.MethodPost, "/deployments/dep-1/messaging/conversations", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// A stopped deployment still serves its DB-backed records, so the chat page loads
// and fires messaging calls (e.g. agent/config) at it. Those must 404 before any
// upstream dial rather than 503 against the dead backend, which would trip
// AstroServerHigh5xxRateByRoute for a deployment nobody is running.
func TestMessagingProxy_NotRunningDeploymentReturns404(t *testing.T) {
	var upstreamHit atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamHit.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupMessagingProxyRouter(upstream.URL, true)
	expectDeploymentLookupWithStatus(deployMock, "dep-1", "acct-1", "my-agent", "build-1", "test-ns", "stopped")
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-workos-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest(http.MethodGet, "/deployments/dep-1/messaging/agent/config", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for stopped deployment, got %d: %s", rec.Code, rec.Body.String())
	}
	if upstreamHit.Load() {
		t.Error("upstream was dialed for a stopped deployment; expected 404 before proxying")
	}
}

func TestMessagingProxy_CreateConversation_Success(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(oidcIdentityHeader) != "user-workos-1" {
			http.Error(w, "missing identity", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/conversations" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"conversation_id":"conv-abc"}`)
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupMessagingProxyRouter(upstream.URL, true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodPost, "/deployments/dep-1/messaging/conversations", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "conv-abc") {
		t.Errorf("expected conversation_id in response, got: %s", rec.Body.String())
	}
}

func TestMessagingProxy_SendMessage_Success(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations/conv-1/messages" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupMessagingProxyRouter(upstream.URL, true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodPost,
		"/deployments/dep-1/messaging/conversations/conv-1/messages",
		strings.NewReader(`{"text":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// The interaction response endpoint has no dedicated astro-server route: it rides
// the catch-all messaging proxy. This pins that contract — the deep subpath, POST
// method, request body, and injected identity all reach the sidecar unchanged.
func TestMessagingProxy_InteractionResponse_ForwardsToSidecar(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(oidcIdentityHeader) != "user-workos-1" {
			http.Error(w, "missing identity", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/chat/conversations/conv-1/interactions/int-1" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"action":"submit"`) {
			http.Error(w, "body not forwarded", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"resolved","action":"submit"}`)
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupMessagingProxyRouter(upstream.URL, true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodPost,
		"/deployments/dep-1/messaging/chat/conversations/conv-1/interactions/int-1",
		strings.NewReader(`{"action":"submit","content":{"approve":true}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "resolved") {
		t.Errorf("expected sidecar ack forwarded, got: %s", rec.Body.String())
	}
}

func TestMessagingProxy_AgentConfig_ExplicitAPIPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/config" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"agent"}`)
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupMessagingProxyRouter(upstream.URL, true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodGet, "/deployments/dep-1/messaging/api/agent/config", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMessagingProxy_Stream_SSEHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations/conv-1/stream" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"chunk\",\"content\":\"hi\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupMessagingProxyRouter(upstream.URL, true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodGet, "/deployments/dep-1/messaging/conversations/conv-1/stream", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
	if rec.Header().Get("X-Accel-Buffering") != "no" {
		t.Errorf("expected X-Accel-Buffering: no, got %q", rec.Header().Get("X-Accel-Buffering"))
	}
}

// The proxy must forward the SSE resume cursor to the sidecar; without it, a
// reconnecting EventSource can't resume and the missed finish stays lost. The
// upstream echoes the header it saw into the stream body to keep the assertion
// race-free.
func TestMessagingProxy_Stream_ForwardsLastEventID(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/conversations/conv-1/stream" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"seen_last_event_id\":%q}\n\n", r.Header.Get("Last-Event-ID"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupMessagingProxyRouter(upstream.URL, true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodGet, "/deployments/dep-1/messaging/conversations/conv-1/stream", nil)
	req.Header.Set("Last-Event-ID", "7")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), `"seen_last_event_id":"7"`) {
		t.Errorf("proxy must forward Last-Event-ID to the sidecar; body=%s", rec.Body.String())
	}
}

func TestMessagingProxy_NoOverride_NoK8s(t *testing.T) {
	router, accountMock, deployMock := setupMessagingProxyRouter("", true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodPost, "/deployments/dep-1/messaging/conversations", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMessagingUpstreamPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"conversations", "/api/conversations"},
		{"/conversations/conv-1/stream", "/api/conversations/conv-1/stream"},
		{"api/agent/config", "/api/agent/config"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := messagingUpstreamPath(tc.in); got != tc.want {
			t.Errorf("messagingUpstreamPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsValidMessagingProxyPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"plain path", "/conversations/conv-1/stream", true},
		{"api-prefixed", "api/agent/config", true},
		{"uuid conversation id", "/conversations/123e4567-e89b-12d3-a456-426614174000/audio", true},
		{"health", "health", true},
		{"raw traversal", "../../secrets", false},
		{"single-encoded traversal", "%2e%2e%2f%2e%2e%2fsecrets", false},
		{"double-encoded traversal (edge-WAF bypass)", "%252e%252e%252f%252e%252e%252fapi%252fv1", false},
		{"backslash traversal", "..\\..\\secrets", false},
		{"embedded traversal segment", "api/foo/../../../namespaces", false},
		{"dot rejected (no '.' in surface)", "api/config.json", false},
		{"empty rejected", "", false},
	}
	for _, tc := range tests {
		if got := isValidMessagingProxyPath(tc.in); got != tc.want {
			t.Errorf("isValidMessagingProxyPath(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
