package githubbuild

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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

// CollectComponentBuilds returns one entry per component that has a build block,
// mirroring the components that astro push builds.
func CollectComponentBuilds(astroSpec *spec.AstroSpec, agentName string) []ComponentBuild {
	var builds []ComponentBuild

	agentBuild := spec.BuildConfig{}
	if astroSpec.Agent.Build != nil {
		agentBuild = *astroSpec.Agent.Build
	}
	builds = append(builds, ComponentBuild{"agent", agentName, agentBuild})

	for modelName, model := range astroSpec.Models {
		if model.Container != nil && model.Container.Build != nil {
			builds = append(builds, ComponentBuild{
				"model-" + modelName,
				fmt.Sprintf("%s-model-%s", agentName, modelName),
				*model.Container.Build,
			})
		}
	}
	for knowledgeName, knowledge := range astroSpec.Knowledge {
		c := knowledge.ResolvedContainer()
		if c.Build != nil {
			builds = append(builds, ComponentBuild{
				"knowledge-" + knowledgeName,
				fmt.Sprintf("%s-knowledge-%s", agentName, knowledgeName),
				*c.Build,
			})
		}
	}
	for toolName, tool := range astroSpec.Integrations {
		if tool.Container != nil && tool.Container.Build != nil {
			builds = append(builds, ComponentBuild{
				"integration-" + toolName,
				fmt.Sprintf("%s-integration-%s", agentName, toolName),
				*tool.Container.Build,
			})
		}
	}
	for ingestionName, ingestion := range astroSpec.Ingestion {
		if ingestion.Container.Build != nil {
			builds = append(builds, ComponentBuild{
				"ingestion-" + ingestionName,
				fmt.Sprintf("%s-ingestion-%s", agentName, ingestionName),
				*ingestion.Container.Build,
			})
		}
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

// repoBase returns the first two slash-separated segments of repoFullName ("owner/repo").
func repoBase(repoFullName string) string {
	parts := strings.SplitN(repoFullName, "/", 3)
	if len(parts) < 2 {
		return repoFullName
	}
	return parts[0] + "/" + parts[1]
}

// repoSubPath returns everything after the second slash, or "" for root connections.
func repoSubPath(repoFullName string) string {
	parts := strings.SplitN(repoFullName, "/", 3)
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}

// FetchFileContent fetches a file's raw content from GitHub at a specific ref.
// repoFullName may be "owner/repo" or "owner/repo/sub/path"; the subpath is
// prepended to filePath and the base repo is used for the API URL.
// Returns ("", nil) when the file does not exist at that ref.
func FetchFileContent(ctx context.Context, token, repoFullName, ref, filePath string) (string, error) {
	base := repoBase(repoFullName)
	if sub := repoSubPath(repoFullName); sub != "" {
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
