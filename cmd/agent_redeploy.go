package cmd

import (
	"net/http"

	"github.com/spf13/cobra"
)

var agentRedeployCmd = &cobra.Command{
	Use:   "redeploy",
	Short: "Redeploy an existing agent",
	Args:  agentTargetArgs,
	RunE:  runAgentRedeploy,
}

func init() {
	agentCmd.AddCommand(agentRedeployCmd)
	registerDeployCommonFlags(agentRedeployCmd)
	registerAgentTargetFlags(agentRedeployCmd)
	agentRedeployCmd.Flags().Bool("latest", false, "Redeploy the blueprint's newest published build")
}

func runAgentRedeploy(cmd *cobra.Command, args []string) error {
	_, cancel, err := orchestrationContext(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	schedules, err := parseDeploySchedulesFromCmd(cmd)
	if err != nil {
		return err
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	adapters, _ := cmd.Flags().GetStringArray("adapter")
	build, _ := cmd.Flags().GetString("build")
	latest, _ := cmd.Flags().GetBool("latest")
	clusterID, _ := cmd.Flags().GetString("cluster")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if _, err := buildCommit(cmd, build); err != nil {
		return err
	}
	if latest && build != "" {
		return errRedeployLatestWithBuild()
	}

	// A nil interfaces block preserves the deployment's stored adapters.
	var iface *deployTemplateInterfaces
	if len(adapters) > 0 {
		iface, err = buildDeployInterfaces(adapters)
		if err != nil {
			return err
		}
	}

	vars, err := parseDeployVarsFromCmd(cmd)
	if err != nil {
		return err
	}

	commit, _ := cmd.Flags().GetString("commit")
	wait, _ := cmd.Flags().GetBool("wait")
	dep, err := resolveAgentDeploymentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}
	if (commit != "" || wait) && dep.CurrentRevision < 1 {
		return errDeploymentRevisionUnsupported()
	}

	if latest {
		buildPublisher := dep.SourceAccount
		if buildPublisher == "" {
			buildPublisher = at.Account
		}
		build, err = latestBlueprintBuild(cmd, at, buildPublisher, dep.Name, verbose)
		if err != nil {
			return err
		}
	}

	req := deployTemplateRequest{
		Build:        build,
		DeploymentID: dep.ID,
		Interfaces:   iface,
		ClusterID:    clusterID,
	}
	if len(vars) > 0 {
		req.Variables = vars
	}
	if len(schedules) > 0 {
		req.Schedules = schedules
	}

	// The URL account scopes knowledge bindings; the server finds the publisher from the deployment.
	return runDeployWithRequest(cmd, at, verbose, deployOptions{
		TemplateAccount: at.Account, Blueprint: dep.Name, DisplayName: dep.DisplayName,
		BuildPublisher: dep.SourceAccount, ExpectedRevision: dep.CurrentRevision,
		Request: req, DryRun: dryRun,
	})
}

func latestBlueprintBuild(cmd *cobra.Command, at AccountToken, publisher, name string, verbose bool) (string, error) {
	u := apiPath(blueprintBaseURL(), publisher, "agents", name)
	var bp blueprintItem
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &bp)
	if isNotFound(status, err) {
		return "", errBlueprintNotFound(name, publisher)
	}
	if err != nil {
		return "", err
	}
	v := blueprintLatestVersion(bp.Versions)
	if v == nil || v.BuildID == "" {
		return "", errBlueprintNoPublishedBuild(name)
	}
	return v.BuildID, nil
}
