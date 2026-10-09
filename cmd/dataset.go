package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/buildinfo"
)

// datasetServerURLOverride is set in tests to redirect API calls to a test server.
var datasetServerURLOverride string

func datasetBaseURL() string {
	if datasetServerURLOverride != "" {
		return strings.TrimSuffix(datasetServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

var datasetCmd = &cobra.Command{
	Use:   "dataset",
	Short: "Manage an agent's evaluation datasets",
}

var datasetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the account's datasets",
	Long:  "Lists the account's datasets, newest first, with the blueprint and deployment each one belongs to.",
	Args:  cobra.NoArgs,
	RunE:  runDatasetList,
}

func init() {
	rootCmd.AddCommand(datasetCmd)
	datasetCmd.AddCommand(datasetListCmd)
	datasetListCmd.Flags().Int("limit", 50, "Number of datasets to list (max 100)")
	datasetListCmd.Flags().Int("offset", 0, "Pagination offset for list")
	datasetListCmd.Flags().Bool("json", false, "Print raw JSON output")
}

type datasetListItem struct {
	ID           string `json:"id"`
	DatasetName  string `json:"dataset_name"`
	DeploymentID string `json:"deployment_id"`
	AgentName    string `json:"agent_name"`
	CreatedAt    string `json:"created_at"`
}

type datasetListResponse struct {
	Datasets []datasetListItem `json:"datasets"`
	Total    int               `json:"total"`
}

func runDatasetList(cmd *cobra.Command, _ []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	if err := validateListPagination(limit, offset); err != nil {
		return err
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	q := url.Values{}
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("offset", fmt.Sprintf("%d", offset))
	u := apiPath(datasetBaseURL(), at.Account, "accounts", "datasets") + "?" + q.Encode()
	var result datasetListResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result)
	}
	if len(result.Datasets) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoDatasets(), colorReset) //nolint:errcheck,gosec
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tBLUEPRINT\tDEPLOYMENT") //nolint:errcheck,gosec
	for _, d := range result.Datasets {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", d.DatasetName, d.AgentName, d.DeploymentID) //nolint:errcheck,gosec
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if result.Total > offset+len(result.Datasets) {
		fmt.Fprintf(w, "%s\nShowing %d–%d of %d. Page with --offset %d.%s\n", colorDim, //nolint:errcheck,gosec
			offset+1, offset+len(result.Datasets), result.Total, offset+limit, colorReset)
	}
	return nil
}
