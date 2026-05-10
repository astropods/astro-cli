package githubbuild

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	spec "github.com/astropods/astro/packages/astro-spec"
	"gopkg.in/yaml.v3"
)

// httpClient is used for GitHub API calls.
// A 30-second timeout bounds any individual request without cutting off
// the overall 25-minute job budget.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// ComponentBuild holds everything needed to run one BuildKit job for a spec component.
type ComponentBuild struct {
	Suffix string // K8s job name suffix, e.g. "agent", "model-llm"
	Name   string // image name segment, e.g. "my-agent", "my-agent-model-llm"
	Build  spec.BuildConfig
}

// CollectComponentBuilds returns one entry per component that has a build block.
// Delegates to the shared spec.CollectComponents for canonical naming and iteration.
func CollectComponentBuilds(astroSpec *spec.AstroSpec, agentName string) []ComponentBuild {
	components := spec.CollectComponents(astroSpec, agentName)
	builds := make([]ComponentBuild, 0, len(components))
	for _, c := range components {
		builds = append(builds, ComponentBuild{
			Suffix: c.Suffix(),
			Name:   c.ImageName,
			Build:  *c.Build,
		})
	}
	return builds
}

// FetchAstroSpec downloads astropods.yml via the GitHub contents API at a specific SHA.
func FetchAstroSpec(ctx context.Context, token, repoFullName, commitSHA string) (*spec.AstroSpec, string, error) {
	content, err := FetchFileContent(ctx, token, repoFullName, commitSHA, "astropods.yml")
	if err != nil {
		return nil, "", fmt.Errorf("fetch astropods.yml: %w", err)
	}
	var s spec.AstroSpec
	if err := yaml.Unmarshal([]byte(content), &s); err != nil {
		return nil, "", fmt.Errorf("parse astropods.yml: %w", err)
	}
	return &s, content, nil
}

// FetchFileContent fetches a file's raw content from GitHub at a specific ref.
// repoFullName may be "owner/repo" or "owner/repo/sub/path"; the subpath is
// prepended to filePath and the base repo is used for the API URL.
// Returns ("", nil) when the file does not exist at that ref.
func FetchFileContent(ctx context.Context, token, repoFullName, ref, filePath string) (string, error) {
	base := githubconnection.RepoBase(repoFullName)
	if sub := githubconnection.RepoSubPath(repoFullName); sub != "" {
		filePath = sub + "/" + filePath
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s?ref=%s", base, filePath, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	req.Header.Set("X-Github-Api-Version", "2022-11-28")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// BuildAgentCardJSON mirrors the agent card generation logic in handlers/agents.go.
func BuildAgentCardJSON(readme string, specMap map[string]any) string {
	if readme == "" {
		return ""
	}
	card, err := spec.ParseAgentCard(readme)
	if err != nil || card == nil {
		return ""
	}
	var providers []string
	if integrations, ok := specMap["integrations"].(map[string]any); ok {
		for _, v := range integrations {
			if entry, ok := v.(map[string]any); ok {
				if p, ok := entry["provider"].(string); ok && p != "" {
					providers = append(providers, p)
				}
			}
		}
	}
	card.ResolvedIntegrations = spec.MergeResolvedIntegrations(card.ResolvedIntegrations, providers)
	out, err := json.Marshal(card)
	if err != nil {
		return ""
	}
	return string(out)
}
