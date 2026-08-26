package cmd

import (
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
}

func runAgentRedeploy(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	adapters, _ := cmd.Flags().GetStringArray("adapter")
	build, _ := cmd.Flags().GetString("build")
	clusterID, _ := cmd.Flags().GetString("cluster")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

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

	req := deployTemplateRequest{
		Build:        build,
		DeploymentID: dep.ID,
		Interfaces:   iface,
		ClusterID:    clusterID,
	}
	if len(vars) > 0 {
		req.Variables = vars
	}

	return runDeployWithRequest(cmd, at, verbose, dep.Name, dep.DisplayName, req, dryRun)
}
