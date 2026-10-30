package cmd

import (
	"github.com/spf13/cobra"
)

var githubCmd = &cobra.Command{
	Use:   "github",
	Short: "Work with GitHub pull requests and issues",
	Long: `Work with GitHub pull requests and issues.

These commands shell out to the locally installed gh and git. The pr and
issue commands run gh, so run ` + "`gh auth login`" + ` first; push runs only git,
using its own credentials. Inside an ` + "`ast dev`" + ` session that shared a
GitHub connection, every command uses that connection's token instead.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var githubPRCmd = &cobra.Command{
	Use:   "pr",
	Short: "Inspect and act on pull requests",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var githubPRViewCmd = &cobra.Command{
	Use:   "view [number]",
	Short: "Show a pull request",
	Long:  "Show a pull request. Without a number, shows the one associated with the current branch.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runGitHubPRView,
}

var githubPRCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Open a pull request",
	Args:  cobra.NoArgs,
	RunE:  runGitHubPRCreate,
}

var githubPRCommentCmd = &cobra.Command{
	Use:   "comment <number>",
	Short: "Comment on a pull request",
	Args:  cobra.ExactArgs(1),
	RunE:  runGitHubPRComment,
}

var githubPRChecksCmd = &cobra.Command{
	Use:   "checks [number]",
	Short: "Show a pull request's CI checks",
	Long:  "Show a pull request's CI checks. Without a number, shows the one associated with the current branch.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runGitHubPRChecks,
}

var githubIssueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Inspect issues",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var githubIssueViewCmd = &cobra.Command{
	Use:   "view <number>",
	Short: "Show an issue",
	Args:  cobra.ExactArgs(1),
	RunE:  runGitHubIssueView,
}

var githubPushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push the current branch",
	Args:  cobra.NoArgs,
	RunE:  runGitHubPush,
}

func init() {
	rootCmd.AddCommand(githubCmd)
	githubCmd.AddCommand(githubPRCmd)
	githubCmd.AddCommand(githubIssueCmd)
	githubCmd.AddCommand(githubPushCmd)
	githubPRCmd.AddCommand(githubPRViewCmd)
	githubPRCmd.AddCommand(githubPRCreateCmd)
	githubPRCmd.AddCommand(githubPRCommentCmd)
	githubPRCmd.AddCommand(githubPRChecksCmd)
	githubIssueCmd.AddCommand(githubIssueViewCmd)

	githubPRViewCmd.Flags().Bool("json", false, "Print raw JSON output")
	githubPRViewCmd.Flags().String("fields", "", "Print only these JSON fields (comma-separated, e.g. number,title,state); implies --json")
	githubPRViewCmd.Flags().String("repo", "", "Target repository as OWNER/REPO (defaults to the current directory's remote)")

	githubPRCreateCmd.Flags().String("title", "", "Pull request title (required)")
	githubPRCreateCmd.Flags().String("body", "", "Pull request body (required)")
	githubPRCreateCmd.Flags().String("base", "", "Base branch (defaults to the repository's default branch)")
	githubPRCreateCmd.Flags().String("head", "", "Head branch (defaults to the current branch)")
	githubPRCreateCmd.Flags().Bool("draft", false, "Open as a draft")
	githubPRCreateCmd.Flags().String("repo", "", "Target repository as OWNER/REPO (defaults to the current directory's remote)")
	githubPRCreateCmd.MarkFlagRequired("title") //nolint:errcheck,gosec
	githubPRCreateCmd.MarkFlagRequired("body")  //nolint:errcheck,gosec

	githubPRCommentCmd.Flags().String("body", "", "Comment body (required)")
	githubPRCommentCmd.Flags().String("repo", "", "Target repository as OWNER/REPO (defaults to the current directory's remote)")
	githubPRCommentCmd.MarkFlagRequired("body") //nolint:errcheck,gosec

	githubPRChecksCmd.Flags().Bool("json", false, "Print raw JSON output")
	githubPRChecksCmd.Flags().String("fields", "", "Print only these JSON fields (comma-separated, e.g. name,state,bucket); implies --json")
	githubPRChecksCmd.Flags().String("repo", "", "Target repository as OWNER/REPO (defaults to the current directory's remote)")

	githubIssueViewCmd.Flags().Bool("json", false, "Print raw JSON output")
	githubIssueViewCmd.Flags().String("fields", "", "Print only these JSON fields (comma-separated, e.g. number,title,state); implies --json")
	githubIssueViewCmd.Flags().String("repo", "", "Target repository as OWNER/REPO (defaults to the current directory's remote)")

	githubPushCmd.Flags().String("remote", "origin", "Remote to push to")
	githubPushCmd.Flags().Bool("set-upstream", false, "Set the pushed branch as its upstream")
}
