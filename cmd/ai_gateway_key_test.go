package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	spec "github.com/astropods/astro-spec"
)

func TestSpecUsesAIGateway(t *testing.T) {
	tests := []struct {
		name string
		s    *spec.AstroSpec
		want bool
	}{
		{name: "nil spec", s: nil, want: false},
		{name: "marker absent", s: &spec.AstroSpec{Agent: spec.Container{Image: "x"}}, want: false},
		{name: "marker true", s: &spec.AstroSpec{Agent: spec.Container{Image: "x", AIGateway: true}}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, specUsesAIGateway(tc.s))
		})
	}
}

func TestApplyAIGatewayDevKey_InjectsSingularPair(t *testing.T) {
	// agent.astro_ai_gateway: true → exactly ASTRO_GATEWAY_URL and
	// ASTRO_GATEWAY_API_KEY in the env map. Same names the deployer would
	// inject in prod.
	s := &spec.AstroSpec{
		Agent: spec.Container{Image: "x", AIGateway: true},
	}
	resp := &aiGatewayDevKeyResponse{
		KeyID:   "tok-1",
		APIKey:  "sk-astro-test",
		BaseURL: "https://aig.test",
	}
	envVars := map[string]string{}
	require.NoError(t, applyAIGatewayDevKey(s, resp, envVars))

	assert.Equal(t, "sk-astro-test", envVars["ASTRO_GATEWAY_API_KEY"])
	assert.Equal(t, "https://aig.test", envVars["ASTRO_GATEWAY_URL"])
	// No other names — no per-model fanout, no BASE_URL.
	assert.Len(t, envVars, 2)
	if _, ok := envVars["ASTRO_GATEWAY_BASE_URL"]; ok {
		t.Error("ASTRO_GATEWAY_BASE_URL must not appear; the pair is ASTRO_GATEWAY_URL")
	}
}

func TestApplyAIGatewayDevKey_NoOpWhenMarkerOff(t *testing.T) {
	s := &spec.AstroSpec{Agent: spec.Container{Image: "x", AIGateway: false}}
	resp := &aiGatewayDevKeyResponse{APIKey: "sk", BaseURL: "u"}
	envVars := map[string]string{}
	require.NoError(t, applyAIGatewayDevKey(s, resp, envVars))
	assert.Empty(t, envVars, "no env vars when agent.astro_ai_gateway is false")
}

func TestFetchAIGatewayDevKey_NoOpWhenMarkerOff(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		http.Error(w, "should not be called", http.StatusBadRequest)
	}))
	defer srv.Close()
	prev := aiGatewayServerURLOverride
	aiGatewayServerURLOverride = srv.URL
	defer func() { aiGatewayServerURLOverride = prev }()

	at := AccountToken{Account: "acme", Token: "tok"}
	s := &spec.AstroSpec{Agent: spec.Container{Image: "x", AIGateway: false}}
	resp, err := fetchAIGatewayDevKey(context.Background(), at, s, false)
	require.NoError(t, err)
	assert.Nil(t, resp)
	assert.False(t, called, "server should not be called when marker is off")
}

func TestFetchAIGatewayDevKey_503MeansGatewayDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"AI Gateway is not configured in this environment"}`))
	}))
	defer srv.Close()
	prev := aiGatewayServerURLOverride
	aiGatewayServerURLOverride = srv.URL
	defer func() { aiGatewayServerURLOverride = prev }()

	at := AccountToken{Account: "acme", Token: "tok"}
	s := &spec.AstroSpec{Agent: spec.Container{Image: "x", AIGateway: true}}
	_, err := fetchAIGatewayDevKey(context.Background(), at, s, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled in this environment")
}

func TestFetchAIGatewayDevKey_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/accounts/acme/ai-gateway-keys", r.URL.Path)
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(aiGatewayDevKeyResponse{
			KeyID:     "tok-abc",
			APIKey:    "sk-astro-x",
			BaseURL:   "https://aig.test",
			ExpiresAt: "2026-06-04T00:00:00Z",
		})
	}))
	defer srv.Close()
	prev := aiGatewayServerURLOverride
	aiGatewayServerURLOverride = srv.URL
	defer func() { aiGatewayServerURLOverride = prev }()

	at := AccountToken{Account: "acme", Token: "tok"}
	s := &spec.AstroSpec{Agent: spec.Container{Image: "x", AIGateway: true}}
	resp, err := fetchAIGatewayDevKey(context.Background(), at, s, false)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "sk-astro-x", resp.APIKey)
	assert.Equal(t, "https://aig.test", resp.BaseURL)
}
