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
// against GitHub's API.
const ghAuthCheckTimeout = 10 * time.Second

// devSessionToken returns the dev session's GitHub connection token, or ""
// to use the operator's own local credentials.
func devSessionToken(cmd *cobra.Command) string {
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	return resolveGitHubToken(cmd.Context(), verbose)
}

// ghAuth returns the token a gh-based subcommand should use: the dev
// session's GitHub connection when one is open, otherwise the operator's
// own `gh auth login`, verified to actually work. Push does not call this;
// it only runs git, which resolves its own credentials independently of gh.
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
		if token != "" {
			return errGitHubCommandFailedWithDevSessionToken(err)
		}
		return errGitHubCommandFailed(err)
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

func runGitHubPRView(cmd *cobra.Command, args []string) error {
	ghArgs := []string{"pr", "view"}
	if fields, _ := cmd.Flags().GetString("json"); fields != "" {
		ghArgs = append(ghArgs, "--json", fields)
	}
	ghArgs = githubcli.WithPositional(withRepoFlag(cmd, ghArgs), args...)
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
	ghArgs = githubcli.WithPositional(withRepoFlag(cmd, ghArgs), args...)
	return runGHPassthrough(cmd, ghArgs...)
}

func runGitHubPRChecks(cmd *cobra.Command, args []string) error {
	ghArgs := []string{"pr", "checks"}
	if fields, _ := cmd.Flags().GetString("json"); fields != "" {
		ghArgs = append(ghArgs, "--json", fields)
	}
	ghArgs = githubcli.WithPositional(withRepoFlag(cmd, ghArgs), args...)
	return runGHPassthrough(cmd, ghArgs...)
}

func runGitHubIssueView(cmd *cobra.Command, args []string) error {
	ghArgs := []string{"issue", "view"}
	if fields, _ := cmd.Flags().GetString("json"); fields != "" {
		ghArgs = append(ghArgs, "--json", fields)
	}
	ghArgs = githubcli.WithPositional(withRepoFlag(cmd, ghArgs), args...)
	return runGHPassthrough(cmd, ghArgs...)
}

func runGitHubPush(cmd *cobra.Command, _ []string) error {
	token := devSessionToken(cmd)
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
		if token != "" {
			return errGitHubCommandFailedWithDevSessionToken(err)
		}
		return errGitHubCommandFailed(err)
	}
	// git push writes its own hints (e.g. "Create a pull request for...")
	// to stderr even on success.
	writePassthrough(w, result.Stderr)
	return nil
}
