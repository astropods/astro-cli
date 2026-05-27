package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPollDeploymentPublicURL_Ready(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("testaccount"))

	ready := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/deployments/dep-1" {
			http.NotFound(w, r)
			return
		}
		ready = true
		_ = json.NewEncoder(w).Encode(agentDeploymentFullResponse{
			Deployment: agentDeploymentFull{
				ID: "dep-1",
				ExternalURLs: []serviceEndpointInfo{{
					Type:  "messaging",
					URL:   "https://agent.example.com",
					Ready: ready,
				}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	agentServerURLOverride = srv.URL
	t.Cleanup(func() { agentServerURLOverride = "" })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	var buf bytes.Buffer
	err := pollDeploymentPublicURL(ctx, "dep-1", AccountToken{Account: "testaccount", Token: "tok"}, false, &buf)
	require.NoError(t, err)
	assert.Contains(t, stripANSI(buf.String()), "https://agent.example.com")
}

func TestMessagingEndpoint(t *testing.T) {
	urls := []serviceEndpointInfo{
		{Type: "agent", URL: "https://a.example.com"},
		{Type: "messaging", URL: "https://m.example.com"},
	}
	ep := messagingEndpoint(urls)
	require.NotNil(t, ep)
	assert.Equal(t, "https://m.example.com", ep.URL)
}
