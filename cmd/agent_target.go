package cmd

import (
	"context"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func agentTargetArgs(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		return errAgentUnexpectedArgument(args[0])
	}
	return nil
}

func deploymentLabel(dep *agentDeployment) string {
	if dep.DisplayName != "" {
		return dep.DisplayName
	}
	return dep.Name
}

func registerAgentTargetFlags(cmd *cobra.Command) {
	cmd.Flags().String("name", "", "Display name or blueprint name (from agent list; not a deployment ID)")
	cmd.Flags().String("id", "", "Deployment ID (from agent list)")
	cmd.Flags().String("env", "", "Environment the agent runs in (with --blueprint)")
	cmd.Flags().StringP("blueprint", "b", "", "Blueprint of the agent (with --env)")
	cmd.MarkFlagsOneRequired("name", "id", "env")       //nolint:errcheck,gosec
	cmd.MarkFlagsMutuallyExclusive("name", "id", "env") //nolint:errcheck,gosec
	cmd.MarkFlagsRequiredTogether("env", "blueprint")   //nolint:errcheck,gosec
}

func resolveAgentTarget(cmd *cobra.Command, at AccountToken, verbose bool) (*agentDeployment, error) {
	if id, _ := cmd.Flags().GetString("id"); id != "" {
		return fetchAgentDeploymentSummary(cmd.Context(), id, at, verbose)
	}
	if env := flagString(cmd, "env"); env != "" {
		blueprint := flagString(cmd, "blueprint")
		if blueprint == "" {
			return nil, errEnvironmentNeedsBlueprint()
		}
		e, err := findBlueprintEnvironment(cmd.Context(), at, blueprint, env, verbose)
		if err != nil {
			return nil, err
		}
		if e.DeploymentID == "" {
			return nil, errEnvironmentHasNoAgent(env, blueprint)
		}
		return fetchAgentDeploymentSummary(cmd.Context(), e.DeploymentID, at, verbose)
	}

	name, _ := cmd.Flags().GetString("name")
	if name == "" {
		return nil, errAgentTargetRequired()
	}

	return findDeploymentByTarget(cmd, name, at, verbose)
}

func fetchAgentDeploymentSummary(ctx context.Context, id string, at AccountToken, verbose bool) (*agentDeployment, error) {
	full, err := getAgentDeploymentFull(ctx, id, at, verbose)
	if err != nil {
		return nil, errAgentDeploymentNotFoundForID(id)
	}
	return &agentDeployment{
		ID:              full.ID,
		Name:            full.Name,
		DisplayName:     full.DisplayName,
		BuildID:         full.BuildID,
		Namespace:       full.Namespace,
		Status:          full.Status,
		CreatedAt:       full.CreatedAt,
		EnvironmentID:   full.EnvironmentID,
		EnvironmentName: full.EnvironmentName,
	}, nil
}

func findDeploymentByTarget(cmd *cobra.Command, target string, at AccountToken, verbose bool) (*agentDeployment, error) {
	u := agentBaseURL() + "/api/v1/deployments?account=" + url.QueryEscape(at.Account)
	var result listDeploymentsResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return nil, err
	}
	var byBlueprint []*agentDeployment
	for i := range result.Deployments {
		d := &result.Deployments[i]
		if d.DisplayName == target {
			return d, nil
		}
		if d.Name == target {
			byBlueprint = append(byBlueprint, d)
		}
	}
	switch len(byBlueprint) {
	case 0:
		return nil, errAgentDeploymentNotFound(target)
	case 1:
		return byBlueprint[0], nil
	}
	matches := make([]string, len(byBlueprint))
	for i, d := range byBlueprint {
		matches[i] = agentTargetMatch(d)
	}
	return nil, errAgentTargetAmbiguous(target, matches)
}

func agentTargetMatch(d *agentDeployment) string {
	label := deploymentLabel(d) + "  --id " + d.ID
	if d.EnvironmentName != "" {
		label += "  --env " + d.EnvironmentName
	}
	return label
}
