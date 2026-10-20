package cmd

import (
	"context"
	"net/http"
	"os"
	"time"
)

const githubConnectionProvider = "github"

// resolveGitHubTokenTimeout bounds the connections-token fetch.
const resolveGitHubTokenTimeout = 5 * time.Second

type issueConnectionTokenRequest struct {
	Provider string `json:"provider"`
}

type issueConnectionTokenResponse struct {
	AccessToken string `json:"access_token"`
}

// resolveGitHubToken returns a token from the GitHub connection shared with
// the current `ast dev` session, or "" if no session is open or the
// connection isn't available; callers treat "" as "fall back to gh's own
// login", not as an error.
func resolveGitHubToken(ctx context.Context, verbose bool) string {
	authzToken := os.Getenv(sandboxTokenEnvVar)
	if authzToken == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, resolveGitHubTokenTimeout)
	defer cancel()
	// Not apiPath: this endpoint takes no :account segment, which apiPath always inserts.
	reqURL := sandboxBaseURL() + "/api/v1/deployments/connections/token"
	var resp issueConnectionTokenResponse
	_, err := apiCall(ctx, http.MethodPost, reqURL, issueConnectionTokenRequest{Provider: githubConnectionProvider}, authzToken, verbose, &resp)
	if err != nil {
		return ""
	}
	return resp.AccessToken
}
