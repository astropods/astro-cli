package devenv

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/go-github/v69/github"
)

type Environment struct {
	Name        string
	Index       int
	PrimaryCIDR string
	ManagedCIDR string
	Domain      string
}

const registryPath = "terraform/environments/dev/dev-registry.json"

func (c *Client) FetchRegistry(ctx context.Context) (map[string]int, error) {
	file, _, _, err := c.gh.Repositories.GetContents(ctx, c.owner, c.repo, registryPath,
		&github.RepositoryContentGetOptions{Ref: "main"})
	if err != nil {
		return nil, fmt.Errorf("fetching registry: %w", err)
	}

	content, err := file.GetContent()
	if err != nil {
		return nil, fmt.Errorf("decoding registry: %w", err)
	}

	var registry map[string]int
	if err := json.Unmarshal([]byte(content), &registry); err != nil {
		return nil, fmt.Errorf("parsing registry JSON: %w", err)
	}
	return registry, nil
}

func RegistryToEnvList(registry map[string]int) []Environment {
	var envs []Environment
	for name, idx := range registry {
		envs = append(envs, Environment{
			Name:        name,
			Index:       idx,
			PrimaryCIDR: fmt.Sprintf("10.%d.0.0/16", 10+(idx-1)*2),
			ManagedCIDR: fmt.Sprintf("10.%d.0.0/16", 11+(idx-1)*2),
			Domain:      fmt.Sprintf("%s.dev.astropod.ai", name),
		})
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].Index < envs[j].Index })
	return envs
}
