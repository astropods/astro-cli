// Package git reads the local checkout's GitHub remote and branch by shelling
// out to git. The CLI has no git library dependency and needs only three facts,
// so the exec calls stay here and the parsing beside them stays pure.
package git

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

var (
	// ErrNotARepo means the directory is not inside a git working tree.
	ErrNotARepo = errors.New("git: not a repository")
	// ErrNoOrigin means the repository has no remote named origin.
	ErrNoOrigin = errors.New("git: no origin remote")
	// ErrDetachedHead means HEAD does not name a branch, so there is no branch
	// to build from.
	ErrDetachedHead = errors.New("git: HEAD is detached")
	// ErrNotGitHub means the remote points somewhere the platform cannot build
	// from. GitHub is the only host with a push webhook and a build pipeline.
	ErrNotGitHub = errors.New("git: remote is not a github.com URL")
	// ErrUnparsableRemote means the URL is a github.com address whose path is
	// not owner/repo.
	ErrUnparsableRemote = errors.New("git: cannot read owner/repo from remote")
)

// OriginURL returns the URL of the origin remote for the repository containing
// dir. An empty dir means the process working directory.
func OriginURL(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		// git distinguishes the two in its exit status, but not portably enough
		// to rely on, so ask whether this is a repository at all.
		if _, repoErr := run(ctx, dir, "rev-parse", "--git-dir"); repoErr != nil {
			return "", ErrNotARepo
		}
		return "", ErrNoOrigin
	}
	if out == "" {
		return "", ErrNoOrigin
	}
	return out, nil
}

// CurrentBranch returns the branch HEAD points at for the repository containing
// dir. An empty dir means the process working directory.
func CurrentBranch(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", ErrNotARepo
	}
	// A detached HEAD abbreviates to the literal "HEAD", and a repository with
	// no commits yet fails the call above.
	if out == "" || out == "HEAD" {
		return "", ErrDetachedHead
	}
	return out, nil
}

// ParseGitHubRemote reduces a git remote URL to its "owner/repo" form.
//
// It accepts the shapes git prints for a GitHub remote — scp-style
// (git@github.com:owner/repo.git), ssh:// and https:// URLs, a bare
// github.com/owner/repo, and a bare owner/repo — and rejects every other host.
func ParseGitHubRemote(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrUnparsableRemote
	}

	// A bare owner/repo carries no host to check, so take it as GitHub. Anything
	// with a scheme, an "@", or more than one slash goes through host parsing.
	if !strings.Contains(s, "://") && !strings.Contains(s, "@") && strings.Count(s, "/") == 1 {
		return normalizePath(s)
	}

	var hostAndPath string
	switch {
	case strings.Contains(s, "://"):
		// scheme://[user[:pass]@]host[:port]/path
		rest := s[strings.Index(s, "://")+3:]
		hostAndPath = rest
	case strings.Contains(s, ":") && !strings.HasPrefix(s, "/"):
		// scp-style: [user@]host:path — the colon separates host from path and
		// is not a port, which is why this cannot go through net/url.
		at := strings.LastIndex(s, "@")
		hostPart := s[at+1:]
		colon := strings.Index(hostPart, ":")
		if colon < 0 {
			return "", ErrUnparsableRemote
		}
		hostAndPath = hostPart[:colon] + "/" + hostPart[colon+1:]
	default:
		hostAndPath = s
	}

	// Drop any userinfo the scheme branch left behind.
	if at := strings.LastIndex(hostAndPath, "@"); at >= 0 {
		hostAndPath = hostAndPath[at+1:]
	}

	slash := strings.Index(hostAndPath, "/")
	if slash < 0 {
		return "", ErrUnparsableRemote
	}
	host := hostAndPath[:slash]
	path := hostAndPath[slash+1:]

	// Strip a port, and compare case-insensitively: hostnames are not
	// case-sensitive and a remote may be written GitHub.com.
	if colon := strings.Index(host, ":"); colon >= 0 {
		host = host[:colon]
	}
	host = strings.ToLower(host)
	if host != "github.com" && host != "www.github.com" {
		return "", ErrNotGitHub
	}

	return normalizePath(path)
}

// normalizePath reduces a remote's path to owner/repo.
func normalizePath(path string) (string, error) {
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")

	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", ErrUnparsableRemote
	}
	return parts[0] + "/" + parts[1], nil
}

// run executes git and returns its trimmed stdout.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	out, err := exec.CommandContext(ctx, "git", args...).Output() //nolint:gosec // fixed subcommands; dir is the caller's working directory, never user input
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
