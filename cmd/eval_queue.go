package cmd

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/theme"
)

var evalQueueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Show an agent's evaluation review queue",
	Args:  agentTargetArgs,
	RunE:  runEvalQueue,
}

func init() {
	evalCmd.AddCommand(evalQueueCmd)
	registerAgentTargetFlags(evalQueueCmd)
	evalQueueCmd.Flags().Int("limit", 20, "Number of traces to list")
	evalQueueCmd.Flags().String("evaluation", "", "Filter by evaluation state: evaluated or not_evaluated")
	evalQueueCmd.Flags().Bool("json", false, "Print raw JSON output")
}

type evaluationRun struct {
	ID     string `json:"id,omitempty"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type reviewQueueItem struct {
	TraceID   string         `json:"trace_id"`
	Timestamp string         `json:"timestamp"`
	LatencyMs int            `json:"latency_ms"`
	TotalCost float64        `json:"total_cost"`
	Run       *evaluationRun `json:"run"`
}

type reviewQueueResponse struct {
	Items      []reviewQueueItem `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

type evaluationStatusCounts struct {
	Queued        int `json:"queued"`
	InProgress    int `json:"in_progress"`
	Completed     int `json:"completed"`
	Failed        int `json:"failed"`
	OutdatedCount int `json:"outdated_count"`
}

func runEvalQueue(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}

	limit, _ := cmd.Flags().GetInt("limit")
	if limit <= 0 {
		return errPositiveIntFlag("limit")
	}
	evaluation, _ := cmd.Flags().GetString("evaluation")
	if evaluation != "" && evaluation != "evaluated" && evaluation != "not_evaluated" {
		return errUnknownEvaluationFilter(evaluation)
	}

	su := fmt.Sprintf("%s/api/v1/deployments/%s/review-queue/status", evalBaseURL(), url.PathEscape(dep.ID))
	var status evaluationStatusCounts
	statusCode, err := apiCall(cmd.Context(), http.MethodGet, su, nil, at.Token, verbose, &status)
	if statusCode == http.StatusNotFound {
		return errAgentDeploymentNotFound(deploymentLabel(dep))
	}
	if err != nil {
		return err
	}

	qu := fmt.Sprintf("%s/api/v1/deployments/%s/review-queue?limit=%d", evalBaseURL(), url.PathEscape(dep.ID), limit)
	if evaluation != "" {
		qu += "&evaluation=" + url.QueryEscape(evaluation)
	}
	var queue reviewQueueResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, qu, nil, at.Token, verbose, &queue); err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, struct {
			Status evaluationStatusCounts `json:"status"`
			Queue  reviewQueueResponse    `json:"queue"`
		}{status, queue})
	}

	accent := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)
	accent.Fprintf(w, "%s\n", deploymentLabel(dep))                     //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Queued:      %s\n", thousands(status.Queued))     //nolint:errcheck,gosec
	fmt.Fprintf(w, "  In progress: %s\n", thousands(status.InProgress)) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Completed:   %s\n", thousands(status.Completed))  //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Failed:      %s\n", thousands(status.Failed))     //nolint:errcheck,gosec
	if status.OutdatedCount > 0 {
		fmt.Fprintf(w, "  Outdated:    %s\n", thousands(status.OutdatedCount)) //nolint:errcheck,gosec
	}
	fmt.Fprintln(w) //nolint:errcheck,gosec

	if len(queue.Items) == 0 {
		dim.Fprintf(w, "  %s\n", msgNoQueueItems()) //nolint:errcheck,gosec
		return nil
	}
	for _, item := range queue.Items {
		runState := "not evaluated"
		if item.Run != nil {
			runState = item.Run.Status
		}
		fmt.Fprintf(w, "  %-20s %-12s %6dms  %s  %s\n", //nolint:errcheck,gosec
			item.TraceID, shortDate(item.Timestamp), item.LatencyMs, msgUsageDollars(item.TotalCost), runState)
	}
	if queue.NextCursor != "" {
		dim.Fprintf(w, "  %s\n", msgMoreQueueItems()) //nolint:errcheck,gosec
	}
	return nil
}
