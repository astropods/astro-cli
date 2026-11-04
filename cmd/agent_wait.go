package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

var agentWaitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Wait for a deployment revision without submitting another deployment",
	Args:  cobra.NoArgs,
	RunE:  runAgentWait,
}

func init() {
	agentCmd.AddCommand(agentWaitCmd)
	agentWaitCmd.Flags().String("id", "", "Deployment ID")
	agentWaitCmd.Flags().String("build", "", "Expected build ID")
	agentWaitCmd.Flags().Int("revision", 0, "Expected deployment revision")
	agentWaitCmd.Flags().Bool("json", false, "Print the ready deployment as JSON")
	agentWaitCmd.Flags().Duration("timeout", orchestrationTimeout, "Maximum time to wait")
}

func runAgentWait(cmd *cobra.Command, _ []string) error {
	id, _ := cmd.Flags().GetString("id")
	build, _ := cmd.Flags().GetString("build")
	revision, _ := cmd.Flags().GetInt("revision")
	if id == "" || build == "" || revision < 1 {
		return errDeploymentWaitSelector()
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
	result := agentDeployResult{DeploymentID: id, BuildID: build, Revision: revision}
	if err := waitForDeployment(ctx, result, at, verbose, cmd.ErrOrStderr()); err != nil {
		return err
	}
	result.Status = "active"
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(cmd.OutOrStdout(), result)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), msgDeploymentReady(id, revision))
	return err
}

func waitForDeployment(ctx context.Context, expected agentDeployResult, at AccountToken, verbose bool, progress io.Writer) error {
	lastStatus := ""
	for {
		var err error
		at, err = refreshPollingToken(ctx, at)
		if err != nil {
			return errDeploymentWaitEnded(expected, err)
		}
		dep, err := pollAgentDeploymentFull(ctx, expected.DeploymentID, at, verbose)
		if err != nil {
			return errDeploymentWaitEnded(expected, err)
		}
		if dep.CurrentRevision < 1 {
			return errDeploymentRevisionUnsupported()
		}
		if dep.CurrentRevision != expected.Revision || dep.BuildID != expected.BuildID {
			return errDeploymentSuperseded(expected)
		}
		status := strings.ToLower(dep.Status)
		switch status {
		case "running", "active":
			runtime, err := pollDeploymentRuntime(ctx, expected.DeploymentID, at, verbose)
			if err != nil {
				return errDeploymentWaitEnded(expected, err)
			}
			if runtime.ReadyRevision == expected.Revision {
				return nil
			}
		case "stopped", "suspended", "undeploying", "undeployed", "restricted":
			return errDeploymentStopped(expected.DeploymentID, dep.Status)
		case "failed", "error":
			// The server sets these once its own retries are exhausted, so the
			// expected revision will not become ready on its own.
			return errDeploymentFailed(expected.DeploymentID, dep.Status, dep.ErrorMessage)
		}
		if detail := dep.Status + "\x00" + dep.ErrorMessage; detail != lastStatus {
			fmt.Fprintln(progress, msgDeploymentWaiting(dep.Status, dep.ErrorMessage)) //nolint:errcheck
			lastStatus = detail
		}
		if err := waitOrchestrationPoll(ctx); err != nil {
			return errDeploymentWaitEnded(expected, err)
		}
	}
}
