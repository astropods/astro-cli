// Package claudesettings reads and edits Claude Code's settings files without
// disturbing anything it does not own. It changes keys inside the env block
// only, keeps every other key as it was, and records what it changed so the
// change can be undone exactly.
package claudesettings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Env keys Claude Code reads from a settings file.
const (
	EnvBaseURL       = "ANTHROPIC_BASE_URL"
	EnvCustomHeaders = "ANTHROPIC_CUSTOM_HEADERS"
)

// UserSettingsPath is Claude Code's user settings file. CLAUDE_CONFIG_DIR moves
// Claude Code's whole config directory, so it moves this file too.
func UserSettingsPath() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); dir != "" {
		return filepath.Join(dir, "settings.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// ManagedSettingsPath is where an organization's managed settings live on this OS.
func ManagedSettingsPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/ClaudeCode/managed-settings.json"
	case "windows":
		return `C:\ProgramData\ClaudeCode\managed-settings.json`
	}
	return "/etc/claude-code/managed-settings.json"
}

// Scope is which of Claude Code's settings files a Layer is.
type Scope int

const (
	ScopeManaged Scope = iota
	ScopeProjectLocal
	ScopeProject
	ScopeUser
)

// Layer is one settings file Claude Code reads env from.
type Layer struct {
	Scope Scope
	Path  string
}

// Layers lists the settings files Claude Code reads in cwd, highest precedence first.
func Layers(userPath, cwd string) []Layer {
	return []Layer{
		{ScopeManaged, ManagedSettingsPath()},
		{ScopeProjectLocal, filepath.Join(cwd, ".claude", "settings.local.json")},
		{ScopeProject, filepath.Join(cwd, ".claude", "settings.json")},
		{ScopeUser, userPath},
	}
}

// File is one settings file, held as a generic document so unknown keys
// round-trip untouched.
type File struct {
	Path string
	doc  map[string]any
	mode fs.FileMode
}

// Load reads path. A missing file loads as an empty document.
func Load(path string) (*File, error) {
	f := &File{Path: path, doc: map[string]any{}, mode: 0o600}
	data, err := os.ReadFile(path) //nolint:gosec
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if info, statErr := os.Stat(path); statErr == nil {
		f.mode = info.Mode().Perm()
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return f, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&f.doc); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	if f.doc == nil {
		f.doc = map[string]any{}
	}
	return f, nil
}

func (f *File) env(create bool) (map[string]any, error) {
	raw, ok := f.doc["env"]
	if !ok {
		if !create {
			return nil, nil
		}
		env := map[string]any{}
		f.doc["env"] = env
		return env, nil
	}
	env, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: \"env\" is not an object", f.Path)
	}
	return env, nil
}

// Env returns the env value for key and whether the key is present.
func (f *File) Env(key string) (string, bool, error) {
	env, err := f.env(false)
	if err != nil || env == nil {
		return "", false, err
	}
	raw, ok := env[key]
	if !ok {
		return "", false, nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", true, fmt.Errorf("%s: env.%s is not a string", f.Path, key)
	}
	return s, true, nil
}

// SetEnv sets an env key, creating the env block if needed.
func (f *File) SetEnv(key, value string) error {
	env, err := f.env(true)
	if err != nil {
		return err
	}
	env[key] = value
	return nil
}

// DeleteEnv removes an env key, and the env block too once it is empty.
func (f *File) DeleteEnv(key string) error {
	env, err := f.env(false)
	if err != nil || env == nil {
		return err
	}
	delete(env, key)
	if len(env) == 0 {
		delete(f.doc, "env")
	}
	return nil
}

// Save writes the file atomically, keeping its owner's permissions and removing
// any group or other access, because it can hold credentials.
func (f *File) Save() error {
	data, err := json.MarshalIndent(f.doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	// Renaming over a symlink would replace the link, so write its target.
	target := f.Path
	if resolved, err := filepath.EvalSymlinks(f.Path); err == nil {
		target = resolved
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".settings-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck
	if _, err := tmp.Write(data); err != nil {
		tmp.Close() //nolint:errcheck,gosec
		return err
	}
	if err := tmp.Chmod(f.mode &^ 0o077); err != nil {
		tmp.Close() //nolint:errcheck,gosec
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}
