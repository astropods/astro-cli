package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
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
// against GitHub's API, so a slow or blocked network can't hang a command
// indefinitely.
const ghAuthCheckTimeout = 10 * time.Second

// resolveToken returns the dev session's GitHub connection token, or "" to
// use the operator's own local credentials.
func resolveToken(cmd *cobra.Command) string {
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	return resolveGitHubToken(cmd.Context(), verbose)
}

// ghAuth returns the token a gh-based subcommand should use, preferring the
// dev session's GitHub connection; with none open, it verifies the
// operator's own `gh auth login` actually works, so the command fails with
// one clear message instead of gh's own cryptic prompt. Push does not call
// this: it only ever runs git, and git has its own credential resolution
// (SSH key, credential helper, ...) independent of whether gh is installed.
func ghAuth(cmd *cobra.Command) (token string, err error) {
	token = resolveToken(cmd)
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
		return errGitHubCommandFailed(err)
	}
	return nil
}

// withRepoFlag appends gh's own -R flag when --repo was given, so these
// commands can target a repository other than the one in the working
// directory, the same way a plain gh invocation would.
func withRepoFlag(cmd *cobra.Command, ghArgs []string) []string {
	if repo, _ := cmd.Flags().GetString("repo"); repo != "" {
		return append(ghArgs, "-R", repo)
	}
	return ghArgs
}

// withPositional appends a `--` separator before any positional args, so gh
// never reinterprets one as a flag if it starts with "-". PR/issue numbers
// are always plain positive integers from GitHub itself, but nothing
// enforces that before this point, and --remote on Push showed live that a
// caller-supplied value reinterpreted as a flag is a real problem, not a
// theoretical one. githubcli.Push applies the same `--` to remote/branch
// independently (git, not gh); any new bare positional anywhere needs this
// same treatment.
func withPositional(ghArgs []string, positional []string) []string {
	if len(positional) == 0 {
		return ghArgs
	}
	return append(append(ghArgs, "--"), positional...)
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

func runGitHubPRView(cmd *cobra.Command, args []string) error {
	ghArgs := []string{"pr", "view"}
	if fields, _ := cmd.Flags().GetString("json"); fields != "" {
		ghArgs = append(ghArgs, "--json", fields)
	}
	ghArgs = withPositional(withRepoFlag(cmd, ghArgs), args)
	return runGHPassthrough(cmd, ghArgs...)
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
	ghArgs = withPositional(withRepoFlag(cmd, ghArgs), args)
	return runGHPassthrough(cmd, ghArgs...)
}

func runGitHubPRChecks(cmd *cobra.Command, args []string) error {
	ghArgs := []string{"pr", "checks"}
	if fields, _ := cmd.Flags().GetString("json"); fields != "" {
		ghArgs = append(ghArgs, "--json", fields)
	}
	ghArgs = withPositional(withRepoFlag(cmd, ghArgs), args)
	return runGHPassthrough(cmd, ghArgs...)
}

func runGitHubIssueView(cmd *cobra.Command, args []string) error {
	ghArgs := []string{"issue", "view"}
	if fields, _ := cmd.Flags().GetString("json"); fields != "" {
		ghArgs = append(ghArgs, "--json", fields)
	}
	ghArgs = withPositional(withRepoFlag(cmd, ghArgs), args)
	return runGHPassthrough(cmd, ghArgs...)
}

func runGitHubPush(cmd *cobra.Command, _ []string) error {
	token := resolveToken(cmd)
	remote, _ := cmd.Flags().GetString("remote")
	setUpstream, _ := cmd.Flags().GetBool("set-upstream")

	ctx := cmd.Context()
	branch, err := ghCurrentBranch(ctx, "")
	if err != nil {
		return errGitHubCommandFailed(err)
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
		return errGitHubCommandFailed(err)
	}
	// git push writes its own hints (e.g. "Create a pull request for...")
	// to stderr even on success.
	writePassthrough(w, result.Stderr)
	return nil
}
