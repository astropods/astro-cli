package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var deploymentIDPattern = regexp.MustCompile(`(?i)^[a-z0-9]{3}-[a-z0-9]{3}-[a-z0-9]{3}$`)

func agentTargetArgs(cmd *cobra.Command, args []string) error {
	if len(args) >= 1 {
		return nil
	}
	if cmd != nil {
		if id, _ := cmd.Flags().GetString("id"); id != "" {
			return nil
		}
	}
	return errAgentTargetRequired()
}

func joinAgentTargetArgs(args []string) string {
	return strings.Join(args, " ")
}

func looksLikeDeploymentID(s string) bool {
	return deploymentIDPattern.MatchString(strings.TrimSpace(s))
}

func deploymentLabel(dep *agentDeployment) string {
	if dep.DisplayName != "" {
		return dep.DisplayName
	}
	return dep.Name
}

func registerAgentTargetFlags(cmd *cobra.Command) {
	cmd.Flags().String("id", "", "Deployment ID (from agent list; skips name lookup)")
}

func resolveAgentTarget(cmd *cobra.Command, args []string, at AccountToken, verbose bool) (*agentDeployment, error) {
	if id, _ := cmd.Flags().GetString("id"); id != "" {
		return fetchAgentDeploymentSummary(cmd.Context(), id, at, verbose)
	}

	target := joinAgentTargetArgs(args)
	if target == "" {
		return nil, errAgentTargetRequired()
	}

	if looksLikeDeploymentID(target) {
		if dep, err := fetchAgentDeploymentSummary(cmd.Context(), target, at, verbose); err == nil {
			return dep, nil
		}
	}

	return findDeploymentByTarget(cmd, target, at, verbose)
}

func fetchAgentDeploymentSummary(ctx context.Context, id string, at AccountToken, verbose bool) (*agentDeployment, error) {
	full, err := getAgentDeploymentFull(ctx, id, at, verbose)
	if err != nil {
		return nil, fmt.Errorf("no deployment found for ID %q", id)
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
		if d.DisplayName == target || d.ID == target || strings.EqualFold(d.ID, target) || d.Name == target {
			return d, nil
		}
	}
	return nil, fmt.Errorf("no deployment found for %q", target)
}
