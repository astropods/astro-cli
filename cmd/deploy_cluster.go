package cmd

import (
	"context"
	"net/http"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// allowedCluster is one cluster an account may deploy to, from GET /accounts/:account.
type allowedCluster struct {
	ClusterID   string `json:"cluster_id"`
	Region      string `json:"region"`
	RegionLabel string `json:"region_label"`
	RegionFlag  string `json:"region_flag"`
	IsDefault   bool   `json:"is_default"`
}

type accountClustersResponse struct {
	AllowedClusters []allowedCluster `json:"allowed_clusters"`
}

// fetchAllowedClusters returns the clusters the account may deploy to. A
// failure is not fatal: the caller falls back to the account's default cluster,
// which is what an empty cluster_id already asks the server for.
func fetchAllowedClusters(ctx context.Context, at AccountToken, verbose bool) []allowedCluster {
	u := apiPath(blueprintBaseURL(), at.Account, "accounts")
	var resp accountClustersResponse
	if _, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &resp); err != nil {
		return nil
	}
	return resp.AllowedClusters
}

// resolveDeployCluster decides which cluster a deploy targets. An explicit
// --cluster wins. Otherwise, an account allowed more than one cluster gets an
// interactive prompt, and everything else defers to the account default by
// leaving the value empty.
func resolveDeployCluster(cmd *cobra.Command, at AccountToken, verbose bool) (string, error) {
	flagValue, _ := cmd.Flags().GetString("cluster")
	if flagValue != "" {
		return flagValue, nil
	}

	allowed := fetchAllowedClusters(cmd.Context(), at, verbose)
	if len(allowed) < 2 || !interactiveTerminal() {
		return "", nil
	}

	options := make([]huh.Option[string], 0, len(allowed))
	selected := allowed[0].ClusterID
	for _, c := range allowed {
		label := c.RegionLabel
		if label == "" {
			label = c.Region
		}
		if label == "" {
			label = c.ClusterID
		}
		if c.RegionFlag != "" {
			label = c.RegionFlag + "  " + label
		}
		if c.IsDefault {
			label += " (default)"
			selected = c.ClusterID
		}
		options = append(options, huh.NewOption(label, c.ClusterID))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select region").
				Description("Where this agent runs. You cannot change it after deploying.").
				Options(options...).
				Value(&selected),
		),
	)
	if err := runForm(form); err != nil {
		return "", err
	}
	return selected, nil
}

// interactiveTerminal reports whether prompting the user is possible, so a CI
// deploy silently takes the account default instead of hanging on a form.
func interactiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
