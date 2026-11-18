// Package githubcli shells out to the locally installed `git` and `gh`
// binaries so CLI commands can act on GitHub without astro-cli reimplementing
// a GitHub API client. Every caller supplies its own credential: an empty
// token leaves `gh`/`git` to use the operator's own local login; a non-empty
// token (minted from an Astro GitHub connection) overrides it for that one
// call only, never touching the operator's stored credentials.
package githubcli

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Result struct {
	Stdout string
	Stderr string
}

// runFunc runs name with args in dir, with env appended to the inherited
// environment. Tests substitute a fake to avoid shelling out for real.
type runFunc func(ctx context.Context, dir string, env []string, name string, args ...string) (Result, error)

var run runFunc = runCommand

// waitDelay bounds the pipe drain after a context deadline kills the
// direct child, in case it spawned its own child that inherited the pipe.
const waitDelay = 5 * time.Second

func runCommand(ctx context.Context, dir string, env []string, name string, args ...string) (Result, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // argv-based, never shell-interpolated
	cmd.Dir = dir
	cmd.WaitDelay = waitDelay
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return result, ErrNotInstalled{Name: name}
		}
		// A context kill arrives as an ExitError ("signal: killed"), so
		// ctx.Err() is the only thing distinguishing it from a real exit.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), ctxErr)
		}
		wrapped := fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			wrapped = fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, detail)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return result, ErrExit{Code: exitErr.ExitCode(), Err: wrapped}
		}
		return result, wrapped
	}
	return result, nil
}

// WithPositional appends a `--` separator before any positional args, so
// the invoked binary never reinterprets one as a flag if it starts with
// "-". Call at most once per argv: a second `--` is itself a positional.
func WithPositional(argv []string, positional ...string) []string {
	if len(positional) == 0 {
		return argv
	}
	return append(append(argv, "--"), positional...)
}

// ErrNotInstalled means the binary itself could not be found on PATH.
type ErrNotInstalled struct{ Name string }

func (e ErrNotInstalled) Error() string { return e.Name + " is not installed or not on PATH" }

// ErrExit carries the child's own exit status, which some of gh's codes give
// meaning to: `gh pr checks` exits 8 for "checks pending", 1 for "a check
// failed".
type ErrExit struct {
	Code int
	Err  error
}

func (e ErrExit) Error() string { return e.Err.Error() }
func (e ErrExit) Unwrap() error { return e.Err }

// ExitCode is the child's status, or 0 when it could not be determined.
func (e ErrExit) ExitCode() int { return e.Code }

// GH runs `gh` with args in dir. A non-empty token is passed as GH_TOKEN,
// which gh reads in preference to its own stored login.
func GH(ctx context.Context, dir, token string, args ...string) (Result, error) {
	env := []string{"GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1"}
	if token != "" {
		env = append(env, "GH_TOKEN="+token)
	}
	return run(ctx, dir, env, "gh", args...)
}

// GHAuthenticated reports whether the operator has their own `gh auth login` session.
func GHAuthenticated(ctx context.Context, dir string) error {
	_, err := run(ctx, dir, nil, "gh", "auth", "status")
	return err
}

// Push runs `git push` in dir. A non-empty token authenticates via env
// (GIT_CONFIG_*, git 2.31+), never argv: runCommand's error formatting
// joins args into its message, so a credential placed there would leak.
func Push(ctx context.Context, dir, token, remote, branch string, setUpstream bool) (Result, error) {
	// git prompts for credentials on /dev/tty, which the child's nil stdin
	// does not close off.
	env := []string{"GIT_TERMINAL_PROMPT=0"}
	if token != "" {
		env = append(env, appendGitConfigEnv("http.https://github.com/.extraheader", "AUTHORIZATION: basic "+basicAuthHeader(token))...)
	}
	args := []string{"push"}
	if setUpstream {
		args = append(args, "--set-upstream")
	}
	args = WithPositional(args, remote, branch) // remote is caller-supplied
	return run(ctx, dir, env, "git", args...)
}

// Remotes lists the repository's configured remote names.
func Remotes(ctx context.Context, dir string) ([]string, error) {
	result, err := run(ctx, dir, nil, "git", "remote")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(result.Stdout, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func CurrentBranch(ctx context.Context, dir string) (string, error) {
	result, err := run(ctx, dir, nil, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

func basicAuthHeader(token string) string {
	return base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
}

// appendGitConfigEnv adds one git config value via the GIT_CONFIG_COUNT/
// KEY_n/VALUE_n env vars, at the next free index after whatever the
// caller's own environment already declares.
func appendGitConfigEnv(key, value string) []string {
	n, _ := strconv.Atoi(os.Getenv("GIT_CONFIG_COUNT"))
	if n < 0 {
		n = 0
	}
	return []string{
		fmt.Sprintf("GIT_CONFIG_COUNT=%d", n+1),
		fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", n, key),
		fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", n, value),
	}
}
