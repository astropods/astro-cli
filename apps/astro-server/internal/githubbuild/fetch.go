package githubbuild

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/readmeassets"
	spec "github.com/astropods/astro-spec"
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

// githubContentsGet performs a GitHub contents API GET for repoRelPath (a path
// relative to the repo root) at ref, using the given Accept media type and
// reading at most limit bytes. accept selects the representation: the raw media
// type for a file's bytes, or JSON for a directory listing. Path segments are
// URL-encoded so paths with spaces or unicode (common for image references)
// resolve correctly. Returns (nil, nil) when the path does not exist at that ref.
func githubContentsGet(ctx context.Context, token, repoBase, ref, repoRelPath, accept string, limit int64) ([]byte, error) {
	reqURL := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s?ref=%s", repoBase, encodePathSegments(repoRelPath), ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", accept)
	req.Header.Set("X-Github-Api-Version", "2022-11-28")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", reqURL, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s returned %d", reqURL, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// fetchFile retrieves the raw bytes of a repo file via the GitHub contents API,
// reading at most limit bytes. repoFullName may be "owner/repo" or
// "owner/repo/sub/path"; the subpath is prepended to filePath and the base repo
// is used for the API URL. Returns (nil, nil) when the file does not exist.
func fetchFile(ctx context.Context, token, repoFullName, ref, filePath string, limit int64) ([]byte, error) {
	base := githubconnection.RepoBase(repoFullName)
	if sub := githubconnection.RepoSubPath(repoFullName); sub != "" {
		filePath = sub + "/" + filePath
	}
	return githubContentsGet(ctx, token, base, ref, filePath, "application/vnd.github.raw+json", limit)
}

// FetchFileContent fetches a file's raw content from GitHub at a specific ref.
// repoFullName may be "owner/repo" or "owner/repo/sub/path"; the subpath is
// prepended to filePath and the base repo is used for the API URL.
// Returns ("", nil) when the file does not exist at that ref.
func FetchFileContent(ctx context.Context, token, repoFullName, ref, filePath string) (string, error) {
	data, err := fetchFile(ctx, token, repoFullName, ref, filePath, 1<<20)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FetchFileBytes fetches a file's raw bytes from GitHub at a specific ref,
// capped at readmeassets.MaxAssetSize+1 so oversized images are detected and
// rejected downstream. Returns (nil, nil) when the file does not exist.
func FetchFileBytes(ctx context.Context, token, repoFullName, ref, filePath string) ([]byte, error) {
	return fetchFile(ctx, token, repoFullName, ref, filePath, readmeassets.MaxAssetSize+1)
}

// FetchAgentReadme fetches the agent README (AGENT.md) from GitHub at a specific
// ref, matching the filename case-insensitively. It lists the containing
// directory, finds the entry matching spec.AgentReadmeFilename, and fetches it.
// Returns ("", nil) when no such file exists.
func FetchAgentReadme(ctx context.Context, token, repoFullName, ref string) (string, error) {
	name, err := findAgentReadmeName(ctx, token, repoFullName, ref)
	if err != nil || name == "" {
		return "", err
	}
	return FetchFileContent(ctx, token, repoFullName, ref, name)
}

// ghContentEntry is one entry in a GitHub contents API directory listing.
type ghContentEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// findAgentReadmeName lists the agent's directory on GitHub and returns the
// actual filename matching spec.AgentReadmeFilename case-insensitively, or ""
// if none exists.
func findAgentReadmeName(ctx context.Context, token, repoFullName, ref string) (string, error) {
	base := githubconnection.RepoBase(repoFullName)
	dir := githubconnection.RepoSubPath(repoFullName)
	body, err := githubContentsGet(ctx, token, base, ref, dir, "application/vnd.github+json", 1<<20)
	if err != nil || len(body) == 0 {
		return "", err
	}
	var entries []ghContentEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return "", fmt.Errorf("parse contents listing for %s: %w", repoFullName, err)
	}
	for _, e := range entries {
		if e.Type == "file" && strings.EqualFold(e.Name, spec.AgentReadmeFilename) {
			return e.Name, nil
		}
	}
	return "", nil
}

// encodePathSegments URL-encodes each "/"-separated segment of a repo path,
// preserving the separators.
func encodePathSegments(p string) string {
	segments := strings.Split(p, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}

// BuildAgentCardJSON mirrors the agent card generation logic in handlers/agents.go.
func BuildAgentCardJSON(readme string, specMap map[string]any) string {
	if readme == "" {
		return ""
	}
	card := spec.ParseAgentCard(readme)
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
