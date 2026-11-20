package git

import (
	"errors"
	"strings"
)

var (
	// ErrNotARepo means the directory is not inside a git working tree.
	ErrNotARepo = errors.New("not inside a git repository")
	// ErrNoOrigin means the repository has no remote named origin.
	ErrNoOrigin = errors.New("the repository has no origin remote")
	// ErrDetachedHead means HEAD names no branch, so there is no branch to
	// build from.
	ErrDetachedHead = errors.New("HEAD is not on a branch")
	// ErrNotGitHub means the remote points at a host other than github.com,
	// the only host the platform builds from.
	ErrNotGitHub = errors.New("the remote is not on github.com")
	// ErrUnparsableRemote means the remote names github.com but its path is not
	// owner/repo.
	ErrUnparsableRemote = errors.New("can't read owner/repo from the remote")
)

// OriginURL returns the URL of the origin remote for the repository containing
// dir. An empty dir means the working directory.
func OriginURL(dir string) (string, error) {
	out, err := runGit(dir, "remote", "get-url", "origin")
	if err != nil {
		// git's exit status doesn't portably tell "no such remote" from "not a
		// repository", so ask the second question directly.
		if _, repoErr := runGit(dir, "rev-parse", "--git-dir"); repoErr != nil {
			return "", ErrNotARepo
		}
		return "", ErrNoOrigin
	}
	if out = strings.TrimSpace(out); out == "" {
		return "", ErrNoOrigin
	}
	return out, nil
}

// CurrentBranch returns the branch HEAD points at in the repository containing
// dir.
func CurrentBranch(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", ErrNotARepo
	}
	// A detached HEAD abbreviates to the literal "HEAD".
	if out = strings.TrimSpace(out); out == "" || out == "HEAD" {
		return "", ErrDetachedHead
	}
	return out, nil
}

// RepoPrefix returns dir's path relative to the root of its repository, with
// no trailing slash, or "" at the root.
func RepoPrefix(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--show-prefix")
	if err != nil {
		return "", ErrNotARepo
	}
	return strings.Trim(strings.TrimSpace(out), "/"), nil
}

// ParseGitHubRemote reduces a git remote URL to its "owner/repo" form.
//
// It accepts the shapes git prints for a GitHub remote: scp-style
// (git@github.com:owner/repo.git), ssh:// and https:// URLs, a bare
// github.com/owner/repo, and a bare owner/repo. Every other host is rejected.
func ParseGitHubRemote(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrUnparsableRemote
	}

	// A bare owner/repo carries no host to check, so take it as GitHub.
	if !strings.Contains(s, "://") && !strings.Contains(s, "@") && strings.Count(s, "/") == 1 {
		return normalizeRemotePath(s)
	}

	var hostAndPath string
	switch {
	case strings.Contains(s, "://"):
		hostAndPath = s[strings.Index(s, "://")+3:]
	case strings.Contains(s, ":") && !strings.HasPrefix(s, "/"):
		// scp-style [user@]host:path. The colon separates host from path and is
		// not a port, which is why this can't go through net/url.
		hostPart := s[strings.LastIndex(s, "@")+1:]
		colon := strings.Index(hostPart, ":")
		if colon < 0 {
			return "", ErrUnparsableRemote
		}
		hostAndPath = hostPart[:colon] + "/" + hostPart[colon+1:]
	default:
		hostAndPath = s
	}

	if at := strings.LastIndex(hostAndPath, "@"); at >= 0 {
		hostAndPath = hostAndPath[at+1:]
	}
	slash := strings.Index(hostAndPath, "/")
	if slash < 0 {
		return "", ErrUnparsableRemote
	}
	host, path := hostAndPath[:slash], hostAndPath[slash+1:]
	if colon := strings.Index(host, ":"); colon >= 0 {
		host = host[:colon]
	}
	if host = strings.ToLower(host); host != "github.com" && host != "www.github.com" {
		return "", ErrNotGitHub
	}
	return normalizeRemotePath(path)
}

func normalizeRemotePath(path string) (string, error) {
	path = strings.Trim(strings.TrimSuffix(strings.Trim(path, "/"), ".git"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", ErrUnparsableRemote
	}
	return parts[0] + "/" + parts[1], nil
}
