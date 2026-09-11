package cmd

import (
	"fmt"
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
	grants, _ := cmd.Flags().GetStringArray("grant")
	build, _ := cmd.Flags().GetString("build")
	latest, _ := cmd.Flags().GetBool("latest")
	clusterID, _ := cmd.Flags().GetString("cluster")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if latest && build != "" {
		return fmt.Errorf("--latest and --build are mutually exclusive: --latest resolves the newest build, --build pins one")
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
		iface, err = buildDeployInterfaces(adapters, grants)
		if err != nil {
			return err
		}
	} else if len(grants) > 0 {
		return errGrantNeedsAdapterOnRedeploy()
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
		build, err = latestBlueprintBuild(cmd, at, dep.Name, verbose)
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

	return runDeployWithRequest(cmd, at, verbose, dep.Name, dep.DisplayName, req, dryRun)
}

// latestBlueprintBuild returns the build ID of the blueprint's most recently
// published version.
//
// A redeploy without --build re-runs whatever build the deployment already
// pins, so shipping a fresh push means passing that push's tag back in. The
// tag is only printed in the push banner, which leaves scripted deploys
// scraping it back out of ANSI-formatted output. --latest looks it up instead.
func latestBlueprintBuild(cmd *cobra.Command, at AccountToken, name string, verbose bool) (string, error) {
	u := apiPath(blueprintBaseURL(), at.Account, "agents", name)
	var bp blueprintItem
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &bp)
	if status == http.StatusNotFound {
		return "", fmt.Errorf("blueprint %q not found in account %q", name, at.Account)
	}
	if err != nil {
		return "", err
	}
	v := blueprintLatestVersion(bp.Versions)
	if v == nil || v.BuildID == "" {
		return "", fmt.Errorf("blueprint %q has no published build to redeploy", name)
	}
	return v.BuildID, nil
}
