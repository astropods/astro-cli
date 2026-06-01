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
	cmd.MarkFlagsOneRequired("name", "id")       //nolint:errcheck,gosec
	cmd.MarkFlagsMutuallyExclusive("name", "id") //nolint:errcheck,gosec
}

func resolveAgentTarget(cmd *cobra.Command, at AccountToken, verbose bool) (*agentDeployment, error) {
	if id, _ := cmd.Flags().GetString("id"); id != "" {
		return fetchAgentDeploymentSummary(cmd.Context(), id, at, verbose)
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
		ID:          full.ID,
		Name:        full.Name,
		DisplayName: full.DisplayName,
		BuildID:     full.BuildID,
		Namespace:   full.Namespace,
		Status:      full.Status,
		CreatedAt:   full.CreatedAt,
	}, nil
}

func findDeploymentByTarget(cmd *cobra.Command, target string, at AccountToken, verbose bool) (*agentDeployment, error) {
	u := agentBaseURL() + "/api/v1/deployments?account=" + url.QueryEscape(at.Account)
	var result listDeploymentsResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return nil, err
	}
	for i := range result.Deployments {
		d := &result.Deployments[i]
		if d.DisplayName == target || d.Name == target {
			return d, nil
		}
	}
	return nil, errAgentDeploymentNotFound(target)
}
