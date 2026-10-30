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

	if latest && build != "" {
		return errRedeployLatestWithBuild()
	}

	// A redeploy with no --adapter must PRESERVE the deployment's existing
	// adapters. buildDeployInterfaces defaults an empty list to
	// {Adapters: ["web"], Auth: web/oidc}, which is right for a first deploy
	// but destructive here: the server overwrites stored adapters whenever
	// req.Interfaces is non-nil (astro-server internal/deployment/template.go,
	// "shaped.Interfaces.Adapters = req.Interfaces.Adapters"). So a flagless
	// redeploy silently dropped a Slack-facing agent's slack adapter, leaving a
	// healthy pod with no Slack connection -- Status Running, URL ready,
	// healthcheck passing, and the app showing "This agent is unavailable" in
	// Slack. Sending nil leaves the stored adapters untouched.
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

	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}

	if latest {
		build, err = latestDeploymentBuild(cmd, at, dep, verbose)
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
	return runDeployWithRequest(cmd, at, verbose, at.Account, dep.Name, dep.DisplayName, nil, req, dryRun)
}

func latestDeploymentBuild(cmd *cobra.Command, at AccountToken, dep *agentDeployment, verbose bool) (string, error) {
	full, err := getAgentDeploymentFull(cmd.Context(), dep.ID, at, verbose)
	if err != nil {
		return "", errAgentDeploymentNotFoundForID(dep.ID)
	}
	publisher := full.SourceAccount
	if publisher == "" {
		publisher = at.Account
	}
	return latestBlueprintBuild(cmd, at, publisher, dep.Name, verbose)
}

// latestBlueprintBuild returns the build ID of the blueprint's most recently
// published version.
//
// A redeploy without --build re-runs whatever build the deployment already
// pins, so shipping a fresh push means passing that push's tag back in. The
// tag is only printed in the push banner, which leaves scripted deploys
// scraping it back out of ANSI-formatted output. --latest looks it up instead.
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
