package cmd

import (
	"github.com/spf13/cobra"
)

var githubCmd = &cobra.Command{
	Use:   "github",
	Short: "Work with GitHub pull requests and issues",
	Long: `Work with GitHub pull requests and issues.

These commands shell out to the locally installed gh and git. Run
` + "`gh auth login`" + ` first, unless this is an ` + "`ast dev`" + ` session that shared a
GitHub connection, in which case that connection is used instead.`,
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

	githubPRViewCmd.Flags().String("json", "", "Print these fields as JSON instead of gh's default view (comma-separated, e.g. number,title,state)")

	githubPRCreateCmd.Flags().String("title", "", "Pull request title (required)")
	githubPRCreateCmd.Flags().String("body", "", "Pull request body (required)")
	githubPRCreateCmd.Flags().String("base", "", "Base branch (defaults to the repository's default branch)")
	githubPRCreateCmd.Flags().String("head", "", "Head branch (defaults to the current branch)")
	githubPRCreateCmd.Flags().Bool("draft", false, "Open as a draft")
	githubPRCreateCmd.MarkFlagRequired("title") //nolint:errcheck,gosec
	githubPRCreateCmd.MarkFlagRequired("body")  //nolint:errcheck,gosec

	githubPRCommentCmd.Flags().String("body", "", "Comment body (required)")
	githubPRCommentCmd.MarkFlagRequired("body") //nolint:errcheck,gosec

	githubPRChecksCmd.Flags().String("json", "", "Print these fields as JSON instead of gh's default view (comma-separated, e.g. name,state,conclusion)")

	githubIssueViewCmd.Flags().String("json", "", "Print these fields as JSON instead of gh's default view (comma-separated, e.g. number,title,state)")

	githubPushCmd.Flags().String("remote", "origin", "Remote to push to")
	githubPushCmd.Flags().Bool("set-upstream", false, "Set the pushed branch as its upstream")
}
