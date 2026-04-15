package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
)

// ProjectConfig holds the stored configuration for a single project.
type ProjectConfig struct {
	Name string            `json:"name"`
	Vars map[string]string `json:"vars"`
}

// ProjectConfigs is the top-level structure stored in project-configs.json.
// Projects is keyed by absolute project path.
type ProjectConfigs struct {
	Projects map[string]*ProjectConfig `json:"projects"`
}

// ConfigsPath returns the path to project-configs.json.
func ConfigsPath(binaryName string) (string, error) {
	dir, err := auth.ConfigDir(binaryName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "project-configs.json"), nil
}

// LoadProjectConfigs loads project configs from disk.
// Returns an empty ProjectConfigs (not an error) when the file does not exist.
func LoadProjectConfigs(binaryName string) (*ProjectConfigs, error) {
	path, err := ConfigsPath(binaryName)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return &ProjectConfigs{Projects: make(map[string]*ProjectConfig)}, nil
		}
		return nil, err
	}

	var cfg ProjectConfigs
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Projects == nil {
		cfg.Projects = make(map[string]*ProjectConfig)
	}
	return &cfg, nil
}

// SaveProjectConfigs writes project configs to disk with 0600 permissions.
func SaveProjectConfigs(binaryName string, configs *ProjectConfigs) error {
	path, err := ConfigsPath(binaryName)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// GetProjectVars returns the stored vars for the given project path, or nil if none.
func GetProjectVars(binaryName, projectPath string) map[string]string {
	cfg, err := LoadProjectConfigs(binaryName)
	if err != nil {
		return nil
	}
	proj, ok := cfg.Projects[projectPath]
	if !ok || proj == nil {
		return nil
	}
	return proj.Vars
}

// MergeProjectVars upserts vars for the given project. Only non-empty newVars values
// are written; empty string values are skipped so existing values are preserved.
func MergeProjectVars(binaryName, projectPath, agentName string, newVars map[string]string) error {
	cfg, err := LoadProjectConfigs(binaryName)
	if err != nil {
		return err
	}

	proj, ok := cfg.Projects[projectPath]
	if !ok || proj == nil {
		proj = &ProjectConfig{
			Name: agentName,
			Vars: make(map[string]string),
		}
		cfg.Projects[projectPath] = proj
	}
	if proj.Vars == nil {
		proj.Vars = make(map[string]string)
	}

	for k, v := range newVars {
		proj.Vars[k] = strings.TrimSpace(v)
	}

	return SaveProjectConfigs(binaryName, cfg)
}
