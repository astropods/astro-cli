package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/githubcli"
)

// Package-level seams so tests can fake gh/git without a real gh binary.
var (
	ghRun           = githubcli.GH
	ghAuthenticated = githubcli.GHAuthenticated
	ghPush          = githubcli.Push
	ghCurrentBranch = githubcli.CurrentBranch
)

// ghAuthCheckTimeout bounds `gh auth status`, which validates the token
// against GitHub's API. A var so a test can shorten it rather than sleep out
// the real value.
var ghAuthCheckTimeout = 10 * time.Second

// devSessionToken returns the dev session's GitHub connection token, or ""
// to use the operator's own local credentials.
func devSessionToken(cmd *cobra.Command) string {
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	return resolveGitHubToken(cmd.Context(), verbose)
}

// ghAuth returns the token a gh-based subcommand should use: the dev
// session's connection when open, else the operator's own `gh auth login`,
// verified to work. Push doesn't call this; it only runs git.
func ghAuth(cmd *cobra.Command) (token string, err error) {
	token = devSessionToken(cmd)
	if token != "" {
		return token, nil
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), ghAuthCheckTimeout)
	defer cancel()
	if err := ghAuthenticated(ctx, ""); err != nil {
		var notInstalled githubcli.ErrNotInstalled
		if errors.As(err, &notInstalled) {
			return "", errGHNotInstalled()
		}
		// The check reaches GitHub's API, so a proxy, captive portal or
		// outage trips the deadline. Reporting that as "not authenticated"
		// tells an operator whose login is fine to go and re-login.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", errGHAuthCheckTimedOut(err)
		}
		if devSessionActive() {
			return "", errGHDevSessionTokenUnavailable(err)
		}
		return "", errGHNotAuthenticated(err)
	}
	return "", nil
}

func runGHPassthrough(cmd *cobra.Command, args ...string) error {
	token, err := ghAuth(cmd)
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	result, err := ghRun(cmd.Context(), "", token, args...)
	writePassthrough(w, result.Stdout)
	if err != nil {
		return classifyGitHubError(err, token)
	}
	return nil
}

// classifyGitHubError maps a githubcli failure to the message naming its real
// cause. Every call site routes failures through here, because the one that
// did not reported a missing binary as a credential problem: ghAuth returns
// before its own not-installed check whenever a dev session supplies a token,
// so a sandbox with a connection but no gh binary was told its connection was
// at fault.
//
// Order matters: a missing binary and a cancelled command are both true
// regardless of which credential was in play, so they are named before the
// credential source is.
func classifyGitHubError(err error, token string) error {
	var notInstalled githubcli.ErrNotInstalled
	if errors.As(err, &notInstalled) {
		if notInstalled.Name == "git" {
			return errGitNotInstalled()
		}
		return errGHNotInstalled()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errGitHubCommandInterrupted(err)
	}
	// gh's own error text may suggest `gh auth login`, which has no effect
	// once GH_TOKEN is set, so name the credential actually in use.
	if token != "" {
		return errGitHubCommandFailedWithDevSessionToken(err)
	}
	return errGitHubCommandFailed(err)
}

// validateRemoteName keeps --remote to a configured remote's name. git also
// accepts a URL in that position, which would let a caller send the branch to
// a repository the operator never configured. `github push` is reachable as an
// MCP write tool, alongside read tools that can fetch a PR or issue body an
// attacker wrote, so the value is not always the operator's own. Pushing to a
// URL is not part of this command; a remote has to be configured first.
func validateRemoteName(remote string) error {
	if remote == "" {
		return errGitHubRemoteEmpty()
	}
	// ":" catches scp-style (git@host:owner/repo) and any scheme; "/" catches
	// a path. git remote names permit neither.
	if strings.ContainsAny(remote, ":/") {
		return errGitHubRemoteNotAName(remote)
	}
	return nil
}

// withRepoFlag appends gh's own -R flag when --repo was given.
func withRepoFlag(cmd *cobra.Command, ghArgs []string) []string {
	if repo, _ := cmd.Flags().GetString("repo"); repo != "" {
		return append(ghArgs, "-R", repo)
	}
	return ghArgs
}

func writePassthrough(w io.Writer, s string) {
	if s == "" {
		return
	}
	fmt.Fprint(w, s) //nolint:errcheck,gosec
	if s[len(s)-1] != '\n' {
		fmt.Fprintln(w) //nolint:errcheck,gosec
	}
}

// Default field sets for --json. gh requires a field list and errors with
// the available names when given none, so a boolean --json has to pick one.
// Every name here is from the respective `gh <cmd> --json` listing.
const (
	prViewJSONFields    = "number,title,state,author,url,body,isDraft,headRefName,baseRefName,reviewDecision,createdAt,updatedAt"
	prChecksJSONFields  = "name,state,bucket,description,workflow,event,link,startedAt,completedAt"
	issueViewJSONFields = "number,title,state,author,url,body,labels,assignees,createdAt,updatedAt"
)

// jsonFields resolves --json/--fields to the list handed to gh, or "" for
// gh's own human view. --json is a bool here to match the other ~40 commands
// that carry one; --fields keeps gh's field selection available and implies
// --json, so neither has to be given with the other.
func jsonFields(cmd *cobra.Command, defaults string) string {
	if fields, _ := cmd.Flags().GetString("fields"); fields != "" {
		return fields
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return defaults
	}
	return ""
}

// runGHWithOptionalNumber builds `base [--json fields] [-R repo] [-- number]`
// and runs it: the shape shared by pr view, pr checks, and issue view.
func runGHWithOptionalNumber(cmd *cobra.Command, base []string, args []string, defaultFields string) error {
	ghArgs := base
	if fields := jsonFields(cmd, defaultFields); fields != "" {
		ghArgs = append(ghArgs, "--json", fields)
	}
	ghArgs = githubcli.WithPositional(withRepoFlag(cmd, ghArgs), args...)
	return runGHPassthrough(cmd, ghArgs...)
}

func runGitHubPRView(cmd *cobra.Command, args []string) error {
	return runGHWithOptionalNumber(cmd, []string{"pr", "view"}, args, prViewJSONFields)
}

func runGitHubPRCreate(cmd *cobra.Command, _ []string) error {
	title, _ := cmd.Flags().GetString("title")
	body, _ := cmd.Flags().GetString("body")
	base, _ := cmd.Flags().GetString("base")
	head, _ := cmd.Flags().GetString("head")
	draft, _ := cmd.Flags().GetBool("draft")

	ghArgs := []string{"pr", "create", "--title", title, "--body", body}
	if base != "" {
		ghArgs = append(ghArgs, "--base", base)
	}
	if head != "" {
		ghArgs = append(ghArgs, "--head", head)
	}
	if draft {
		ghArgs = append(ghArgs, "--draft")
	}
	return runGHPassthrough(cmd, withRepoFlag(cmd, ghArgs)...)
}

func runGitHubPRComment(cmd *cobra.Command, args []string) error {
	body, _ := cmd.Flags().GetString("body")
	ghArgs := []string{"pr", "comment", "--body", body}
	ghArgs = githubcli.WithPositional(withRepoFlag(cmd, ghArgs), args...)
	return runGHPassthrough(cmd, ghArgs...)
}

func runGitHubPRChecks(cmd *cobra.Command, args []string) error {
	return runGHWithOptionalNumber(cmd, []string{"pr", "checks"}, args, prChecksJSONFields)
}

func runGitHubIssueView(cmd *cobra.Command, args []string) error {
	return runGHWithOptionalNumber(cmd, []string{"issue", "view"}, args, issueViewJSONFields)
}

func runGitHubPush(cmd *cobra.Command, _ []string) error {
	token := devSessionToken(cmd)
	remote, _ := cmd.Flags().GetString("remote")
	if err := validateRemoteName(remote); err != nil {
		return err
	}
	setUpstream, _ := cmd.Flags().GetBool("set-upstream")

	ctx := cmd.Context()
	branch, err := ghCurrentBranch(ctx, "")
	if err != nil {
		// No token: resolving the branch runs plain git, never the connection.
		return classifyGitHubError(err, "")
	}
	if branch == "HEAD" {
		return errGitHubDetachedHead()
	}

	w := cmd.OutOrStdout()
	result, err := ghPush(ctx, "", token, remote, branch, setUpstream)
	writePassthrough(w, result.Stdout)
	if err != nil {
		// runCommand's error already carries stderr; printing it here too
		// would show it twice.
		return classifyGitHubError(err, token)
	}
	// git push writes its own hints (e.g. "Create a pull request for...")
	// to stderr even on success.
	writePassthrough(w, result.Stderr)
	return nil
}
