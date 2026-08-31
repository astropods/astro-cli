package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

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

var interactiveTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func fetchAllowedClusters(ctx context.Context, at AccountToken, verbose bool) []allowedCluster {
	u := apiPath(blueprintBaseURL(), at.Account, "accounts")
	var resp accountClustersResponse
	if _, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &resp); err != nil {
		return nil
	}
	return resp.AllowedClusters
}

func clusterRegionLabel(c allowedCluster) string {
	flag := c.RegionFlag
	if flag == "" {
		flag = "🌐"
	}
	label := c.RegionLabel
	if label == "" {
		label = c.Region
	}
	if label == "" {
		label = c.ClusterID
	}
	if c.IsDefault {
		label += "  Default"
	}
	return flag + "  " + label
}

func clusterPromptOptions(allowed []allowedCluster) ([]huh.Option[string], string) {
	if len(allowed) == 0 {
		return nil, ""
	}
	options := make([]huh.Option[string], 0, len(allowed))
	selected := allowed[0].ClusterID
	for _, c := range allowed {
		if c.IsDefault {
			selected = c.ClusterID
		}
		options = append(options, huh.NewOption(clusterRegionLabel(c), c.ClusterID))
	}
	return options, selected
}

func deployClusterPromptSuppressed(cmd *cobra.Command) bool {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	jsonOut, _ := cmd.Flags().GetBool("json")
	return dryRun || jsonOut
}

func resolveDeployCluster(cmd *cobra.Command, at AccountToken, verbose bool) (string, error) {
	if flagValue, _ := cmd.Flags().GetString("cluster"); flagValue != "" {
		return flagValue, nil
	}
	if deployClusterPromptSuppressed(cmd) || !interactiveTerminal() {
		return "", nil
	}

	allowed := fetchAllowedClusters(cmd.Context(), at, verbose)
	if len(allowed) < 2 {
		return "", nil
	}

	options, selected := clusterPromptOptions(allowed)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select region").
				Description(msgSelectRegionDescription()).
				Options(options...).
				Value(&selected),
		),
	)
	if err := runForm(form); err != nil {
		return "", err
	}
	return selected, nil
}

func clusterNotAvailableFromErr(err error) error {
	_, body := apiErrorCodeAndBody(err)
	var parsed struct {
		ClusterID         string   `json:"cluster_id"`
		AvailableClusters []string `json:"available_clusters"`
	}
	if json.Unmarshal([]byte(body), &parsed) != nil || parsed.ClusterID == "" {
		return nil
	}
	return errClusterNotAvailable(parsed.ClusterID, parsed.AvailableClusters)
}
