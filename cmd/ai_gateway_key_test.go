package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	spec "github.com/astropods/astro-spec"
)

// The parser rejects a spec that sets both agent.astro_ai_gateway and a
// provider: gateway model, so no case below combines the two opt-ins.
func gatewayModelSpec() *spec.AstroSpec {
	return &spec.AstroSpec{
		Agent:  spec.Container{Image: "x"},
		Models: map[string]spec.Model{"default": {Provider: spec.GatewayProviderName}},
	}
}

func markerSpec() *spec.AstroSpec {
	return &spec.AstroSpec{Agent: spec.Container{Image: "x", AIGateway: true}}
}

func noGatewaySpec() *spec.AstroSpec {
	return &spec.AstroSpec{Agent: spec.Container{Image: "x"}}
}

func TestSpecUsesAIGateway(t *testing.T) {
	tests := []struct {
		name string
		s    *spec.AstroSpec
		want bool
	}{
		{name: "nil spec", s: nil, want: false},
		{name: "no opt-in", s: noGatewaySpec(), want: false},
		{name: "deprecated agent marker", s: markerSpec(), want: true},
		{name: "model with provider gateway", s: gatewayModelSpec(), want: true},
		{
			name: "model with provider gateway in mixed case",
			s: &spec.AstroSpec{
				Agent:  spec.Container{Image: "x"},
				Models: map[string]spec.Model{"default": {Provider: "GateWay"}},
			},
			want: true,
		},
		{
			name: "model with a non-gateway provider",
			s: &spec.AstroSpec{
				Agent:  spec.Container{Image: "x"},
				Models: map[string]spec.Model{"default": {Provider: "anthropic"}},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, specUsesAIGateway(tc.s),
				"this decides whether dev mints a gateway key at all")
		})
	}
}

func TestApplyAIGatewayDevKey(t *testing.T) {
	const (
		devKey = "sk-astro-test"
		devURL = "https://aig.test"
	)
	keyResp := func() *aiGatewayDevKeyResponse {
		return &aiGatewayDevKeyResponse{KeyID: "tok-1", APIKey: devKey, BaseURL: devURL}
	}
	wantPair := map[string]string{
		"ASTRO_GATEWAY_URL":     devURL,
		"ASTRO_GATEWAY_API_KEY": devKey,
	}

	tests := []struct {
		name    string
		s       *spec.AstroSpec
		resp    *aiGatewayDevKeyResponse
		wantEnv map[string]string
	}{
		{name: "deprecated agent marker", s: markerSpec(), resp: keyResp(), wantEnv: wantPair},
		{name: "model with provider gateway", s: gatewayModelSpec(), resp: keyResp(), wantEnv: wantPair},
		{name: "no opt-in", s: noGatewaySpec(), resp: keyResp(), wantEnv: map[string]string{}},
		{name: "nil spec", s: nil, resp: keyResp(), wantEnv: map[string]string{}},
		{name: "nil response", s: markerSpec(), resp: nil, wantEnv: map[string]string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ASTRO_GATEWAY_URL", "")
			t.Setenv("ASTRO_GATEWAY_API_KEY", "")

			envVars := map[string]string{}
			require.NoError(t, applyAIGatewayDevKey(tc.s, tc.resp, envVars))

			assert.Equal(t, tc.wantEnv, envVars,
				"exactly the pair the deployer injects in prod: no per-model fanout, no ASTRO_GATEWAY_BASE_URL")
		})
	}
}

func TestFetchAIGatewayDevKey(t *testing.T) {
	successResp := aiGatewayDevKeyResponse{
		KeyID:     "tok-abc",
		APIKey:    "sk-astro-x",
		BaseURL:   "https://aig.test",
		ExpiresAt: "2026-06-04T00:00:00Z",
	}

	tests := []struct {
		name       string
		s          *spec.AstroSpec
		handler    func(t *testing.T) http.HandlerFunc
		wantCalled bool
		wantErr    error
		wantResp   *aiGatewayDevKeyResponse
	}{
		{
			name: "no opt-in skips the request",
			s:    noGatewaySpec(),
			handler: func(*testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "a spec without the gateway must not mint a key", http.StatusBadRequest)
				}
			},
		},
		{
			name: "gateway disabled in this environment",
			s:    gatewayModelSpec(),
			handler: func(*testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"error":"AI Gateway is not configured in this environment"}`))
				}
			},
			wantCalled: true,
			wantErr:    errAIGatewayNotEnabled(),
		},
		{
			name: "success returns the key and base URL",
			s:    gatewayModelSpec(),
			handler: func(t *testing.T) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "/api/v1/accounts/acme/ai-gateway-keys", r.URL.Path,
						"the key is minted per account")
					assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"),
						"the account-scoped token must reach the server")
					_ = json.NewEncoder(w).Encode(successResp)
				}
			},
			wantCalled: true,
			wantResp:   &successResp,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				tc.handler(t)(w, r)
			}))
			t.Cleanup(srv.Close)
			aiGatewayServerURLOverride = srv.URL
			t.Cleanup(func() { aiGatewayServerURLOverride = "" })

			at := AccountToken{Account: "acme", Token: "tok"}
			resp, err := fetchAIGatewayDevKey(context.Background(), at, tc.s, false)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.wantErr.Error(), err.Error(),
					"the error must come from errAIGatewayNotEnabled")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantResp, resp)
			}
			assert.Equal(t, tc.wantCalled, called, "whether the CLI called the server")
		})
	}
}

func TestInjectAIGatewayDevKeyWithoutOptIn(t *testing.T) {
	tests := []struct {
		name string
		s    *spec.AstroSpec
	}{
		{name: "no opt-in", s: noGatewaySpec()},
		{name: "nil spec", s: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			envVars := map[string]string{"EXISTING": "1"}
			err := injectAIGatewayDevKey(context.Background(), io.Discard, tc.s, envVars, false)

			assert.NoError(t, err,
				"no credentials exist here, so a spec without the gateway must not reach the login check")
			assert.Equal(t, map[string]string{"EXISTING": "1"}, envVars, "env must be untouched")
		})
	}
}
