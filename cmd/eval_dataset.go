package cmd

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/theme"
)

var evalDatasetCmd = &cobra.Command{
	Use:   "dataset",
	Short: "Show an agent's evaluation dataset",
	Args:  agentTargetArgs,
	RunE:  runEvalDataset,
}

func init() {
	evalCmd.AddCommand(evalDatasetCmd)
	registerAgentTargetFlags(evalDatasetCmd)
	evalDatasetCmd.Flags().Bool("items", false, "List the dataset's items")
	evalDatasetCmd.Flags().Int("limit", 20, "Number of items to list with --items")
	evalDatasetCmd.Flags().Bool("json", false, "Print raw JSON output")
}

type evalDatasetValueCount struct {
	Value any `json:"value"`
	Count int `json:"count"`
}

type evalDatasetEvaluatorSummary struct {
	Key          string                  `json:"key"`
	Label        string                  `json:"label"`
	Distribution []evalDatasetValueCount `json:"distribution"`
}

type evalDatasetSummary struct {
	ID          string                        `json:"id"`
	DatasetName string                        `json:"dataset_name"`
	ItemCount   int                           `json:"item_count"`
	Evaluators  []evalDatasetEvaluatorSummary `json:"evaluators"`
}

type evalDatasetItem struct {
	ID            string `json:"id"`
	SourceTraceID string `json:"source_trace_id"`
	CreatedAt     string `json:"created_at"`
	Outdated      bool   `json:"outdated"`
}

type evalDatasetItemsResponse struct {
	Items      []evalDatasetItem `json:"items"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
	TotalItems int               `json:"total_items"`
	TotalPages int               `json:"total_pages"`
}

func runEvalDataset(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}
	full, err := getAgentDeploymentFull(cmd.Context(), dep.ID, at, verbose)
	if err != nil {
		return err
	}
	if full.EvalDatasetID == "" {
		return errNoEvalDataset(deploymentLabel(dep))
	}

	u := fmt.Sprintf("%s/api/v1/datasets/%s", evalBaseURL(), url.PathEscape(full.EvalDatasetID))
	var dataset evalDatasetSummary
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &dataset); err != nil {
		return err
	}

	var items evalDatasetItemsResponse
	itemsFlag, _ := cmd.Flags().GetBool("items")
	if itemsFlag {
		limit, _ := cmd.Flags().GetInt("limit")
		if limit <= 0 {
			return errPositiveIntFlag("limit")
		}
		iu := fmt.Sprintf("%s/api/v1/datasets/%s/items?limit=%d", evalBaseURL(), url.PathEscape(full.EvalDatasetID), limit)
		if _, err := apiCall(cmd.Context(), http.MethodGet, iu, nil, at.Token, verbose, &items); err != nil {
			return err
		}
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		if !itemsFlag {
			return writeJSON(w, dataset)
		}
		return writeJSON(w, struct {
			Dataset evalDatasetSummary       `json:"dataset"`
			Items   evalDatasetItemsResponse `json:"items"`
		}{dataset, items})
	}

	accent := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)
	accent.Fprintf(w, "%s\n", dataset.DatasetName)                     //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Items:      %s\n", thousands(dataset.ItemCount)) //nolint:errcheck,gosec
	for _, e := range dataset.Evaluators {
		fmt.Fprintf(w, "  Evaluator:  %s\n", e.Label) //nolint:errcheck,gosec
		for _, v := range e.Distribution {
			dim.Fprintf(w, "    %v: %s\n", v.Value, thousands(v.Count)) //nolint:errcheck,gosec
		}
	}

	if !itemsFlag {
		return nil
	}
	fmt.Fprintln(w) //nolint:errcheck,gosec
	if len(items.Items) == 0 {
		dim.Fprintf(w, "  %s\n", msgNoDatasetItems()) //nolint:errcheck,gosec
		return nil
	}
	for _, item := range items.Items {
		flag := ""
		if item.Outdated {
			flag = "  (outdated)"
		}
		fmt.Fprintf(w, "  %-20s %s%s\n", item.SourceTraceID, shortDate(item.CreatedAt), flag) //nolint:errcheck,gosec
	}
	dim.Fprintf(w, "  page %d of %d, %s total\n", items.Page, items.TotalPages, thousands(items.TotalItems)) //nolint:errcheck,gosec
	return nil
}
