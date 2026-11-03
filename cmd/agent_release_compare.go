package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func newReleaseCompareCommand() *cobra.Command {
	command := &cobra.Command{Use: "compare", Short: "Compare completed evaluations from two builds of a candidate", Args: cobra.NoArgs, RunE: runReleaseCompare}
	command.Flags().String("id", "", "Candidate deployment ID")
	command.Flags().String("run", "", "Current evaluation run ID")
	command.Flags().String("baseline", "", "Earlier build's evaluation run ID")
	command.Flags().Bool("json", false, "Print every case and evaluator comparison as JSON")
	return command
}

type releaseComparison struct {
	BaselineBuildID string `json:"baseline_build_id"`
	CurrentBuildID  string `json:"current_build_id"`
	Changed         int    `json:"changed"`
	Improved        int    `json:"improved"`
	Regressed       int    `json:"regressed"`
}

func runReleaseCompare(cmd *cobra.Command, _ []string) error {
	id, err := releaseDeploymentID(cmd)
	if err != nil {
		return err
	}
	run, _ := cmd.Flags().GetString("run")
	baseline, _ := cmd.Flags().GetString("baseline")
	if run == "" || baseline == "" || run == baseline {
		return errReleaseComparisonRuns()
	}
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	query := url.Values{"baseline": {baseline}}
	var raw json.RawMessage
	if _, err = apiCall(cmd.Context(), http.MethodGet, releaseURL(id, "build-evaluations", url.PathEscape(run), "compare")+"?"+query.Encode(), nil, at.Token, verbose, &raw); err != nil {
		return err
	}
	if output, _ := cmd.Flags().GetBool("json"); output {
		return writeJSON(cmd.OutOrStdout(), raw)
	}
	var comparison releaseComparison
	if err = json.Unmarshal(raw, &comparison); err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), msgReleaseComparison(comparison))
	return err
}
