package cmd

import (
	"context"
	"net/http"
	"os"
	"strings"
)

const githubConnectionProvider = "github"

type issueConnectionTokenRequest struct {
	Provider string `json:"provider"`
}

type issueConnectionTokenResponse struct {
	AccessToken string `json:"access_token"`
}

// resolveGitHubToken prefers the GitHub connection shared with the current
// `ast dev` session (ASTRO_AUTHZ_TOKEN is only set while one is open) over
// the operator's own local `gh` login, since a dev session's sandbox may have
// no local git credentials of its own. An empty return means "no connection
// available here"; callers fall back to the operator's own `gh`/`git` login
// rather than treating it as fatal, since that's a normal, common case.
func resolveGitHubToken(ctx context.Context, verbose bool) string {
	authzToken := os.Getenv(sandboxTokenEnvVar)
	if authzToken == "" {
		return ""
	}
	reqURL := strings.TrimSuffix(sandboxBaseURL(), "/") + "/api/v1/deployments/connections/token"
	var resp issueConnectionTokenResponse
	_, err := apiCall(ctx, http.MethodPost, reqURL, issueConnectionTokenRequest{Provider: githubConnectionProvider}, authzToken, verbose, &resp)
	if err != nil {
		return ""
	}
	return resp.AccessToken
}
