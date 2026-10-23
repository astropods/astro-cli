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

// resolveGitHubToken returns the dev session's connection token, or "" (not
// an error) if no session is open or the connection isn't available.
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

// devSessionActive reports whether a dev session's token was available to
// try, as opposed to there being no dev session at all.
func devSessionActive() bool {
	return os.Getenv(sandboxTokenEnvVar) != ""
}
