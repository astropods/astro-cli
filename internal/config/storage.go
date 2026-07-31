package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/astropods/astro-cli/internal/auth"
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
	canonicalizeProjectKeys(&cfg)
	return &cfg, nil
}

// canonicalizeProjectKeys rewrites project keys to their symlink-resolved form
// in-place. Older CLIs stored keys as computed by `filepath.Abs` (which does
// not resolve symlinks), but the rest of the CLI now looks up with
// `os.Getwd()`-style canonical paths.
//
// When a stored file contains both a legacy raw key (e.g. "/var/.../proj")
// and its symlink-resolved canonical key (e.g. "/private/var/.../proj"), the
// canonical entry wins on every field; the legacy entry only fills in keys
// the canonical entry was missing. This ordering is enforced by walking the
// map in two deterministic passes rather than relying on Go's randomized
// map iteration order, which previously made the "canonical wins" rule
// non-deterministic.
func canonicalizeProjectKeys(cfg *ProjectConfigs) {
	if len(cfg.Projects) == 0 {
		return
	}
	paths := make([]string, 0, len(cfg.Projects))
	for p := range cfg.Projects {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	canonical := make(map[string]*ProjectConfig, len(cfg.Projects))
	// Pass 1: seed the map with entries that are already canonical. These
	// are authoritative — their Vars and Name values win against any
	// legacy entry that resolves to the same canonical key.
	for _, path := range paths {
		proj := cfg.Projects[path]
		if proj == nil {
			continue
		}
		if canonicalProjectPath(path) == path {
			canonical[path] = proj
		}
	}
	// Pass 2: fold in legacy entries (raw key != canonical key). If no
	// authoritative entry exists the legacy entry is adopted as-is;
	// otherwise it only fills in vars/metadata the canonical entry lacks,
	// so a stale legacy value cannot silently overwrite a live canonical
	// one.
	for _, path := range paths {
		proj := cfg.Projects[path]
		if proj == nil {
			continue
		}
		key := canonicalProjectPath(path)
		if key == path {
			continue
		}
		existing, ok := canonical[key]
		if !ok {
			canonical[key] = proj
			continue
		}
		if existing.Vars == nil {
			existing.Vars = make(map[string]string)
		}
		for k, v := range proj.Vars {
			if _, present := existing.Vars[k]; !present {
				existing.Vars[k] = v
			}
		}
		if existing.Name == "" {
			existing.Name = proj.Name
		}
	}
	cfg.Projects = canonical
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

// canonicalProjectPath returns a normalized key for the project-configs map.
// It resolves symlinks when the path exists so that callers which compute the
// path via filepath.Abs (e.g. `ast create`, before chdir) and callers which
// compute it via os.Getwd (e.g. `ast configure`/`ast dev`, after chdir) end up
// with the same key on macOS where /var → /private/var and /tmp → /private/tmp.
// Falls back to the Cleaned input when the path does not exist yet or cannot
// be resolved — losing canonicalization but never losing the path.
func canonicalProjectPath(projectPath string) string {
	cleaned := filepath.Clean(projectPath)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return cleaned
	}
	return resolved
}

// lookupProject returns the project entry stored under projectPath's canonical
// form (LoadProjectConfigs migrates legacy keys so only canonical keys live in
// memory). The returned key is always canonical so fresh inserts use the new
// scheme automatically.
func lookupProject(cfg *ProjectConfigs, projectPath string) (*ProjectConfig, string, bool) {
	canonical := canonicalProjectPath(projectPath)
	if proj, ok := cfg.Projects[canonical]; ok && proj != nil {
		return proj, canonical, true
	}
	return nil, canonical, false
}

// GetProjectVars returns the stored vars for the given project path, or nil if none.
func GetProjectVars(binaryName, projectPath string) map[string]string {
	cfg, err := LoadProjectConfigs(binaryName)
	if err != nil {
		return nil
	}
	proj, _, ok := lookupProject(cfg, projectPath)
	if !ok {
		return nil
	}
	return proj.Vars
}

// MergeProjectVars upserts vars for the given project. Only non-empty newVars
// values (after whitespace trimming) are written; empty values are treated as
// "left blank" and skipped so that previously stored values are preserved. To
// deliberately clear a key use UnsetProjectVars.
//
// This is the semantics required by the interactive `ast configure` form,
// which always submits every field: pre-populated-but-untouched fields would
// otherwise silently clobber stored secrets.
func MergeProjectVars(binaryName, projectPath, agentName string, newVars map[string]string) error {
	cfg, err := LoadProjectConfigs(binaryName)
	if err != nil {
		return err
	}

	proj, key, ok := lookupProject(cfg, projectPath)
	if !ok {
		proj = &ProjectConfig{
			Name: agentName,
			Vars: make(map[string]string),
		}
		cfg.Projects[key] = proj
	}
	if proj.Vars == nil {
		proj.Vars = make(map[string]string)
	}

	for k, v := range newVars {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			proj.Vars[k] = trimmed
		}
	}

	return SaveProjectConfigs(binaryName, cfg)
}

// SetProjectVars writes vars for the given project, including empty strings.
// Use this for explicit flag-driven writes (e.g. --var KEY=) where an empty
// value is intentional. For interactive form submissions use MergeProjectVars.
func SetProjectVars(binaryName, projectPath, agentName string, newVars map[string]string) error {
	cfg, err := LoadProjectConfigs(binaryName)
	if err != nil {
		return err
	}

	proj, key, ok := lookupProject(cfg, projectPath)
	if !ok {
		proj = &ProjectConfig{
			Name: agentName,
			Vars: make(map[string]string),
		}
		cfg.Projects[key] = proj
	}
	if proj.Vars == nil {
		proj.Vars = make(map[string]string)
	}

	for k, v := range newVars {
		proj.Vars[k] = v
	}

	return SaveProjectConfigs(binaryName, cfg)
}

// UnsetProjectVars removes the given keys from the stored vars for the project.
// Missing keys are a no-op. The project entry itself is preserved so `name`
// metadata survives even if every var is cleared.
func UnsetProjectVars(binaryName, projectPath string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	cfg, err := LoadProjectConfigs(binaryName)
	if err != nil {
		return err
	}
	proj, _, ok := lookupProject(cfg, projectPath)
	if !ok || proj.Vars == nil {
		return nil
	}
	for _, k := range keys {
		delete(proj.Vars, k)
	}
	return SaveProjectConfigs(binaryName, cfg)
}
