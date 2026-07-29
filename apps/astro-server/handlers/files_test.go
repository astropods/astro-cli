package handlers

import (
	"encoding/json"
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

// parseFileKey is the trust-boundary guard: whatever it accepts is url.PathEscape'd
// and spliced into the upstream sidecar URL, so a regression here (letting a path
// separator or control char through) would let a client reshape the upstream
// request. These cases pin the accept/reject contract.
func TestParseFileKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"uuid", "550e8400-e29b-41d4-a716-446655440000", true},
		{"plain filename", "report.pdf", true},
		{"filename with spaces", "my report.pdf", true},
		{"unicode filename", "résumé.pdf", true},
		{"dotted but not traversal", "..config", true},
		{"max length", strings.Repeat("a", filesKeyMaxRunes), true},

		{"empty", "", false},
		{"dot", ".", false},
		{"dotdot", "..", false},
		{"forward slash", "a/b", false},
		{"back slash", "a\\b", false},
		{"leading slash traversal", "/etc/passwd", false},
		{"nul byte", "a\x00b", false},
		{"tab control char", "a\tb", false},
		{"newline control char", "a\nb", false},
		{"over length", strings.Repeat("a", filesKeyMaxRunes+1), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseFileKey(tc.in)
			if ok != tc.want {
				t.Fatalf("parseFileKey(%q) ok = %v, want %v", tc.in, ok, tc.want)
			}
			if ok && got != tc.in {
				t.Errorf("parseFileKey(%q) returned %q, want the input unchanged", tc.in, got)
			}
		})
	}
}

// setupFilesRouter wires the files handlers against sqlmock stores and a fake
// sidecar reached via MessagingURLOverride (the same seam the messaging proxy
// tests use). Routes are registered in the same order as main.go so the static
// "usage" segment resolves ahead of the ":fileKey" param.
func setupFilesRouter(upstreamURL string, withAuth bool) (*gin.Engine, sqlmock.Sqlmock, sqlmock.Sqlmock) {
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

	router.GET("/deployments/:id/files",
		ListDeploymentFiles(log, cfg, nil, accountStore, deployStore))
	router.POST("/deployments/:id/files",
		CreateDeploymentFile(log, cfg, nil, accountStore, deployStore))
	router.GET("/deployments/:id/files/usage",
		GetDeploymentStorageUsage(log, cfg, nil, accountStore, deployStore))
	router.GET("/deployments/:id/files/:fileKey",
		GetDeploymentFile(log, cfg, nil, accountStore, deployStore))
	router.DELETE("/deployments/:id/files/:fileKey",
		DeleteDeploymentFile(log, cfg, nil, accountStore, deployStore))
	router.PUT("/deployments/:id/files/:fileKey/content",
		UploadDeploymentFileContent(log, cfg, nil, accountStore, deployStore))
	router.GET("/deployments/:id/files/:fileKey/content",
		DownloadDeploymentFileContent(log, cfg, nil, accountStore, deployStore))

	return router, accountMock, deployMock
}

// A stopped deployment must 404 before any upstream dial.
func TestFilesProxy_NotRunningDeploymentReturns404(t *testing.T) {
	var upstreamHit atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamHit.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupFilesRouter(upstream.URL, true)
	expectDeploymentLookupWithStatus(deployMock, "dep-1", "acct-1", "my-agent", "build-1", "test-ns", "stopped")
	accountMock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-workos-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	req := httptest.NewRequest(http.MethodGet, "/deployments/dep-1/files/usage", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for stopped deployment, got %d: %s", rec.Code, rec.Body.String())
	}
	if upstreamHit.Load() {
		t.Error("upstream was dialed for a stopped deployment; expected 404 before proxying")
	}
}

func TestFilesProxy_NoAuth(t *testing.T) {
	router, _, _ := setupFilesRouter("", false)

	req := httptest.NewRequest(http.MethodGet, "/deployments/dep-1/files", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

// The download path must carry the caller's identity to the sidecar — that
// header is how the sidecar enforces per-user file isolation. If it were dropped
// the isolation boundary would silently open.
func TestFilesProxy_DownloadForwardsIdentityAndBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(oidcIdentityHeader) != "user-workos-1" {
			http.Error(w, "missing identity", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/files/report.pdf/content" || r.Method != http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="report.pdf"`)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "PDFBYTES")
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupFilesRouter(upstream.URL, true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodGet,
		"/deployments/dep-1/files/report.pdf/content", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "PDFBYTES" {
		t.Errorf("expected forwarded body, got %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("expected Content-Type forwarded, got %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "report.pdf") {
		t.Errorf("expected Content-Disposition forwarded, got %q", cd)
	}
}

// An oversized declared upload must be rejected at the proxy with a 413 before it
// dials the sidecar, so a bad request never opens an upstream connection.
func TestFilesProxy_OversizeUploadRejectedBeforeUpstream(t *testing.T) {
	var upstreamHit atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupFilesRouter(upstream.URL, true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodPut,
		"/deployments/dep-1/files/report.pdf/content", strings.NewReader("x"))
	// Declare a length past the cap; the guard reads Content-Length, not the body.
	req.ContentLength = filesProxyMaxUploadBytes + 1
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rec.Code, rec.Body.String())
	}
	if upstreamHit.Load() {
		t.Error("upstream was dialed for an oversized upload; expected rejection before connecting")
	}
}

// A 3xx from the sidecar (a presigned object URL once an S3-backed store lands)
// must reach the client verbatim, not be followed by the proxy.
func TestFilesProxy_RedirectPassedThroughNotFollowed(t *testing.T) {
	const presigned = "https://objects.example.com/presigned-download"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/files/report.pdf/content" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Location", presigned)
		w.WriteHeader(http.StatusFound)
		// A body accompanies the redirect so the recorder flushes the status.
		fmt.Fprint(w, "redirecting")
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupFilesRouter(upstream.URL, true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodGet,
		"/deployments/dep-1/files/report.pdf/content", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 passed through, got %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != presigned {
		t.Errorf("expected Location %q forwarded, got %q", presigned, loc)
	}
}

// The 507 the sidecar returns when its volume is full must be forwarded verbatim
// so the client can map it to the storage-full message.
func TestFilesProxy_InsufficientStoragePassedThrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/files" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInsufficientStorage)
		fmt.Fprint(w, `{"error":"storage full"}`)
	}))
	defer upstream.Close()

	router, accountMock, deployMock := setupFilesRouter(upstream.URL, true)
	expectMessagingProxyAuth(accountMock, deployMock)

	req := httptest.NewRequest(http.MethodPost, "/deployments/dep-1/files",
		strings.NewReader(`{"name":"big.bin","size":123}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("expected 507 passed through, got %d: %s", rec.Code, rec.Body.String())
	}
	var apiErr ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("expected JSON error response, got %q: %v", rec.Body.String(), err)
	}
	if apiErr.Error != "insufficient_storage" || !strings.Contains(apiErr.Details, "storage is full") {
		t.Errorf("expected normalized storage-full error, got %+v", apiErr)
	}
}

func TestNormalizedFilesError_PreservesKnownSidecarCode(t *testing.T) {
	status, apiErr := normalizedFilesError(http.StatusBadRequest, []byte(
		`{"error":"invalid_file_name","details":"untrusted replacement"}`,
	))

	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if apiErr.Error != "invalid_file_name" || apiErr.Details != "This file name isn't supported." {
		t.Fatalf("expected canonical invalid-name error, got %+v", apiErr)
	}
}

func TestNormalizedFilesError_SanitizesKubernetesStatus(t *testing.T) {
	status, apiErr := normalizedFilesError(http.StatusForbidden, []byte(
		`{"kind":"Status","status":"Failure","message":"pods internal-name is forbidden"}`,
	))

	// A K8s proxy failure means the backend pod is unreachable — a 4xx, not a
	// route 5xx, and never leaks internal names.
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if apiErr.Error != "file_storage_unavailable" || strings.Contains(apiErr.Details, "internal-name") {
		t.Fatalf("expected sanitized availability error, got %+v", apiErr)
	}
}

// An invalid file key must be rejected at the handler with a 400 before any auth
// or upstream work — the parse gate runs first.
func TestFilesProxy_InvalidKeyRejectedBeforeUpstream(t *testing.T) {
	var upstreamHit atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	router, _, _ := setupFilesRouter(upstream.URL, true)

	// Over-length key: a valid URL path segment that parseFileKey rejects.
	req := httptest.NewRequest(http.MethodGet,
		"/deployments/dep-1/files/"+strings.Repeat("a", filesKeyMaxRunes+1), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if upstreamHit.Load() {
		t.Error("upstream was dialed for an invalid key; expected rejection at the parse gate")
	}
}
