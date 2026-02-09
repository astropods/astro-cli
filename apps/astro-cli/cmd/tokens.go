package cmd

import "os"

// githubPackagesToken is set at build time via ldflags when building the CLI in CI.
// Example: go build -ldflags "-X github.com/postman/astro/apps/astro-cli/cmd.githubPackagesToken=$GITHUB_PACKAGES_TOKEN" .
var githubPackagesToken string

// getGitHubPackagesToken returns the token for GitHub Packages (GHCR, npm.pkg.github.com).
// It uses the build-injected value when set, otherwise the GITHUB_PACKAGES_TOKEN env var.
func getGitHubPackagesToken() string {
	if githubPackagesToken != "" {
		return githubPackagesToken
	}
	return os.Getenv("GITHUB_PACKAGES_TOKEN")
}
