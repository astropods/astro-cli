package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// aiGatewayServerURLOverride is set in tests to redirect API calls to a test server.
var aiGatewayServerURLOverride string

func aiGatewayBaseURL() string {
	if aiGatewayServerURLOverride != "" {
		return strings.TrimSuffix(aiGatewayServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

// aiGatewayDevKeyResponse mirrors handlers.AIGatewayKeyResponse on the server.
type aiGatewayDevKeyResponse struct {
	KeyID     string `json:"key_id"`
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url"`
	ExpiresAt string `json:"expires_at"`
}

// fetchAIGatewayDevKey calls POST /api/v1/accounts/:account/ai-gateway-keys
// and returns the minted key + base URL. Returns nil when the spec does not
// opt into the gateway (agent.astro_ai_gateway is false) — no key needed.
//
// The CLI does not revoke keys on session end; the server-side TTL is the
// single lifecycle mechanism, so every `astro dev` invocation fetches a
// fresh key.
func fetchAIGatewayDevKey(ctx context.Context, at AccountToken, s *spec.AstroSpec, verbose bool) (*aiGatewayDevKeyResponse, error) {
	if !specUsesAIGateway(s) {
		return nil, nil
	}
	url := apiPath(aiGatewayBaseURL(), at.Account, "accounts", "ai-gateway-keys")
	var resp aiGatewayDevKeyResponse
	status, err := apiCall(ctx, http.MethodPost, url, nil, at.Token, verbose, &resp)
	if status == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("AI Gateway is not enabled in this environment; agents with agent.astro_ai_gateway: true can't run locally here")
	}
	if err != nil {
		return nil, fmt.Errorf("fetch AI Gateway dev key: %w", err)
	}
	return &resp, nil
}

// applyAIGatewayDevKey populates the local env map with the singular pair
// ASTRO_GATEWAY_URL + ASTRO_GATEWAY_API_KEY. Same env-var names the deployer
// injects in prod — agent code reads identical names in dev and prod.
//
// Also sets via os.Setenv so the --local agent process (which inherits the
// CLI's env) and composeBuilder (which reads envVars when populating the
// compose service env) both see them.
func applyAIGatewayDevKey(s *spec.AstroSpec, resp *aiGatewayDevKeyResponse, envVars map[string]string) error {
	if resp == nil || s == nil || !s.Agent.AIGateway {
		return nil
	}
	set := func(k, v string) error {
		envVars[k] = v
		if err := os.Setenv(k, v); err != nil {
			return fmt.Errorf("set env var %s: %w", k, err)
		}
		return nil
	}
	if err := set("ASTRO_GATEWAY_URL", resp.BaseURL); err != nil {
		return err
	}
	if err := set("ASTRO_GATEWAY_API_KEY", resp.APIKey); err != nil {
		return err
	}
	return nil
}

// specUsesAIGateway reports whether the spec opts into the AI Gateway via
// agent.astro_ai_gateway: true.
func specUsesAIGateway(s *spec.AstroSpec) bool {
	return s != nil && s.Agent.AIGateway
}
