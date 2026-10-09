package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/theme"
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

var datasetGetCmd = &cobra.Command{
	Use:   "get <dataset-name>",
	Short: "Show a dataset's item count and evaluator values",
	Long:  "Prints the dataset's item count and, for each evaluator, how many items have each value. Run `dataset list` to see dataset names.",
	Args:  cobra.ExactArgs(1),
	RunE:  runDatasetGet,
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
	datasetCmd.AddCommand(datasetGetCmd)
	datasetGetCmd.Flags().Bool("json", false, "Print raw JSON output")
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

type datasetValueCount struct {
	Value json.RawMessage `json:"value"`
	Count int             `json:"count"`
}

type datasetEvaluatorSummary struct {
	Key          string              `json:"key"`
	Label        string              `json:"label"`
	Distribution []datasetValueCount `json:"distribution"`
}

type datasetSummaryResponse struct {
	ID          string                    `json:"id"`
	DatasetName string                    `json:"dataset_name"`
	ItemCount   int                       `json:"item_count"`
	Evaluators  []datasetEvaluatorSummary `json:"evaluators"`
}

// resolveDataset finds the account's dataset with this exact name.
func resolveDataset(cmd *cobra.Command, at AccountToken, verbose bool, name string) (*datasetListItem, error) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("limit", "2")
	u := apiPath(datasetBaseURL(), at.Account, "accounts", "datasets") + "?" + q.Encode()
	var result datasetListResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return nil, err
	}
	switch len(result.Datasets) {
	case 0:
		return nil, errDatasetNotFound(name)
	case 1:
		return &result.Datasets[0], nil
	}
	ids := make([]string, len(result.Datasets))
	for i, d := range result.Datasets {
		ids[i] = d.ID
	}
	return nil, errDatasetAmbiguous(name, ids)
}

func runDatasetGet(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	name := args[0]
	dataset, err := resolveDataset(cmd, at, verbose, name)
	if err != nil {
		return err
	}

	u := fmt.Sprintf("%s/api/v1/datasets/%s", datasetBaseURL(), url.PathEscape(dataset.ID))
	var summary datasetSummaryResponse
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &summary)
	if status == http.StatusNotFound {
		return errDatasetNotFound(name)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, summary)
	}

	accent := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)
	accent.Fprintf(w, "%s\n", summary.DatasetName)             //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Blueprint:  %s\n", dataset.AgentName)    //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Deployment: %s\n", dataset.DeploymentID) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Items:      %d\n", summary.ItemCount)    //nolint:errcheck,gosec
	for _, e := range summary.Evaluators {
		dim.Fprintf(w, "\n%s (%s)\n", e.Label, e.Key) //nolint:errcheck,gosec
		if len(e.Distribution) == 0 {
			fmt.Fprintln(w, "  No values yet") //nolint:errcheck,gosec
			continue
		}
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		for _, v := range e.Distribution {
			fmt.Fprintf(tw, "  %s\t%d\n", rawValue(v.Value), v.Count) //nolint:errcheck,gosec
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return nil
}
