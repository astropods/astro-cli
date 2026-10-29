package cmd

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/spf13/cobra"
)

const orchestrationTimeout = 45 * time.Minute

var orchestrationPollInterval = 3 * time.Second

var blueprintBuildsWaitCmd = &cobra.Command{
	Use:   "wait <name>",
	Short: "Wait for an exact build or commit to publish",
	Args:  exactValidAgentName,
	RunE:  runBlueprintBuildsWait,
}

func init() {
	blueprintBuildsCmd.AddCommand(blueprintBuildsWaitCmd)
	blueprintBuildsWaitCmd.Flags().String("build", "", "Build ID to wait for")
	blueprintBuildsWaitCmd.Flags().String("commit", "", "Full Git commit SHA to wait for")
	blueprintBuildsWaitCmd.Flags().Bool("json", false, "Print the published build as JSON")
	blueprintBuildsWaitCmd.Flags().Duration("timeout", orchestrationTimeout, "Maximum time to wait")
}

func buildCommit(cmd *cobra.Command, build string) (string, error) {
	commit, _ := cmd.Flags().GetString("commit")
	latest, _ := cmd.Flags().GetBool("latest")
	if commit == "" {
		return "", nil
	}
	if build != "" || latest {
		return "", errCommitWithBuild()
	}
	if _, err := hex.DecodeString(commit); err != nil || len(commit) != 40 && len(commit) != 64 {
		return "", errInvalidCommit()
	}
	return strings.ToLower(commit), nil
}

func orchestrationContext(cmd *cobra.Command) (context.Context, context.CancelFunc, error) {
	timeout := orchestrationTimeout
	if cmd.Flags().Lookup("timeout") != nil {
		timeout, _ = cmd.Flags().GetDuration("timeout")
	}
	if timeout <= 0 {
		return nil, nil, errWaitTimeout()
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	return ctx, cancel, nil
}

func runBlueprintBuildsWait(cmd *cobra.Command, args []string) error {
	build, _ := cmd.Flags().GetString("build")
	commit, err := buildCommit(cmd, build)
	if err != nil {
		return err
	}
	if build == "" && commit == "" {
		return errBuildSelectorRequired()
	}
	ctx, cancel, err := orchestrationContext(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	result, err := waitForBuild(ctx, at, at.Account, args[0], build, commit, verbose, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(cmd.OutOrStdout(), result)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), msgBuildPublished(result.BuildID, result.CommitSHA))
	return err
}

func waitForBuild(ctx context.Context, at AccountToken, publisher, name, build, commit string, verbose bool, progress io.Writer) (blueprintBuild, error) {
	lastStatus := ""
	for {
		var err error
		at, err = refreshPollingToken(ctx, at)
		if err != nil {
			return blueprintBuild{}, errBuildWaitEnded(name, build, commit, err)
		}
		query := url.Values{"limit": {"1"}}
		if commit != "" {
			query.Set("commit", commit)
		}
		if build != "" {
			query.Set("build_id", build)
		}
		u := apiPath(blueprintBaseURL(), publisher, "agents", name, "builds") + "?" + query.Encode()
		var response blueprintBuildsResponse
		_, err = pollingGET(ctx, u, at.Token, verbose, &response)
		if err != nil {
			return blueprintBuild{}, errBuildWaitEnded(name, build, commit, err)
		}
		if len(response.Builds) > 0 {
			candidate := response.Builds[0]
			if build != "" && candidate.BuildID != build || commit != "" && candidate.CommitSHA != commit {
				return blueprintBuild{}, errBuildSelectorUnsupported()
			}
			if commit != "" && candidate.Source == "cli" && (candidate.WorkingTreeDirty == nil || *candidate.WorkingTreeDirty) {
				return blueprintBuild{}, errBuildSourceUnverified(candidate.BuildID)
			}
			build = candidate.BuildID
			if candidate.Status == "registered" {
				return candidate, nil
			}
			if !isRunningBuild(candidate.Status) {
				return blueprintBuild{}, errBuildNotPublished(build, candidate.Status, candidate.Error)
			}
			if candidate.Status != lastStatus {
				fmt.Fprintln(progress, msgBuildWaiting(build, candidate.Status)) //nolint:errcheck
				lastStatus = candidate.Status
			}
		}
		if err := waitOrchestrationPoll(ctx); err != nil {
			return blueprintBuild{}, errBuildWaitEnded(name, build, commit, err)
		}
	}
}

func waitOrchestrationPoll(ctx context.Context) error {
	return waitWithContext(ctx, orchestrationPollInterval)
}

func refreshPollingToken(ctx context.Context, at AccountToken) (AccountToken, error) {
	if at.ExpiresAt.IsZero() || time.Until(at.ExpiresAt) >= tokenRenewMargin {
		return at, nil
	}
	token, err := getAccountToken(ctx, at.Account)
	if err != nil {
		return at, err
	}
	at.Token = token
	at.ExpiresAt, _ = auth.ParseJWTExpiry(token)
	return at, nil
}
