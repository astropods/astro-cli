package cmd

import (
	"errors"
	"fmt"
	"io"

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

// ghAuth returns the token a github subcommand should use, preferring the
// dev session's GitHub connection and falling back to the operator's own
// `gh auth login`, which it verifies is actually set up.
func ghAuth(cmd *cobra.Command) (token string, err error) {
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	ctx := cmd.Context()

	token = resolveGitHubToken(ctx, verbose)
	if token != "" {
		return token, nil
	}
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
	ghArgs = append(ghArgs, args...)
	if fields, _ := cmd.Flags().GetString("json"); fields != "" {
		ghArgs = append(ghArgs, "--json", fields)
	}
	return runGHPassthrough(cmd, withRepoFlag(cmd, ghArgs)...)
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
	ghArgs := []string{"pr", "comment", args[0], "--body", body}
	return runGHPassthrough(cmd, withRepoFlag(cmd, ghArgs)...)
}

func runGitHubPRChecks(cmd *cobra.Command, args []string) error {
	ghArgs := []string{"pr", "checks"}
	ghArgs = append(ghArgs, args...)
	if fields, _ := cmd.Flags().GetString("json"); fields != "" {
		ghArgs = append(ghArgs, "--json", fields)
	}
	return runGHPassthrough(cmd, withRepoFlag(cmd, ghArgs)...)
}

func runGitHubIssueView(cmd *cobra.Command, args []string) error {
	ghArgs := []string{"issue", "view", args[0]}
	if fields, _ := cmd.Flags().GetString("json"); fields != "" {
		ghArgs = append(ghArgs, "--json", fields)
	}
	return runGHPassthrough(cmd, withRepoFlag(cmd, ghArgs)...)
}

func runGitHubPush(cmd *cobra.Command, _ []string) error {
	token, err := ghAuth(cmd)
	if err != nil {
		return err
	}
	remote, _ := cmd.Flags().GetString("remote")
	setUpstream, _ := cmd.Flags().GetBool("set-upstream")

	ctx := cmd.Context()
	branch, err := ghCurrentBranch(ctx, "")
	if err != nil {
		return errGitHubCommandFailed(err)
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
