package cmd

import (
	"context"
	"fmt"
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
		if verbose {
			fmt.Fprintf(os.Stderr, "github: could not fetch this dev session's connection token: %v\n", err) //nolint:errcheck,gosec
		}
		return ""
	}
	return resp.AccessToken
}

// devSessionActive reports whether ASTRO_AUTHZ_TOKEN is set, i.e. whether
// resolveGitHubToken had a dev session to try, as opposed to there being
// none at all. Callers use this to tell "this session's connection token
// fetch failed" from "no dev session is open" in their own error text.
func devSessionActive() bool {
	return os.Getenv(sandboxTokenEnvVar) != ""
}
