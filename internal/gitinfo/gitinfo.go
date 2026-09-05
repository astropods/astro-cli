// Package gitinfo reads local git metadata for scaffolding. Every lookup is
// best-effort: a missing git binary, a directory outside a repository, or a
// repository without an origin all return a zero value.
package gitinfo

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// Repository points at the git origin that holds a project, and at the
// project's path inside it.
type Repository struct {
	URL       string
	Directory string
}

// For describes the repository that will contain a project created at
// projectDir. It resolves origin from the parent directory, because projectDir
// does not exist yet when a scaffold renders.
func For(projectDir string) Repository {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return Repository{}
	}
	parent := filepath.Dir(abs)

	url := BrowsableURL(run(parent, "remote", "get-url", "origin"))
	if url == "" {
		return Repository{}
	}

	return Repository{URL: url, Directory: directoryIn(parent, filepath.Base(abs))}
}

// UserName returns the git user.name configured for dir, or an empty string.
func UserName(dir string) string {
	return run(dir, "config", "user.name")
}

// BrowsableURL rewrites a git remote into an http(s) URL a browser can open.
// It returns an empty string for a remote it cannot rewrite, such as a local
// path or an unrecognized scheme.
func BrowsableURL(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	remote = strings.TrimSuffix(strings.TrimSuffix(remote, "/"), ".git")

	switch {
	case strings.HasPrefix(remote, "https://"), strings.HasPrefix(remote, "http://"):
		return remote
	case strings.HasPrefix(remote, "ssh://"):
		return "https://" + stripUserInfo(strings.TrimPrefix(remote, "ssh://"))
	case strings.HasPrefix(remote, "git://"):
		return "https://" + stripUserInfo(strings.TrimPrefix(remote, "git://"))
	}

	// scp-like form: git@github.com:org/repo
	if host, path, ok := strings.Cut(remote, ":"); ok && strings.Contains(host, "@") && !strings.HasPrefix(path, "/") {
		return "https://" + stripUserInfo(host) + "/" + path
	}
	return ""
}

func stripUserInfo(hostAndPath string) string {
	if _, rest, ok := strings.Cut(hostAndPath, "@"); ok {
		return rest
	}
	return hostAndPath
}

// directoryIn returns the path of parent/name relative to the repository root.
func directoryIn(parent, name string) string {
	root := run(parent, "rev-parse", "--show-toplevel")
	if root == "" {
		return ""
	}
	// git resolves symlinks in the root path, so resolve the project side too.
	if resolved, err := filepath.EvalSymlinks(parent); err == nil {
		parent = resolved
	}
	rel, err := filepath.Rel(root, filepath.Join(parent, name))
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}

func run(dir string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
