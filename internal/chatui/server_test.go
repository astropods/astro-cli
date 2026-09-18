package chatui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// The chat composer's files API (/files) must be proxied to the sidecar's
// /api/files* routes. Before it was wired, /files fell through to the SPA
// catch-all, which returns http.NotFound for any "api/" path — so the Files tab
// showed "request failed with status 404" even when the list was simply empty.
func TestServer_ProxiesFilesContract(t *testing.T) {
	var (
		mu       sync.Mutex
		gotPaths []string
	)
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPaths = append(gotPaths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"files":[]}`))
	}))
	defer sidecar.Close()

	s, err := New(Config{Addr: "127.0.0.1:0", MessagingURL: sidecar.URL, AgentName: "test"})
	require.NoError(t, err)
	handler := s.Handler()

	tests := []struct {
		name         string
		method       string
		reqPath      string
		wantUpstream string
	}{
		{"list", http.MethodGet, "/api/v1/deployments/local/files", "/api/files"},
		{"create", http.MethodPost, "/api/v1/deployments/local/files", "/api/files"},
		{"metadata", http.MethodGet, "/api/v1/deployments/local/files/abc123", "/api/files/abc123"},
		{"content", http.MethodGet, "/api/v1/deployments/local/files/abc123/content", "/api/files/abc123/content"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mu.Lock()
			gotPaths = nil
			mu.Unlock()

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.reqPath, nil))

			require.Equal(t, http.StatusOK, rec.Code,
				"files request must reach the sidecar, not 404 at the SPA fallback")
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, []string{tt.wantUpstream}, gotPaths,
				"sidecar should receive the rewritten /api/files path")
		})
	}
}

// The chat shell reconciles the switcher summary against the per-account
// deployment list, and invalidates the summary query for as long as a
// chat-eligible deployment is missing from it. A summary that omits the local
// deployment therefore costs a summary read on every mount of the chat page.
func TestServer_SummaryAndListReportTheSameDeployment(t *testing.T) {
	s, err := New(Config{
		Addr:         "127.0.0.1:0",
		MessagingURL: "http://127.0.0.1:1",
		AgentName:    "my-agent",
		AgentDisplay: "My Agent",
	})
	require.NoError(t, err)
	handler := s.Handler()

	get := func(path string, dest any) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code)
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), dest))
	}

	var summary deploymentsSummaryResponse
	get("/api/v1/deployments/summary", &summary)
	var list deploymentsListResponse
	get("/api/v1/deployments", &list)

	require.Len(t, list.Deployments, 1)
	require.Len(t, summary.Accounts, 1)
	require.Equal(t, LocalAccount, summary.Accounts[0].Name)
	require.Equal(t, []deploymentSummaryItem{{
		ID:                     list.Deployments[0].ID,
		Name:                   list.Deployments[0].Name,
		DisplayName:            list.Deployments[0].DisplayName,
		Status:                 localDeploymentStatus,
		MessagingWebConfigured: list.Deployments[0].MessagingWebConfigured,
	}}, summary.Accounts[0].Deployments,
		"the summary must carry the deployment the list reports, or the chat page refetches it on every mount")
}
